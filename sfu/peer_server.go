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

type PeerServer struct {
	userId   string
	room     *Room
	peerConn *webrtc.PeerConnection
	wsConn   *WsConn
}

func newPeerServer(
	joinPayload JoinPayload,
	room *Room,
	peerConn *webrtc.PeerConnection,
	wsConn *WsConn) *PeerServer {

	userId := joinPayload.UserId
	peerServer := &PeerServer{userId, room, peerConn, wsConn}
	return peerServer
}

func (ps *PeerServer) loop() {
	var m WsMessageIn

	// sends "finishing" message before rooms does finish
	go func() {
		<-ps.room.waitForAllCh
		<-time.After(time.Duration(ps.room.FinishingDelay()) * time.Second)
		log.Printf("[user %s] wsConn> finishing\n", ps.userId)
		ps.wsConn.Send("finishing")
	}()

	for {
		err := ps.wsConn.ReadJSON(&m)

		if err != nil {
			ps.room.DisconnectUser(ps.userId)
			log.Println("[ws] reading JSON failed")
			return
		}

		switch m.Kind {
		case "candidate":
			candidate := webrtc.ICECandidateInit{}
			if err := json.Unmarshal([]byte(m.Payload), &candidate); err != nil {
				log.Println(err)
				return
			}

			if err := ps.peerConn.AddICECandidate(candidate); err != nil {
				log.Println(err)
				return
			}
		case "answer":
			answer := webrtc.SessionDescription{}
			if err := json.Unmarshal([]byte(m.Payload), &answer); err != nil {
				log.Println(err)
				return
			}

			if err := ps.peerConn.SetRemoteDescription(answer); err != nil {
				log.Println(err)
				return
			}
		}
	}
}

// API

// handle incoming websockets
func RunPeerServer(unsafeConn *websocket.Conn) {

	wsConn := &WsConn{sync.Mutex{}, unsafeConn}
	defer wsConn.Close()

	// first message must be a join request
	joinPayload, err := wsConn.ReadJoin()
	if err != nil {
		log.Print(err)
		log.Println("[ws] join payload corrupted")
		return
	}

	room, joinErr := JoinRoom(joinPayload)
	if joinErr != nil {
		// joinErr is meaningful to client
		log.Printf("[user %s-%s] join failed: %s", joinPayload.UserId, joinPayload.Name, joinErr)
		wsConn.Send(fmt.Sprintf("error-%s", joinErr))
		return
	}

	peerConn := NewPeerConnection(joinPayload, room, wsConn)
	defer peerConn.Close()

	peerServer := newPeerServer(joinPayload, room, peerConn, wsConn)

	// bind and signal
	room.Bind(peerServer)
	room.UpdateSignaling()

	peerServer.loop() // blocking
}
