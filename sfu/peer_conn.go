package sfu

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/ducksouplab/ducksoup/engine"
	"github.com/ducksouplab/ducksoup/iceservers"
	"github.com/ducksouplab/ducksoup/types"
	"github.com/pion/interceptor/pkg/cc"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
	"github.com/rs/zerolog"
)

// too many PLI may be requested when interaction starts
// (new peer joins, encoder detecting poor quality... to be investigated)
// that's why we throttle PLI request with initialPLIMinInterval and
// later with mainPLIMinInterval
const initialPLIMinInterval = 1 * time.Second
const mainPLIMinInterval = 500 * time.Millisecond
const changePLIMinIntervalAfter = 3 * time.Second

// New type created mostly to extend webrtc.PeerConnection with additional methods
type peerConn struct {
	sync.Mutex
	*webrtc.PeerConnection
	userId         string
	i              *interaction
	lastPLI        time.Time
	pliMinInterval time.Duration
	ccEstimator    cc.BandwidthEstimator
}

func (pc *peerConn) logError() *zerolog.Event {
	return pc.i.logger.Error().Str("user", pc.userId)
}

func (pc *peerConn) logInfo() *zerolog.Event {
	return pc.i.logger.Info().Str("user", pc.userId)
}

func (pc *peerConn) logDebug() *zerolog.Event {
	return pc.i.logger.Debug().Str("user", pc.userId)
}

// API

func newPionPeerConn(i *interaction) (ppc *webrtc.PeerConnection, ccEstimator cc.BandwidthEstimator, err error) {
	// create RTC API
	estimatorCh := make(chan cc.BandwidthEstimator, 1)
	api, err := engine.NewWebRTCAPI(estimatorCh, i.logger)
	if err != nil {
		return
	}
	// configure and create a new RTCPeerConnection
	config := webrtc.Configuration{}
	config.ICEServers = iceservers.GetDefaultSTUNServers()
	ppc, err = api.NewPeerConnection(config)
	// Wait until our Bandwidth Estimator has been created
	ccEstimator = <-estimatorCh
	return
}

func (pc *peerConn) prepareInTracks(jp types.JoinPayload) (err error) {
	// accept one audio
	_, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		pc.logError().Str("context", "track").Err(err).Msg("add_audio_transceiver_failed")
		return
	}

	// accept one video
	videoTransceiver, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		pc.logError().Str("context", "track").Err(err).Msg("add_video_transceiver_failed")
		return
	}

	// force codec preference if H264 (so VP8 won't prevail)
	if jp.VideoFormat == "H264" {
		err = videoTransceiver.SetCodecPreferences(engine.H264Codecs)
		if err != nil {
			pc.logError().Str("context", "track").Err(err).Msg("set_codec_preferences_failed")
			return
		}
	}
	return
}

func newPeerConn(jp types.JoinPayload, i *interaction) (pc *peerConn, err error) {
	ppc, ccEstimator, err := newPionPeerConn(i)
	if err != nil {
		// pc is not created for now so we use the interaction logger
		i.logger.Error().Err(err).Str("user", jp.UserId)
		return
	}

	// initial lastPLI far enough in the past
	lastPLI := time.Now().Add(-2 * initialPLIMinInterval)

	pc = &peerConn{sync.Mutex{}, ppc, jp.UserId, i, lastPLI, initialPLIMinInterval, ccEstimator}

	// after an initial delay, change the minimum PLI interval
	go func() {
		<-time.After(changePLIMinIntervalAfter)
		pc.pliMinInterval = mainPLIMinInterval
	}()

	err = pc.prepareInTracks(jp)
	return
}

func (pc *peerConn) printSelectedCandidatePair() string {
	candidatePair, _ := pc.SCTP().Transport().ICETransport().GetSelectedCandidatePair()
	return fmt.Sprintf("%+v", candidatePair)
}

