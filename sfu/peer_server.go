package sfu

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

type peerServer struct {
	userId     string
	room       *trialRoom
	pc         *peerConn
	ws         *wsConn
	audioTrack *localTrack
	videoTrack *localTrack
}

func newPeerServer(
	join joinPayload,
	room *trialRoom,
	pc *peerConn,
	ws *wsConn) *peerServer {
	return &peerServer{
		userId: join.UserId,
		room:   room,
		pc:     pc,
		ws:     ws,
	}
}

func (ps *peerServer) setLocalTrack(kind string, outputTrack *localTrack) {
	if kind == "audio" {
		ps.audioTrack = outputTrack
	} else if kind == "video" {
		ps.videoTrack = outputTrack
	}
}

func (ps *peerServer) loop() {
	var m messageIn

	// sends "ending" message before rooms does end
	go func() {
		<-ps.room.waitForAllCh
		<-time.After(time.Duration(ps.room.endingDelay()) * time.Second)
		log.Printf("[user %s] ending\n", ps.userId)
		ps.ws.Send("ending")
	}()

	for {
		err := ps.ws.ReadJSON(&m)

		if err != nil {
			ps.room.disconnectUser(ps.userId)
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure) {
				log.Printf("[user %s error] reading JSON: %v\n", ps.userId, err)
			}
			return
		}

		switch m.Kind {
		case "candidate":
			candidate := webrtc.ICECandidateInit{}
			if err := json.Unmarshal([]byte(m.Payload), &candidate); err != nil {
				log.Printf("[user %s error] unmarshal candidate: %v\n", ps.userId, err)
				return
			}

			if err := ps.pc.AddICECandidate(candidate); err != nil {
				log.Printf("[user %s error] add candidate: %v\n", ps.userId, err)
				return
			}
		case "answer":
			answer := webrtc.SessionDescription{}
			if err := json.Unmarshal([]byte(m.Payload), &answer); err != nil {
				log.Printf("[user %s error] unmarshal answer: %v\n", ps.userId, err)
				return
			}

			if err := ps.pc.SetRemoteDescription(answer); err != nil {
				log.Printf("[user %s error] SetRemoteDescription: %v\n", ps.userId, err)
				return
			}
		case "control":
			payload := controlPayload{}
			if err := json.Unmarshal([]byte(m.Payload), &payload); err != nil {
				log.Printf("[user %s error] unmarshal control: %v\n", ps.userId, err)
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

// API

// handle incoming websockets
func RunPeerServer(origin string, unsafeConn *websocket.Conn) {

	ws := NewWsConn(unsafeConn)
	defer ws.Close()

	// first message must be a join request
	joinPayload, err := ws.ReadJoin(origin)
	if err != nil {
		log.Printf("[user unknown] join payload corrupted: %v\n", err)
		return
	}

	// used to log info with user id
	ws.SetUserId(joinPayload.UserId)

	room, err := joinRoom(joinPayload)

	if err != nil {
		// joinErr is meaningful to client
		log.Printf("[user %s] join failed: %s", joinPayload.UserId, err)
		ws.Send(fmt.Sprintf("error-%s", err))
		return
	}

	pc := newPeerConn(joinPayload, room, ws)
	defer pc.Close()

	ps := newPeerServer(joinPayload, room, pc, ws)

	// bind (and automatically trigger a signaling update)
	room.bindPeerServer(ps)

	ps.loop() // blocking
}
