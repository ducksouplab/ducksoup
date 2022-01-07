package sfu

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/creamlab/ducksoup/engine"
	"github.com/creamlab/ducksoup/types"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
	"github.com/rs/zerolog"
)

const delayBetweenPLIs = 300 * time.Millisecond

// New type created mostly to extend webrtc.PeerConnection with additional methods
type peerConn struct {
	sync.Mutex
	*webrtc.PeerConnection
	userId  string
	r       *room
	lastPLI time.Time
}

// API

func newPionPeerConn(join types.JoinPayload, r *room) (ppc *webrtc.PeerConnection, err error) {
	// create RTC API
	api, err := engine.NewWebRTCAPI()
	if err != nil {
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
	return
}

func newPeerConn(join types.JoinPayload, r *room) (pc *peerConn, err error) {
	ppc, err := newPionPeerConn(join, r)
	if err != nil {
		// pc is not created for now so we use the room logger
		r.logError().Err(err).Str("user", join.UserId)
		return
	}

	// initial lastPLI far enough in the past
	lastPLI := time.Now().Add(-2 * delayBetweenPLIs)

	pc = &peerConn{sync.Mutex{}, ppc, join.UserId, r, lastPLI}

	// accept one audio
	_, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		pc.logError().Err(err).Msg("can't add audio transceiver")
		return
	}

	// accept one video
	videoTransceiver, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		pc.logError().Err(err).Msg("can't add video transceiver")
		return
	}

	// force codec preference if H264 (so VP8 won't prevail)
	if join.VideoFormat == "H264" {
		err = videoTransceiver.SetCodecPreferences(engine.H264Codecs)
		if err != nil {
			pc.logError().Err(err).Msg("can't set codec preferences")
			return
		}
	}
	return
}

func (pc *peerConn) logError() *zerolog.Event {
	return pc.r.logError().Str("user", pc.userId)
}

func (pc *peerConn) logInfo() *zerolog.Event {
	return pc.r.logInfo().Str("user", pc.userId)
}

func (pc *peerConn) connectPeerServer(ps *peerServer) {
	// trickle ICE. Emit server candidate to client
	pc.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i == nil {
			// see https://pkg.go.dev/github.com/pion/webrtc/v3#PeerConnection.OnICECandidate
			return
		}

		candidateString, err := json.Marshal(i.ToJSON())
		if err != nil {
			pc.logError().Err(err).Msg("[pc] can't marshal candidate")
			return
		}

		ps.ws.sendWithPayload("candidate", string(candidateString))
	})

	pc.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		pc.logInfo().Str("track", remoteTrack.ID()).Msgf("[pc] new incoming %s track", remoteTrack.Codec().RTPCodecCapability.MimeType)
		ps.r.runMixerSliceFromRemote(ps, remoteTrack, receiver)
	})

	// if PeerConnection is closed remove it from global list
	pc.OnConnectionStateChange(func(p webrtc.PeerConnectionState) {
		pc.logInfo().Msgf("[pc] connection state: %v", p.String())
		switch p {
		case webrtc.PeerConnectionStateFailed:
			if err := pc.Close(); err != nil {
				pc.logError().Err(err).Msg("[pc] peer connection state failed")
			}
		case webrtc.PeerConnectionStateClosed:
			ps.close("pc closed")
		}
	})

	// for logging

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		pc.logInfo().Msgf("[pc] ice state change: %v", state)
	})

	pc.OnICEGatheringStateChange(func(state webrtc.ICEGathererState) {
		pc.logInfo().Msgf("[pc] ice gathering state change: %v", state)
	})

	pc.OnNegotiationNeeded(func() {
		pc.logInfo().Msg("[pc] negotiation needed")
	})

	pc.OnSignalingStateChange(func(state webrtc.SignalingState) {
		pc.logInfo().Msgf("[pc] signaling state change: %v", state)
	})

	// Debug: send periodic PLIs
	// ticker := time.NewTicker(2 * time.Second)
	// go func() {
	// 	for range ticker.C {
	// 		pc.forcedPLIRequest()
	// 	}
	// }()
}

func (pc *peerConn) writePLI(track *webrtc.TrackRemote) (err error) {
	err = pc.WriteRTCP([]rtcp.Packet{
		&rtcp.PictureLossIndication{
			MediaSSRC: uint32(track.SSRC()),
		},
	})
	if err != nil {
		pc.logError().Err(err).Msg("[pc] can't send PLI")
	} else {
		pc.Lock()
		pc.lastPLI = time.Now()
		pc.Unlock()
		pc.logInfo().Msg("[pc] PLI sent")
	}
	return
}

// func (pc *peerConn) forcedPLIRequest() {
// 	pc.Lock()
// 	defer pc.Unlock()

// 	for _, receiver := range pc.GetReceivers() {
// 		track := receiver.Track()
// 		if track != nil && track.Kind().String() == "video" {
// 			pc.writePLI(track)
// 		}
// 	}
// }

func (pc *peerConn) throttledPLIRequest() {
	// don't rush
	<-time.After(200 * time.Millisecond)

	pc.Lock()
	defer pc.Unlock()

	for _, receiver := range pc.GetReceivers() {
		track := receiver.Track()
		if track != nil && track.Kind().String() == "video" {
			durationSinceLastPLI := time.Since(pc.lastPLI)
			if durationSinceLastPLI < delayBetweenPLIs {
				// throttle: don't send too many PLIs
				pc.logInfo().Msg("[pc] PLI skipped (throttle)")
			} else {
				go pc.writePLI(track)
			}
		}
	}
}