// pc callbacks trigger actions handled by ws or interaction or pc itself
func (pc *peerConn) handleCallbacks(ps *peerServer) {
	// trickle ICE. Emit server candidate to client
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			// gathering is finished
			// see https://pkg.go.dev/github.com/pion/webrtc/v3#PeerConnection.OnICECandidate
			return
		}
		pc.logDebug().Str("context", "signaling").Str("value", fmt.Sprintf("%+v", c)).Msg("server_ice_candidate")

		candidateBytes, err := json.Marshal(c.ToJSON())
		if err != nil {
			pc.logError().Str("context", "signaling").Err(err).Msg("marshal_server_ice_candidate_failed")
			return
		}

		ps.ws.sendWithPayload("candidate", string(candidateBytes))
	})

	pc.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		ssrc := uint32(remoteTrack.SSRC())
		ps.i.addSSRC(ssrc, remoteTrack.Kind().String(), ps.userId)

		pc.logDebug().Str("context", "track").Str("kind", remoteTrack.Kind().String()).Str("ssrc", fmt.Sprintf("%x", ssrc)).Str("track", remoteTrack.ID()).Str("params", fmt.Sprintf("%+v", receiver.GetParameters())).Str("mime", remoteTrack.Codec().RTPCodecCapability.MimeType).Msg("in_track_received")
		ps.i.runMixerSliceFromRemote(ps, remoteTrack, receiver)
	})

	// if PeerConnection is closed remove it from global list
	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		switch s {
		case webrtc.PeerConnectionStateFailed:
			ps.close("pc_failed")
		case webrtc.PeerConnectionStateClosed:
			ps.close("pc_closed")
		case webrtc.PeerConnectionStateDisconnected:
			ps.close("pc_disconnected")
			// unnecessary: already logged OnICEConnectionStateChange
			// case webrtc.PeerConnectionStateConnected:
			// 	ps.logDebug().Str("selected_candidate_pair", pc.printSelectedCandidatePair()).Msg("peer_connection_state_connected")
		}
		pc.logDebug().Str("context", "peer").Msg("server_connection_state_" + s.String())
	})

	// for logging

	pc.OnSignalingStateChange(func(s webrtc.SignalingState) {
		pc.logDebug().Str("context", "signaling").Msg("server_signaling_state_" + s.String())
	})

	pc.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) {
		pc.logDebug().Str("context", "signaling").Msg("ice_connection_state_" + s.String())
		switch s {
		case webrtc.ICEConnectionStateDisconnected:
			ps.shareOffer("server_ice_disconnected", true)
		case webrtc.ICEConnectionStateConnected:
			ps.logDebug().Str("context", "signaling").Str("value", pc.printSelectedCandidatePair()).Msg("selected_candidate_pair")
		// case webrtc.ICEConnectionStateCompleted:
		// 	ps.logDebug().Str("context", "signaling").Str("selected_candidate_pair", pc.printSelectedCandidatePair()).Msg("ice_connection_state_completed")
		default:
		}
	})

	pc.OnICEGatheringStateChange(func(s webrtc.ICEGathererState) {
		pc.logDebug().Str("context", "signaling").Msg("server_ice_gathering_state_" + s.String())
	})

	pc.OnNegotiationNeeded(func() {
		ps.shareOffer("server_negotiation_needed", false)
		// TODO check if this would be better: go ps.i.mixer.managedSignalingForEveryone("negotiation_needed", false)
	})
}

func (pc *peerConn) writePLI(track *webrtc.TrackRemote, cause string) (err error) {
	err = pc.WriteRTCP([]rtcp.Packet{
		&rtcp.PictureLossIndication{
			MediaSSRC: uint32(track.SSRC()),
		},
	})
	if err != nil {
		pc.logError().Err(err).Str("context", "track").Msg("server_send_pli_failed")
	} else {
		pc.Lock()
		pc.lastPLI = time.Now()
		pc.Unlock()
		pc.logInfo().Str("context", "track").Str("cause", cause).Msg("server_pli_sent")
	}
	return
}

func (pc *peerConn) throttledPLIRequest(cause string) {
	pc.Lock()
	defer pc.Unlock()

	for _, receiver := range pc.GetReceivers() {
		track := receiver.Track()
		if track != nil && track.Kind().String() == "video" {
			durationSinceLastPLI := time.Since(pc.lastPLI)
			if durationSinceLastPLI < pc.pliMinInterval {
				// throttle: don't send too many PLIs
				pc.logInfo().Str("context", "track").Str("cause", cause).Msg("server_pli_skipped")
			} else {
				go pc.writePLI(track, cause)
			}
		}
	}
}
