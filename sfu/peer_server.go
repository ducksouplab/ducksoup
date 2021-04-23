package sfu

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

type PeerServer struct {
	peerConn *webrtc.PeerConnection
	wsConn   *WsConn
}

func (ps *PeerServer) loop() {
	var message Message
	for {
		err := ps.wsConn.ReadJSON(&message)

		if err != nil {
			log.Println("[ws] reading JSON failed")
			return
		}

		switch message.Type {
		case "candidate":
			candidate := webrtc.ICECandidateInit{}
			if err := json.Unmarshal([]byte(message.Payload), &candidate); err != nil {
				log.Println(err)
				return
			}

			if err := ps.peerConn.AddICECandidate(candidate); err != nil {
				log.Println(err)
				return
			}
		case "answer":
			answer := webrtc.SessionDescription{}
			if err := json.Unmarshal([]byte(message.Payload), &answer); err != nil {
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

// Handle incoming websockets
func RunPeerServer(unsafeConn *websocket.Conn) {

	wsConn := &WsConn{sync.Mutex{}, unsafeConn}
	defer wsConn.Close()

	// First message must be a join request
	joinRequest, err := wsConn.ReadJoin()
	if err != nil {
		log.Println("[ws] join failed")
		return
	}

	room, joinErr := JoinRoom(joinRequest.Room)
	if joinErr != nil { // joinErr is meaningful to client
		wsConn.WriteJSON(&Message{
			Type:    "error",
			Payload: joinErr.Error(),
		})
	}

	peerConn := NewPeerConnection(room, wsConn, joinRequest.User)
	defer peerConn.Close()

	peerServer := &PeerServer{peerConn, wsConn}

	// Link room and PeerServer
	room.AddPeerServer(peerServer)
	room.SignalingUpdate()

	peerServer.loop()
}
