package sfu

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/creamlab/ducksoup/engine"
	"github.com/creamlab/ducksoup/types"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
	"github.com/rs/zerolog"
)

// too many PLI may be requested when room starts
// (new peer joins, encoder detecting poor quality... to be investigated)
// that's why we throttle PLI request with initialPLIMinInterval and
// later with mainPLIMinInterval
const initialPLIMinInterval = 2000 * time.Millisecond
const mainPLIMinInterval = 300 * time.Millisecond

// New type created mostly to extend webrtc.PeerConnection with additional methods
type peerConn struct {
	sync.Mutex
	*webrtc.PeerConnection
	userId         string
	r              *room
	lastPLI        time.Time
	pliMinInterval time.Duration
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
		r.logger.Error().Err(err).Str("user", join.UserId)
		return
	}

	// initial lastPLI far enough in the past
	lastPLI := time.Now().Add(-2 * initialPLIMinInterval)

	pc = &peerConn{sync.Mutex{}, ppc, join.UserId, r, lastPLI, initialPLIMinInterval}

	// after an initial delay, change the minimum PLI interval
	go func() {
		<-time.After(4000 * time.Millisecond)
		pc.pliMinInterval = mainPLIMinInterval
	}()

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
	return pc.r.logger.Error().Str("context", "signaling").Str("user", pc.userId)
}

func (pc *peerConn) logInfo() *zerolog.Event {
	return pc.r.logger.Info().Str("context", "signaling").Str("user", pc.userId)
}

func (pc *peerConn) logDebug() *zerolog.Event {
	return pc.r.logger.Debug().Str("context", "signaling").Str("user", pc.userId)
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
			pc.logError().Err(err).Msg("can't marshal candidate")
			return
		}

		ps.ws.sendWithPayload("candidate", string(candidateString))
	})

	pc.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		ssrc := uint32(remoteTrack.SSRC())
		ps.r.addSSRC(ssrc, remoteTrack.Kind().String(), ps.userId)

		msg := fmt.Sprintf("client_%s_track_added", remoteTrack.Kind())
		pc.logDebug().Str("context", "track").Uint32("ssrc", ssrc).Str("track", remoteTrack.ID()).Str("mime", remoteTrack.Codec().RTPCodecCapability.MimeType).Msg(msg)
		ps.r.runMixerSliceFromRemote(ps, remoteTrack, receiver)
	})

	// if PeerConnection is closed remove it from global list
	pc.OnConnectionStateChange(func(p webrtc.PeerConnectionState) {
		pc.logInfo().Str("value", p.String()).Msg("connection_state_changed")
		switch p {
		case webrtc.PeerConnectionStateFailed:
			if err := pc.Close(); err != nil {
				pc.logError().Err(err).Msg("peer connection state failed")
			}
		case webrtc.PeerConnectionStateClosed:
			ps.close("PeerConnection closed")
		}
	})

	// for logging

	pc.OnSignalingStateChange(func(state webrtc.SignalingState) {
		pc.logInfo().Str("value", state.String()).Msg("signaling_state_changed")
	})

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		pc.logInfo().Str("value", state.String()).Msg("ice_connection_state_changed")
	})

	pc.OnICEGatheringStateChange(func(state webrtc.ICEGathererState) {
		pc.logInfo().Str("value", state.String()).Msg("ice_gathering_state_changed")
	})

	pc.OnNegotiationNeeded(func() {
		pc.logInfo().Msg("negotiation_needed")
	})

	// Debug: send periodic PLIs
	// ticker := time.NewTicker(2 * time.Second)
	// go func() {
	// 	for range ticker.C {
	// 		pc.forcedPLIRequest()
	// 	}
	// }()
}

func (pc *peerConn) writePLI(track *webrtc.TrackRemote, cause string) (err error) {
	err = pc.WriteRTCP([]rtcp.Packet{
		&rtcp.PictureLossIndication{
			MediaSSRC: uint32(track.SSRC()),
		},
	})
	if err != nil {
		pc.logError().Err(err).Str("context", "track").Msg("can't send PLI")
	} else {
		pc.Lock()
		pc.lastPLI = time.Now()
		pc.Unlock()
		pc.logInfo().Str("context", "track").Str("cause", cause).Msg("pli_sent")
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

func (pc *peerConn) throttledPLIRequest(waitFor int, cause string) {
	// don't rush
	if waitFor != 0 {
		<-time.After(time.Duration(waitFor) * time.Millisecond)
	}

	pc.Lock()
	defer pc.Unlock()

	for _, receiver := range pc.GetReceivers() {
		track := receiver.Track()
		if track != nil && track.Kind().String() == "video" {
			durationSinceLastPLI := time.Since(pc.lastPLI)
			if durationSinceLastPLI < pc.pliMinInterval {
				// throttle: don't send too many PLIs
				pc.logInfo().Str("context", "track").Str("cause", cause).Msg("pli_skipped")
			} else {
				go pc.writePLI(track, cause)
			}
		}
	}
}
