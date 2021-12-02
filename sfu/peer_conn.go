package sfu

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/creamlab/ducksoup/engine"
	_ "github.com/creamlab/ducksoup/helpers" // rely on helpers logger init side-effect
	"github.com/creamlab/ducksoup/types"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const delayBetweenPLIs = 300 * time.Millisecond

// New type created mostly to extend webrtc.PeerConnection with additional methods
type peerConn struct {
	sync.Mutex
	*webrtc.PeerConnection
	lastPLI time.Time
	// log
	logger zerolog.Logger
}

// API

func newPionPeerConn(roomId string, userId string, videoFormat string, logger zerolog.Logger) (ppc *webrtc.PeerConnection, err error) {
	// create RTC API
	api, err := engine.NewWebRTCAPI()
	if err != nil {
		logger.Error().Err(err).Msg("can't create new WebRTC API")
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
		logger.Error().Err(err).Msg("can't create new pion peer connection")
		return
	}

	// accept one audio}
	_, err = ppc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		logger.Error().Err(err).Msg("can't add audio transceiver")
		return
	}

	// accept one video
	videoTransceiver, err := ppc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		logger.Error().Err(err).Msg("can't add video transceiver")
		return
	}

	// force codec preference if H264 (so VP8 won't prevail)
	if videoFormat == "H264" {
		err = videoTransceiver.SetCodecPreferences(engine.H264Codecs)
		if err != nil {
			logger.Error().Err(err).Msg("can't set codec preferences")
			return
		}
	}

	return
}

func newPeerConn(join types.JoinPayload, ws *wsConn) (pc *peerConn, err error) {
	roomId, userId, videoFormat := join.RoomId, join.UserId, join.VideoFormat

	logger := log.With().
		Str("room", join.RoomId).
		Str("user", join.UserId).
		Logger()

	ppc, err := newPionPeerConn(roomId, userId, videoFormat, logger)
	if err != nil {
		return
	}

	// initial lastPLI far enough in the past
	lastPLI := time.Now().Add(-2 * delayBetweenPLIs)

	pc = &peerConn{sync.Mutex{}, ppc, lastPLI, logger}
	return
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
			pc.logger.Error().Err(err).Msg("[pc] can't marshal candidate")
			return
		}

		ps.ws.sendWithPayload("candidate", string(candidateString))
	})

	// if PeerConnection is closed remove it from global list
	pc.OnConnectionStateChange(func(p webrtc.PeerConnectionState) {
		pc.logger.Info().Msgf("[pc] connection state: %v", p.String())
		switch p {
		case webrtc.PeerConnectionStateFailed:
			if err := pc.Close(); err != nil {
				pc.logger.Error().Err(err).Msg("[pc] peer connection state failed")
			}
		case webrtc.PeerConnectionStateClosed:
			ps.close("pc closed")
		}
	})

	pc.OnNegotiationNeeded(func() {
		pc.logger.Info().Msg("[pc] negotiation needed")
	})

	pc.OnSignalingStateChange(func(state webrtc.SignalingState) {
		pc.logger.Info().Msgf("[pc] signaling state: %v", state)
	})

	pc.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		pc.logger.Info().Str("track", remoteTrack.ID()).Msgf("[pc] new incoming %s track", remoteTrack.Codec().RTPCodecCapability.MimeType)
		ps.r.runMixerSliceFromRemote(ps, remoteTrack, receiver)
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
		pc.logger.Error().Err(err).Msg("[pc] can't send PLI")
	} else {
		pc.logger.Info().Msg("[pc] PLI sent")
	}
	return
}

func (pc *peerConn) forcedPLIRequest() {
	pc.Lock()
	defer pc.Unlock()

	for _, receiver := range pc.GetReceivers() {
		track := receiver.Track()
		if track != nil && track.Kind().String() == "video" {
			pc.writePLI(track)
		}
	}
}

func (pc *peerConn) throttledPLIRequest() {
	pc.Lock()
	defer pc.Unlock()

	for _, receiver := range pc.GetReceivers() {
		track := receiver.Track()
		if track != nil && track.Kind().String() == "video" {
			durationSinceLastPLI := time.Since(pc.lastPLI)
			if durationSinceLastPLI < delayBetweenPLIs {
				// throttle: don't send too many PLIs
				pc.logger.Info().Msg("[pc] PLI skipped (throttle)")
			} else {
				err := pc.writePLI(track)
				if err == nil {
					pc.lastPLI = time.Now()
				}
			}
		}
	}
}
