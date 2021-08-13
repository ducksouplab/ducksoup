package sfu

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

type peerServer struct {
	sync.Mutex
	userId     string
	room       *trialRoom
	join       joinPayload
	pc         *peerConn
	ws         *wsConn
	audioTrack *localTrack
	videoTrack *localTrack
	closed     bool
	closedCh   chan struct{}
}

func newPeerServer(
	join joinPayload,
	room *trialRoom,
	pc *peerConn,
	ws *wsConn) *peerServer {
	ps := &peerServer{
		userId:   join.UserId,
		room:     room,
		join:     join,
		pc:       pc,
		ws:       ws,
		closed:   false,
		closedCh: make(chan struct{}),
	}

	// connect components for further communication
	room.connectPeerServer(ps) // also triggers signaling
	pc.connectPeerServer(ps)

	return ps
}

func (ps *peerServer) setLocalTrack(kind string, outputTrack *localTrack) {
	if kind == "audio" {
		ps.audioTrack = outputTrack
	} else if kind == "video" {
		ps.videoTrack = outputTrack
	}
}

func (ps *peerServer) close(reason string) {
	ps.Lock()
	defer ps.Unlock()

	if !ps.closed {
		log.Printf("[info] [user#%s] [ps] closing, reason: %s\n", ps.userId, reason)
		// ps.closed check ensure closedCh is not closed twice
		ps.closed = true

		// listened by localTracks
		close(ps.closedCh)
		// clean up bound components
		ps.room.disconnectUser(ps.userId)
		ps.pc.Close()
		ps.ws.Close()
	}
}

func (ps *peerServer) loop() {

	// sends "ending" message before rooms does end
	go func() {
		<-ps.room.waitForAllCh
		<-time.After(time.Duration(ps.room.endingDelay()) * time.Second)
		log.Printf("[info] [user#%s] [ps] ending message sent\n", ps.userId)
		ps.ws.send("ending")
	}()

	for {
		select {
		case <-ps.room.endCh:
			ps.close("room ended")
			return
		default:
			m, err := ps.ws.read()

			if err != nil {
				ps.close("ws error")
				return
			}

			switch m.Kind {
			case "candidate":
				candidate := webrtc.ICECandidateInit{}
				if err := json.Unmarshal([]byte(m.Payload), &candidate); err != nil {
					log.Printf("[error] [ps user#%s] can't unmarshal candidate: %v\n", ps.userId, err)
					return
				}

				if err := ps.pc.AddICECandidate(candidate); err != nil {
					log.Printf("[error] [ps user#%s] can't add candidate: %v\n", ps.userId, err)
					return
				}
			case "answer":
				answer := webrtc.SessionDescription{}
				if err := json.Unmarshal([]byte(m.Payload), &answer); err != nil {
					log.Printf("[error] [ps user#%s] can't unmarshal answer: %v\n", ps.userId, err)
					return
				}

				if err := ps.pc.SetRemoteDescription(answer); err != nil {
					log.Printf("[error] [ps user#%s] cant' SetRemoteDescription: %v\n", ps.userId, err)
					return
				}
			case "control":
				payload := controlPayload{}
				if err := json.Unmarshal([]byte(m.Payload), &payload); err != nil {
					log.Printf("[error] [ps user#%s] can't unmarshal control: %v\n", ps.userId, err)
				} else {
					go func() {
						if payload.Kind == "audio" && ps.audioTrack != nil {
							ps.audioTrack.controlFx(payload)
						} else if ps.videoTrack != nil {
							ps.videoTrack.controlFx(payload)
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
		log.Printf("[wrong] [ps user unknown] join payload corrupted: %v\n", err)
		return
	}
	userId := joinPayload.UserId

	// used to log info with user id
	ws.setUserId(userId)

	room, err := joinRoom(joinPayload)
	if err != nil {
		// joinRoom err is meaningful to client
		ws.send(fmt.Sprintf("error-%s", err))
		log.Printf("[wrong] [ps user#%s] join failed: %s\n", userId, err)
		return
	}

	pc, err := newPeerConn(joinPayload, room, ws)
	if err != nil {
		ws.send("error-peer-connection")
		log.Printf("[error] [ps user#%s] can't create pc: %s\n", userId, err)
		return
	}

	ps := newPeerServer(joinPayload, room, pc, ws)

	ps.loop() // blocking
}
