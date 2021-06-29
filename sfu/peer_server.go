package sfu

import (
	"encoding/json"
	"fmt"
	"log"
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

	// sends "ending" message before rooms does end
	go func() {
		<-ps.room.waitForAllCh
		<-time.After(time.Duration(ps.room.EndingDelay()) * time.Second)
		log.Printf("[user %s] ending\n", ps.userId)
		ps.wsConn.Send("ending")
	}()

	for {
		err := ps.wsConn.ReadJSON(&m)

		if err != nil {
			ps.room.DisconnectUser(ps.userId)
			log.Printf("[user %s error] reading JSON: %v\n", ps.userId, err)
			return
		}

		switch m.Kind {
		case "candidate":
			candidate := webrtc.ICECandidateInit{}
			if err := json.Unmarshal([]byte(m.Payload), &candidate); err != nil {
				log.Printf("[user %s error] unmarshal candidate: %v\n", ps.userId, err)
				return
			}

			if err := ps.peerConn.AddICECandidate(candidate); err != nil {
				log.Printf("[user %s error] add candidate: %v\n", ps.userId, err)
				return
			}
		case "answer":
			answer := webrtc.SessionDescription{}
			if err := json.Unmarshal([]byte(m.Payload), &answer); err != nil {
				log.Printf("[user %s error] unmarshal answer: %v\n", ps.userId, err)
				return
			}

			if err := ps.peerConn.SetRemoteDescription(answer); err != nil {
				log.Printf("[user %s error] SetRemoteDescription: %v\n", ps.userId, err)
				return
			}
		}
	}
}

// API

// handle incoming websockets
func RunPeerServer(unsafeConn *websocket.Conn) {

	wsConn := NewWsConn(unsafeConn)
	defer wsConn.Close()

	// first message must be a join request
	joinPayload, err := wsConn.ReadJoin()
	if err != nil {
		log.Printf("[user unknown] join payload corrupted: %v\n", err)
		return
	}

	// used to log info with user id
	wsConn.SetUserId(joinPayload.UserId)

	room, joinErr := JoinRoom(joinPayload)
	if joinErr != nil {
		// joinErr is meaningful to client
		log.Printf("[user %s] join failed: %s", joinPayload.UserId, joinErr)
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
