package sfu

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/creamlab/ducksoup/gst"
	"github.com/creamlab/ducksoup/sequencing"
	"github.com/creamlab/ducksoup/types"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type peerServer struct {
	sync.Mutex
	userId     string
	roomId     string
	streamId   string // one stream Id shared by mixerSlices on a given pc
	join       types.JoinPayload
	r          *room
	pc         *peerConn
	ws         *wsConn
	audioSlice *mixerSlice
	videoSlice *mixerSlice
	closed     bool
	closedCh   chan struct{}
	// processing
	pipeline          *gst.Pipeline
	interpolatorIndex map[string]*sequencing.LinearInterpolator
}

func newPeerServer(
	join types.JoinPayload,
	r *room,
	pc *peerConn,
	ws *wsConn) *peerServer {

	pipeline := gst.CreatePipeline(join, r.filePrefixWithCount(join))

	ps := &peerServer{
		userId:            join.UserId,
		roomId:            r.id,
		streamId:          uuid.New().String(),
		join:              join,
		r:                 r,
		pc:                pc,
		ws:                ws,
		closed:            false,
		closedCh:          make(chan struct{}),
		pipeline:          pipeline,
		interpolatorIndex: make(map[string]*sequencing.LinearInterpolator),
	}

	// connect components for further communication
	r.connectPeerServer(ps) // also triggers signaling
	pc.connectPeerServer(ps)

	return ps
}

func (ps *peerServer) logError() *zerolog.Event {
	return ps.r.logger.Error().Str("context", "signaling").Str("user", ps.userId)
}

func (ps *peerServer) logInfo() *zerolog.Event {
	return ps.r.logger.Info().Str("context", "signaling").Str("user", ps.userId)
}

func (ps *peerServer) logDebug() *zerolog.Event {
	return ps.r.logger.Debug().Str("context", "signaling").Str("user", ps.userId)
}

func (ps *peerServer) setMixerSlice(kind string, slice *mixerSlice) {
	if kind == "audio" {
		ps.audioSlice = slice
	} else if kind == "video" {
		ps.videoSlice = slice
	}
}

func (ps *peerServer) close(cause string) {
	ps.Lock()
	defer ps.Unlock()

	if !ps.closed {
		// ps.closed check ensure closedCh is not closed twice
		ps.closed = true

		// listened by mixerSlices
		close(ps.closedCh)
		// clean up bound components
		ps.pc.Close()
		ps.ws.Close()
		ps.r.disconnectUser(ps.userId)

		ps.logInfo().Str("context", "peer").Str("cause", cause).Msg("peer_server_ended")
	} else {
		ps.r.deleteIfEmpty()
	}
}

func (ps *peerServer) controlFx(payload controlPayload) {
	interpolatorId := payload.Name + payload.Property
	interpolator := ps.interpolatorIndex[interpolatorId]

	if interpolator != nil {
		// an interpolation is already running for this pipeline, effect and property
		interpolator.Stop()
	}

	ps.logInfo().
		Str("context", "track").
		Str("name", payload.Name).
		Str("property", payload.Property).
		Float32("value", payload.Value).
		Int("duration", payload.Duration).
		Msg("client_fx_control")

	duration := payload.Duration
	if duration == 0 {
		ps.pipeline.SetFxProp(payload.Name, payload.Property, payload.Value)
	} else {
		if duration > maxInterpolatorDuration {
			duration = maxInterpolatorDuration
		}
		oldValue := ps.pipeline.GetFxProp(payload.Name, payload.Property)
		newInterpolator := sequencing.NewLinearInterpolator(oldValue, payload.Value, duration, defaultInterpolatorStep)

		ps.Lock()
		ps.interpolatorIndex[interpolatorId] = newInterpolator
		ps.Unlock()

		defer func() {
			ps.Lock()
			delete(ps.interpolatorIndex, interpolatorId)
			ps.Unlock()
		}()

		for {
			select {
			case <-ps.r.endCh:
				return
			case <-ps.closedCh:
				return
			case currentValue, more := <-newInterpolator.C:
				if more {
					ps.pipeline.SetFxProp(payload.Name, payload.Property, currentValue)
				} else {
					return
				}
			}
		}
	}
}

