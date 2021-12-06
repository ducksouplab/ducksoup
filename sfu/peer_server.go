package sfu

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/creamlab/ducksoup/gst"
	_ "github.com/creamlab/ducksoup/helpers" // rely on helpers logger init side-effect
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
	pipeline   *gst.Pipeline
	closed     bool
	closedCh   chan struct{}
	// log
	logger zerolog.Logger
}

func newPeerServer(
	join types.JoinPayload,
	r *room,
	pc *peerConn,
	ws *wsConn) *peerServer {

	pipeline := gst.CreatePipeline(join, r.filePrefixWithCount(join))

	logger := log.With().
		Str("room", join.RoomId).
		Str("user", join.UserId).
		Logger()

	ps := &peerServer{
		userId:   join.UserId,
		roomId:   r.id,
		streamId: uuid.New().String(),
		join:     join,
		r:        r,
		pc:       pc,
		ws:       ws,
		pipeline: pipeline,
		closed:   false,
		closedCh: make(chan struct{}),
		logger:   logger,
	}

	// connect components for further communication
	r.connectPeerServer(ps) // also triggers signaling
	pc.connectPeerServer(ps)

	return ps
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
		ps.logger.Info().Msgf("[ps] closing for reason: %s", reason)
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

func (ps *peerServer) loop() {

	// sends "ending" message before rooms does end
	go func() {
		<-ps.r.waitForAllCh

		select {
		case <-time.After(time.Duration(ps.r.endingDelay()) * time.Second):
			// user might have reconnected and this ps could be
			ps.logger.Info().Msg("[ps] ending message sent")
			ps.ws.send("ending")
		case <-ps.closedCh:
			// user might have disconnected
			return
		}
	}()

	for {
		select {
		case <-ps.r.endCh:
			ps.close("room ended")
			return
		default:
			m, err := ps.ws.read()

			if err != nil {
				ps.close(err.Error())
				return
			}

			switch m.Kind {
			case "candidate":
				candidate := webrtc.ICECandidateInit{}
				if err := json.Unmarshal([]byte(m.Payload), &candidate); err != nil {
					ps.logger.Error().Err(err).Msg("[ps] can't unmarshal candidate")
					return
				}

				if err := ps.pc.AddICECandidate(candidate); err != nil {
					ps.logger.Error().Err(err).Msg("[ps] can't add candidate")
					return
				}
				ps.logger.Info().Msgf("[ps] added remote candidate: %+v", candidate)
			case "answer":
				answer := webrtc.SessionDescription{}
				if err := json.Unmarshal([]byte(m.Payload), &answer); err != nil {
					ps.logger.Error().Err(err).Msg("[ps] can't unmarshal answer")
					return
				}

				if err := ps.pc.SetRemoteDescription(answer); err != nil {
					ps.logger.Error().Err(err).Msg("[ps] can't set remote description")
					return
				}
			case "control":
				payload := controlPayload{}
				if err := json.Unmarshal([]byte(m.Payload), &payload); err != nil {
					ps.logger.Error().Err(err).Msg("[ps] can't unmarshal control")
				} else {
					go func() {
						if payload.Kind == "audio" && ps.audioSlice != nil {
							ps.audioSlice.controlFx(payload)
						} else if ps.videoSlice != nil {
							ps.videoSlice.controlFx(payload)
						}
					}()
				}
			default:
				if strings.HasPrefix(m.Kind, "info-") {
					ps.logger.Debug().Msgf("[remote] %v: %v", strings.TrimPrefix(m.Kind, "info-"), m.Payload)
				}
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

	room, err := rooms.join(joinPayload)
	if err != nil {
		// joinRoom err is meaningful to client
		ws.send(fmt.Sprintf("error-%s", err))
		log.Error().Err(err).Str("room", roomId).Str("user", userId).Msg("[ps] join failed")
		return
	}

	pc, err := newPeerConn(joinPayload, ws)
	if err != nil {
		ws.send("error-peer-connection")
		log.Error().Err(err).Str("room", roomId).Str("user", userId).Msg("[ps] can't create pc")
		return
	}

	ps := newPeerServer(joinPayload, room, pc, ws)

	ps.loop() // blocking
}
