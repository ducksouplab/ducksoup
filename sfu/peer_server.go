package sfu

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/creamlab/ducksoup/gst"
	"github.com/creamlab/ducksoup/types"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
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
}

func newPeerServer(
	join types.JoinPayload,
	r *room,
	pc *peerConn,
	ws *wsConn) *peerServer {

	pipeline := gst.CreatePipeline(join, r.filePrefixWithCount(join))

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
		log.Printf("[info] [room#%s] [user#%s] [ps] closing for reason: %s\n", ps.roomId, ps.userId, reason)
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
			log.Printf("[info] [room#%s] [user#%s] [ps] ending message sent\n", ps.roomId, ps.userId)
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
					log.Printf("[error] [room#%s] [user#%s]] [ps] can't unmarshal candidate: %v\n", ps.roomId, ps.userId, err)
					return
				}

				if err := ps.pc.AddICECandidate(candidate); err != nil {
					log.Printf("[error] [room#%s] [user#%s] [ps] can't add candidate: %v\n", ps.roomId, ps.userId, err)
					return
				}
			case "answer":
				answer := webrtc.SessionDescription{}
				if err := json.Unmarshal([]byte(m.Payload), &answer); err != nil {
					log.Printf("[error] [room#%s] [user#%s] [ps] can't unmarshal answer: %v\n", ps.roomId, ps.userId, err)
					return
				}

				if err := ps.pc.SetRemoteDescription(answer); err != nil {
					log.Printf("[error] [room#%s] [user#%s] [ps] can't SetRemoteDescription: %v\n", ps.roomId, ps.userId, err)
					return
				}
			case "control":
				payload := controlPayload{}
				if err := json.Unmarshal([]byte(m.Payload), &payload); err != nil {
					log.Printf("[error] [room#%s] [user#%s] [ps] can't unmarshal control: %v\n", ps.roomId, ps.userId, err)
				} else {
					go func() {
						if payload.Kind == "audio" && ps.audioSlice != nil {
							ps.audioSlice.controlFx(payload)
						} else if ps.videoSlice != nil {
							ps.videoSlice.controlFx(payload)
						}
					}()
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
		log.Printf("[error] [user unknown] [ps] join payload corrupted: %v\n", err)
		return
	}
	userId := joinPayload.UserId
	roomId := joinPayload.RoomId

	// used to log info with room and user id
	ws.setIds(roomId, userId)

	room, err := rooms.join(joinPayload)
	if err != nil {
		// joinRoom err is meaningful to client
		ws.send(fmt.Sprintf("error-%s", err))
		log.Printf("[error] [room#%s] [user#%s] [ps] join failed: %s\n", roomId, userId, err)
		return
	}

	pc, err := newPeerConn(joinPayload, ws)
	if err != nil {
		ws.send("error-peer-connection")
		log.Printf("[error] [room#%s] [user#%s] [ps] can't create pc: %s\n", roomId, userId, err)
		return
	}

	ps := newPeerServer(joinPayload, room, pc, ws)

	ps.loop() // blocking
}