func (ps *peerServer) loop() {

	// sends "ending" message before rooms does end
	go func() {
		<-ps.r.waitForAllCh
		select {
		case <-time.After(time.Duration(ps.r.endingDelay()) * time.Second):
			// user might have reconnected and this ps could be
			ps.logInfo().Str("context", "peer").Msg("room_ending_sent")
			ps.ws.send("ending")
		case <-ps.closedCh:
			// user might have disconnected
			return
		}
	}()

	// wait for room end
	go func() {
		select {
		case <-ps.r.endCh:
			ps.ws.sendWithPayload("files", ps.r.files()) // peer could have left (ws closed) but room is still running
			ps.close("room ended")
		case <-ps.closedCh:
			// user might have disconnected
			return
		}
	}()

	for {
		m, err := ps.ws.read()
		if err != nil {
			ps.close(err.Error())
			return
		}

		switch m.Kind {
		case "client_candidate":
			candidate := webrtc.ICECandidateInit{}
			if err := json.Unmarshal([]byte(m.Payload), &candidate); err != nil {
				ps.logError().Err(err).Msg("can't unmarshal candidate")
				return
			}

			if err := ps.pc.AddICECandidate(candidate); err != nil {
				ps.logError().Err(err).Msg("can't add candidate")
				return
			}
			ps.logDebug().Str("value", fmt.Sprintf("%+v", candidate)).Msg("client_candidate_added")
		case "client_answer":
			answer := webrtc.SessionDescription{}
			if err := json.Unmarshal([]byte(m.Payload), &answer); err != nil {
				ps.logError().Err(err).Msg("can't unmarshal answer")
				return
			}

			if err := ps.pc.SetRemoteDescription(answer); err != nil {
				ps.logError().Err(err).Msg("can't set remote description")
				return
			}
			ps.logDebug().Msg("client_answer_accepted")
		case "client_control":
			payload := controlPayload{}
			if err := json.Unmarshal([]byte(m.Payload), &payload); err != nil {
				ps.logError().Err(err).Msg("can't unmarshal control")
			} else {
				go func() {
					ps.controlFx(payload)
				}()
			}
		case "client_polycontrol":
			payload := polyControlPayload{}
			if err := json.Unmarshal([]byte(m.Payload), &payload); err != nil {
				ps.logError().Err(err).Msg("can't unmarshal control")
			} else {
				go func() {
					ps.pipeline.SetFxPolyProp(payload.Name, payload.Property, payload.Kind, payload.Value)
					ps.logInfo().
						Str("context", "track").
						Str("name", payload.Name).
						Str("property", payload.Property).
						Str("value", payload.Value).
						Int("duration", payload.Duration).
						Msg("client_fx_control")
				}()
			}
		case "client_video_resolution_updated":
			ps.logDebug().Str("source", "client").Str("value", m.Payload).Str("unit", "pixels").Msg(m.Kind)
		default:
			if strings.HasPrefix(m.Kind, "client_") {
				if strings.Contains(m.Kind, "count") {
					if count, err := strconv.ParseInt(m.Payload, 10, 64); err == nil {
						// "count" logs refer to track context
						ps.logDebug().Str("context", "track").Str("source", "client").Int64("value", count).Msg(m.Kind)
					}
				} else {
					ps.logDebug().Str("source", "client").Str("value", m.Payload).Msg(m.Kind)
				}
			} else if strings.HasPrefix(m.Kind, "ext_") {
				ps.logDebug().
					Str("context", "ext").
					Str("source", "client").
					Str("payload", m.Payload).
					Msg(m.Kind)
			}
		}
	}
}

// API

// handle incoming websockets
func RunPeerServer(origin string, unsafeConn *websocket.Conn) {

	ws := newWsConn(unsafeConn)
	defer ws.Close()

	// first message must be a join request
	joinPayload, err := ws.readJoin(origin)
	if err != nil {
		ws.send("error-join")
		log.Error().Str("context", "signaling").Err(err).Msg("join payload corrupted")
		return
	}

	userId := joinPayload.UserId
	roomId := joinPayload.RoomId
	namespace := joinPayload.Namespace

	r, err := roomStoreSingleton.join(joinPayload)
	if err != nil {
		// joinRoom err is meaningful to client
		ws.send(fmt.Sprintf("error-%s", err))
		log.Error().Str("context", "signaling").Err(err).Str("namespace", namespace).Str("room", roomId).Str("user", userId).Msg("join failed")
		return
	}

	pc, err := newPeerConn(joinPayload, r)
	if err != nil {
		ws.send("error-peer-connection")
		log.Error().Str("context", "peer").Err(err).Str("namespace", namespace).Str("room", roomId).Str("user", userId).Msg("can't create pc")
		return
	}

	ps := newPeerServer(joinPayload, r, pc, ws)

	log.Info().Str("context", "peer").Str("namespace", namespace).Str("room", roomId).Str("user", userId).Msg("peer_server_started")

	ps.loop() // blocking
}
