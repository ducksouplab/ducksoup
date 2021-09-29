package sfu

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/creamlab/ducksoup/engine"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

const delayBetweenPLIs = 500 * time.Millisecond

// New type created mostly to extend webrtc.PeerConnection with additional methods
type peerConn struct {
	sync.Mutex
	*webrtc.PeerConnection
	roomId  string
	userId  string
	lastPLI time.Time
}

// API

func newPionPeerConn(userId string, videoCodec string) (ppc *webrtc.PeerConnection, err error) {
	// create RTC API with chosen codecs
	api, err := engine.NewWebRTCAPI()
	if err != nil {
		log.Printf("[info] [room#%s] [user#%s] [pc] NewWebRTCAPI codecs: %v\n", userId, err)
		return
	}
	// configure and create a new RTCPeerConnection
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
	ppc, err = api.NewPeerConnection(config)
	if err != nil {
		return
	}

	// accept one audio}
	_, err = ppc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		return
	}

	// accept one video
	videoTransceiver, err := ppc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		return
	}

	// set codec preference if H264 is required
	if videoCodec == "H264" {
		err = videoTransceiver.SetCodecPreferences(engine.H264Codecs)
		if err != nil {
			return
		}
	}

	return
}

func newPeerConn(join joinPayload, room *trialRoom, ws *wsConn) (pc *peerConn, err error) {
	userId, videoCodec := join.UserId, join.VideoCodec

	ppc, err := newPionPeerConn(userId, videoCodec)
	if err != nil {
		return
	}

	pc = &peerConn{sync.Mutex{}, ppc, room.shortId, userId, time.Now()}
	return
}

func (pc *peerConn) connectPeerServer(ps *peerServer) {
	userId, roomId := pc.userId, pc.roomId

	// trickle ICE. Emit server candidate to client
	pc.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i == nil {
			// see https://pkg.go.dev/github.com/pion/webrtc/v3#PeerConnection.OnICECandidate
			return
		}

		candidateString, err := json.Marshal(i.ToJSON())
		if err != nil {
			log.Printf("[error] [room#%s] [user#%s] [pc] can't marshal candidate: %v\n", roomId, userId, err)
			return
		}

		ps.ws.sendWithPayload("candidate", string(candidateString))
	})

	// if PeerConnection is closed remove it from global list
	pc.OnConnectionStateChange(func(p webrtc.PeerConnectionState) {
		log.Printf("[info] [room#%s] [user#%s] [pc] OnConnectionStateChange: %s\n", roomId, userId, p.String())
		switch p {
		case webrtc.PeerConnectionStateFailed:
			if err := pc.Close(); err != nil {
				log.Printf("[error] [room#%s] [user#%s] [pc] peer connection failed: %v\n", roomId, userId, err)
			}
		case webrtc.PeerConnectionStateClosed:
			ps.close("pc closed")
		}
	})

	pc.OnNegotiationNeeded(func() {
		log.Printf("[info] [room#%s] [user#%s] [pc] OnNegotiationNeeded\n", roomId, userId)
	})

	pc.OnSignalingStateChange(func(state webrtc.SignalingState) {
		log.Printf("[info] [room#%s] [user#%s] [pc] OnSignalingStateChange: %v\n", roomId, userId, state)
	})

	pc.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("[info] [room#%s] [user#%s] [pc] new incoming track: %s\n", roomId, userId, remoteTrack.Codec().RTPCodecCapability.MimeType)
		ps.room.incInTracksReadyCount()
		<-ps.room.waitForAllCh

		ps.room.runLocalTrackFromRemote(ps, remoteTrack, receiver)
	})
}

func (pc *peerConn) requestPLI() {
	pc.Lock()
	defer pc.Unlock()

	for _, receiver := range pc.GetReceivers() {
		track := receiver.Track()
		if track != nil && track.Kind().String() == "video" {
			durationSinceLastPLI := time.Since(pc.lastPLI)
			if durationSinceLastPLI < delayBetweenPLIs {
				// throttle: don't send too many PLIs
				//log.Printf("[info] [user#%s] [pc] PLI skipped (throttle)\n", pc.userId)
			} else {
				err := pc.WriteRTCP([]rtcp.Packet{
					&rtcp.PictureLossIndication{
						MediaSSRC: uint32(track.SSRC()),
					},
				})
				if err != nil {
					log.Printf("[error] [room#%s] [user#%s] [pc] can't send PLI: %v\n", pc.roomId, pc.userId, err)
				} else {
					pc.lastPLI = time.Now()
					log.Printf("[info] [room#%s] [user#%s] [pc] PLI sent\n", pc.roomId, pc.userId)
				}
			}
		}
	}
}
