package sfu

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

type PeerServer struct {
	userId   string
	room     *Room
	peerConn *webrtc.PeerConnection
	wsConn   *WsConn
}

func NewPeerServer(
	joinPayload JoinPayload,
	room *Room,
	peerConn *webrtc.PeerConnection,
	wsConn *WsConn) *PeerServer {

	userId := joinPayload.UserId
	peerServer := &PeerServer{userId, room, peerConn, wsConn}
	return peerServer
}

func (ps *PeerServer) loop() {
	var message Message
	for {
		err := ps.wsConn.ReadJSON(&message)

		if err != nil {
			ps.room.RemovePeer(ps.userId)
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

// handle incoming websockets
func RunPeerServer(unsafeConn *websocket.Conn) {

	wsConn := &WsConn{sync.Mutex{}, unsafeConn}
	defer wsConn.Close()

	// first message must be a join request
	joinPayload, err := wsConn.ReadJoin()
	if err != nil {
		log.Print(err)
		log.Println("[ws] join failed")
		return
	}

	room, joinErr := JoinRoom(joinPayload)
	if joinErr != nil { // joinErr is meaningful to client
		wsConn.Send("error")
	}

	peerConn := NewPeerConnection(joinPayload, room, wsConn)
	defer peerConn.Close()

	peerServer := NewPeerServer(joinPayload, room, peerConn, wsConn)

	// link with room
	room.AddPeer(peerServer)
	room.UpdateSignaling()

	// blocking
	peerServer.loop()
}
