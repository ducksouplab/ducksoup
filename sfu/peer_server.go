package sfu

import (
	"encoding/json"
	"fmt"
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
	return ps.r.logError().Str("user", ps.userId)
}

func (ps *peerServer) logInfo() *zerolog.Event {
	return ps.r.logInfo().Str("user", ps.userId)
}

func (ps *peerServer) logDebug() *zerolog.Event {
	return ps.r.logDebug().Str("user", ps.userId)
}

func (ps *peerServer) setMixerSlice(kind string, slice *mixerSlice) {
	if kind == "audio" {
		ps.audioSlice = slice
	} else if kind == "video" {
		ps.videoSlice = slice
	}
}

func (ps *peerServer) close(reason string) {
	ps.Lock()
	defer ps.Unlock()

	if !ps.closed {
		ps.logInfo().Msgf("[ps] closing for reason: %s", reason)
		// ps.closed check ensure closedCh is not closed twice
		ps.closed = true

		// listened by mixerSlices
		close(ps.closedCh)
		// clean up bound components
		ps.pc.Close()
		ps.ws.Close()
		ps.r.disconnectUser(ps.userId)
	}
}

func (ps *peerServer) controlFx(payload controlPayload) {
	interpolatorId := payload.Name + payload.Property
	interpolator := ps.interpolatorIndex[interpolatorId]

	if interpolator != nil {
		// an interpolation is already running for this pipeline, effect and property
		interpolator.Stop()
	}

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
			ps.logInfo().Msg("[ps] ending message sent")
			ps.ws.send("ending")
		case <-ps.closedCh:
			// user might have disconnected
			return
		}
	}()

	// wait for room end
	go func() {
		<-ps.r.endCh
		ps.ws.sendWithPayload("files", ps.r.files()) // peer could have left (ws closed) but room is still running
		ps.close("room ended")
	}()

	for {
		m, err := ps.ws.read()
		if err != nil {
			ps.close(err.Error())
			return
		}

		switch m.Kind {
		case "candidate":
			candidate := webrtc.ICECandidateInit{}
			if err := json.Unmarshal([]byte(m.Payload), &candidate); err != nil {
				ps.logError().Err(err).Msg("[ps] can't unmarshal candidate")
				return
			}

			if err := ps.pc.AddICECandidate(candidate); err != nil {
				ps.logError().Err(err).Msg("[ps] can't add candidate")
				return
			}
			ps.logInfo().Msgf("[ps] added remote candidate: %+v", candidate)
		case "answer":
			answer := webrtc.SessionDescription{}
			if err := json.Unmarshal([]byte(m.Payload), &answer); err != nil {
				ps.logError().Err(err).Msg("[ps] can't unmarshal answer")
				return
			}

			if err := ps.pc.SetRemoteDescription(answer); err != nil {
				ps.logError().Err(err).Msg("[ps] can't set remote description")
				return
			}
		case "control":
			payload := controlPayload{}
			if err := json.Unmarshal([]byte(m.Payload), &payload); err != nil {
				ps.logError().Err(err).Msg("[ps] can't unmarshal control")
			} else {
				go func() {
					ps.controlFx(payload)
				}()
			}
		case "polycontrol":
			payload := polyControlPayload{}
			if err := json.Unmarshal([]byte(m.Payload), &payload); err != nil {
				ps.logError().Err(err).Msg("[ps] can't unmarshal control")
			} else {
				go func() {
					ps.pipeline.SetFxPolyProp(payload.Name, payload.Property, payload.Kind, payload.Value)
				}()
			}
		default:
			if strings.HasPrefix(m.Kind, "debug-") {
				ps.logDebug().Msgf("[remote] %v: %v", strings.TrimPrefix(m.Kind, "debug-"), m.Payload)
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
		log.Error().Err(err).Msg("[ps] join payload corrupted")
		return
	}

	userId := joinPayload.UserId
	roomId := joinPayload.RoomId
	log.Info().Str("room", roomId).Str("user", userId).Msgf("joined with payload: %+v", joinPayload)

	r, err := rooms.join(joinPayload)
	if err != nil {
		// joinRoom err is meaningful to client
		ws.send(fmt.Sprintf("error-%s", err))
		log.Error().Err(err).Str("room", roomId).Str("user", userId).Msg("[ps] join failed")
		return
	}

	pc, err := newPeerConn(joinPayload, r)
	if err != nil {
		ws.send("error-peer-connection")
		log.Error().Err(err).Str("room", roomId).Str("user", userId).Msg("[ps] can't create pc")
		return
	}

	ps := newPeerServer(joinPayload, r, pc, ws)

	ps.loop() // blocking
}
