package sfu

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ducksouplab/ducksoup/env"
	"github.com/ducksouplab/ducksoup/gst"
	"github.com/ducksouplab/ducksoup/iceservers"
	"github.com/ducksouplab/ducksoup/sequencing"
	"github.com/ducksouplab/ducksoup/types"
	"github.com/google/uuid"
	"github.com/pion/webrtc/v3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	ws "github.com/silently/wsmock"
)

const (
	maxWaitingForJoin       = 10 * time.Second
	maxInterpolatorDuration = 5000
)

type peerServer struct {
	sync.Mutex
	userId          string
	interactionName string
	streamId        string // one stream Id shared by mixerSlices on a given pc
	join            types.JoinPayload
	i               *interaction
	pc              *peerConn
	ws              *wsConn
	audioSlice      *mixerSlice
	videoSlice      *mixerSlice
	closed          bool
	doneCh          chan struct{}
	// processing
	pipeline          *gst.Pipeline
	interpolatorIndex map[string]*sequencing.LinearInterpolator
}

func newPeerServer(
	jp types.JoinPayload,
	i *interaction,
	pc *peerConn,
	ws *wsConn) *peerServer {

	pipeline := gst.NewPipeline(jp, i.randomId, i.joinedCountForUser(jp.UserId), i.logger)

	ps := &peerServer{
		userId:            jp.UserId,
		interactionName:   i.name,
		streamId:          uuid.New().String(),
		join:              jp,
		i:                 i,
		pc:                pc,
		ws:                ws,
		closed:            false,
		doneCh:            make(chan struct{}),
		pipeline:          pipeline,
		interpolatorIndex: make(map[string]*sequencing.LinearInterpolator),
	}

	// connect for further communication
	i.connectPeerServer(ps)
	ws.connectPeerServer(ps)
	if i.allUsersConnected() {
		// optim: update tracks from others (all are there) and share offer
		ps.updateTracksAndShareOffer("initial_offer_room_incomplete")
	} else {
		// ready to share offer, in(coming) tracks already prepared during PC initialization
		ps.shareOffer("initial_offer_room_incomplete", false)
	}
	// some events on pc needs API from ws or interaction
	pc.handleCallbacks(ps)

	i.logger.Info().Str("context", "peer").Str("user", ps.userId).Msg("peer_server_started")

	return ps
}

func (ps *peerServer) isDone() chan struct{} {
	return ps.doneCh
}

func (ps *peerServer) logError() *zerolog.Event {
	return ps.i.logger.Error().Str("user", ps.userId)
}

func (ps *peerServer) logInfo() *zerolog.Event {
	return ps.i.logger.Info().Str("user", ps.userId)
}

func (ps *peerServer) logDebug() *zerolog.Event {
	return ps.i.logger.Debug().Str("user", ps.userId)
}

func (ps *peerServer) setMixerSlice(kind string, ms *mixerSlice) {
	if kind == "audio" {
		ps.audioSlice = ms
	} else if kind == "video" {
		ps.videoSlice = ms
	}
}

func (ps *peerServer) cleanOutTracks() {
	userId := ps.userId
	pc := ps.pc

	for _, sender := range pc.GetSenders() {
		if sender.Track() == nil {
			continue
		}
		sentTrackId := sender.Track().ID()
		// if we have a RTPSender that doesn't map to an existing track remove and signal
		_, ok := ps.i.mixer.sliceIndex[sentTrackId]
		if !ok {
			if err := pc.RemoveTrack(sender); err != nil {
				ps.logError().Str("context", "signaling").Err(err).Str("user", userId).Str("track", sentTrackId).Msg("remove_track_failed")
			} else {
				ps.logInfo().Str("context", "signaling").Str("user", userId).Str("track", sentTrackId).Msg("track_removed")
			}
		}
	}
}

func (ps *peerServer) prepareOutTracks() bool {
	userId := ps.userId
	pc := ps.pc

	// map of sender we are already sending, so we don't double send
	alreadySentIndex := map[string]bool{}

	for _, sender := range pc.GetSenders() {
		if sender.Track() == nil {
			continue
		}
		sentTrackId := sender.Track().ID()
		// if we have a RTPSender that doesn't map to an existing track remove and signal
		_, ok := ps.i.mixer.sliceIndex[sentTrackId]
		if ok {
			alreadySentIndex[sentTrackId] = true
		}
	}

	// add all necessary track (not yet to the PeerConnection or not coming from same peer)
	for id, s := range ps.i.mixer.sliceIndex {
		_, alreadySent := alreadySentIndex[id]

		trackId := s.ID()
		fromId := s.fromPs.userId
		if alreadySent {
			// don't double send
			ps.logInfo().Str("user", userId).Str("from", fromId).Str("track", trackId).Msg("add_dup_track_to_pc_skipped")
		} else if ps.i.size != 1 && s.fromPs.userId == userId {
			// don't send own tracks, except when interaction size is 1 (interaction then acts as a mirror)
			ps.logInfo().Str("user", userId).Str("from", fromId).Str("track", trackId).Msg("add_own_track_to_pc_skipped")
		} else {
			sender, err := pc.AddTrack(s.output)
			if err != nil {
				ps.logError().Str("context", "signaling").Err(err).Str("user", userId).Str("from", fromId).Str("track", trackId).Msg("add_out_track_to_pc_failed")
				return false
			} else {
				ps.logInfo().Str("user", userId).Str("from", fromId).Str("track", trackId).Msg("out_track_added_to_pc")
			}
			s.addSender(pc, sender)
		}
	}
	return true
}

func (ps *peerServer) shareOffer(cause string, iceRestart bool) bool {
	userId := ps.userId
	pc := ps.pc

	ps.logInfo().Str("user", userId).Str("cause", cause).Str("current_state", pc.SignalingState().String()).Msg("server_create_offer_requested")

	if pc.PendingLocalDescription() != nil {
		ps.logError().Str("context", "signaling").Str("user", userId).Msg("server_pending_local_description_blocking_offer")
		return false
	}

	options := &webrtc.OfferOptions{}
	if iceRestart {
		options.ICERestart = true
	}
	offer, err := pc.CreateOffer(options)
	if err != nil {
		ps.logError().Str("context", "signaling").Str("user", userId).Msg("server_create_offer_failed")
		return false
	}

	var gatherComplete <-chan struct{}
	// if we rely on a public IP, we need to gather candidates when creating the offer
	waitForCandidatesGathering := env.ExplicitHostCandidate
	if waitForCandidatesGathering {
		// channel that will be closed when the gathering of local candidates is complete
		// needed especially when using DUCKSOUP_PUBLIC_IP as a host candidate
		gatherComplete = webrtc.GatheringCompletePromise(ps.pc.PeerConnection)
	}

	if err = pc.SetLocalDescription(offer); err != nil {
		ps.logError().Str("context", "signaling").Str("user", userId).Str("sdp", offer.SDP).Err(err).Msg("server_set_local_description_failed")
		return false
	} else {
		ps.logDebug().Str("context", "signaling").Str("user", userId).Str("offer", fmt.Sprintf("%v", offer)).Msg("server_set_local_description")
	}

	// override offer if we had to wait for candidates gathering
	if waitForCandidatesGathering {
		ps.logDebug().Str("context", "signaling").Str("user", userId).Msg("server_candidates_gathering_waiting")
		<-gatherComplete
		ps.logDebug().Str("context", "signaling").Str("user", userId).Msg("server_candidates_gathering_complete")
		offer = *ps.pc.LocalDescription()
	}

	offerString, err := json.Marshal(offer)
	if err != nil {
		ps.logError().Str("context", "signaling").Str("user", userId).Err(err).Msg("marshal_offer_failed")
		return false
	}

	if err = ps.ws.sendWithPayload("offer", string(offerString)); err != nil {
		return false
	}
	return true
}

func (ps *peerServer) updateTracksAndShareOffer(cause string) bool {
	ps.cleanOutTracks()
	if state := ps.prepareOutTracks(); !state {
		return state
	}
	if state := ps.shareOffer(cause, false); !state {
		return state
	}
	return true
}

func (ps *peerServer) close(cause string) {
	ps.Lock()
	defer ps.Unlock()

	if !ps.closed {
		// ps.closed check ensure doneCh is not closed twice
		ps.closed = true

		// listened by mixerSlices
		close(ps.doneCh)
		// clean up bound components
		go ps.pc.Close() // TODO fix/check -> may block
		ps.ws.Close()

		ps.logInfo().Str("context", "peer").Str("cause", cause).Msg("peer_server_ended")
	}
	// cleanup anyway
	ps.i.disconnectUser(ps)
}

func (ps *peerServer) controlFx(payload controlPayload) {
	ps.logInfo().
		Str("context", "track").
		Str("from", payload.fromUserId).
		Str("name", payload.Name).
		Str("property", payload.Property).
		Float32("value", payload.Value).
		Int("duration", payload.Duration).
		Msg("client_fx_control")

	interpolatorId := payload.Name + payload.Property
	ps.Lock()
	interpolator := ps.interpolatorIndex[interpolatorId]
	if interpolator != nil {
		// an interpolation is already running for this pipeline, effect and property
		interpolator.Stop()
	}

	duration := payload.Duration
	if duration == 0 {
		ps.pipeline.SetFxPropFloat(payload.Name, payload.Property, payload.Value)
		ps.Unlock()
		return
	} else {
		if duration > maxInterpolatorDuration {
			duration = maxInterpolatorDuration
		}
		oldValue := ps.pipeline.GetFxPropFloat(payload.Name, payload.Property)
		newInterpolator := sequencing.NewLinearInterpolator(oldValue, payload.Value, duration, defaultInterpolatorStep)
		ps.interpolatorIndex[interpolatorId] = newInterpolator
		ps.Unlock()

		defer func() {
			ps.Lock()
			delete(ps.interpolatorIndex, interpolatorId)
			ps.Unlock()
		}()

		for {
			select {
			case <-ps.isDone():
				return
			case currentValue, more := <-newInterpolator.C:
				if more {
					ps.pipeline.SetFxPropFloat(payload.Name, payload.Property, currentValue)
				} else {
					return
				}
			}
		}
	}
}

func (ps *peerServer) loop() {
	// wait for interaction end
	go func() {
		select {
		case <-ps.i.isAborted():
			ps.ws.send("error-aborted")
			ps.close("interaction_aborted")
			return
		case <-ps.i.isDone():
			ps.ws.sendWithPayload("files", ps.i.files()) // peer could have left (ws closed) but interaction is still running
			ps.ws.send("end")
			ps.close("interaction_ended")
			return
		case <-ps.isDone():
			// user might have disconnected
			return
		}
	}()

	// sends "ending" message before interaction does end
	go func() {
		<-ps.i.isStarted()
		select {
		case <-time.After(time.Duration(ps.i.endingDelay()) * time.Second):
			// user might have reconnected and this ps could be
			ps.logInfo().Str("context", "peer").Msg("interaction_ending_sent")
			ps.ws.send("ending")
		case <-ps.isDone():
			// user might have disconnected
			return
		}
	}()

	for {
		m, err := ps.ws.receive()
		if err != nil {
			return
		}

		switch m.Kind {
		case "client_ice_candidate":
			if ps.pc.RemoteDescription() == nil {
				ps.pc.logError().Str("context", "signaling").Msg("remote_description_should_come_first")
			}

			candidate := webrtc.ICECandidateInit{}
			if err := json.Unmarshal([]byte(m.Payload), &candidate); err != nil {
				ps.logError().Str("context", "signaling").Err(err).Msg("unmarshal_client_ice_candidate_failed")
				return
			}

			if err := ps.pc.AddICECandidate(candidate); err != nil {
				ps.logError().Str("context", "signaling").Err(err).Msg("server_add_client_ice_candidate_failed")
				return
			}
			ps.logDebug().Str("context", "signaling").Str("value", fmt.Sprintf("%+v", candidate)).Msg("server_add_client_ice_candidate")
		case "client_answer":
			answer := webrtc.SessionDescription{}
			if err := json.Unmarshal([]byte(m.Payload), &answer); err != nil {
				ps.logError().Str("context", "signaling").Err(err).Msg("unmarshal_client_answer_failed")
				return
			}

			if err := ps.pc.SetRemoteDescription(answer); err != nil {
				ps.logError().Str("context", "signaling").Err(err).Msg("server_set_remote_description_failed")
				return
			}
			ps.logDebug().Str("context", "signaling").Str("user", ps.userId).Str("answer", fmt.Sprintf("%v", answer)).Msg("server_set_remote_description")
		case "client_negotiation_needed":
			ps.shareOffer(m.Kind, false)
			// previously for all: go ps.i.mixer.managedSignalingForEveryone("client_negotiation_needed", false)
		case "client_ice_connection_state_disconnected":
			ps.shareOffer(m.Kind, true)
		case "client_selected_candidate_pair":
			ps.logDebug().Str("context", "signaling").Str("source", "client").Str("value", m.Payload).Msg(m.Kind)
		case "client_control":
			payload := controlPayload{}
			if err := json.Unmarshal([]byte(m.Payload), &payload); err != nil {
				ps.logError().Str("context", "peer").Err(err).Msg("unmarshal_client_control_failed")
			} else {
				payload.fromUserId = ps.userId
				if targetPs, ok := ps.i.peerServerIndex[payload.UserId]; ok { // control other ps in same interaction
					go targetPs.controlFx(payload)
				} else { // default case: control self ps
					go ps.controlFx(payload)
				}
			}
		case "client_polycontrol":
			payload := polyControlPayload{}
			if err := json.Unmarshal([]byte(m.Payload), &payload); err != nil {
				ps.logError().Str("context", "peer").Err(err).Msg("unmarshal_client_polycontrol_failed")
			} else {
				go func() {
					ps.pipeline.SetFxPolyProp(payload.Name, payload.Property, payload.Kind, payload.Value)
					ps.logInfo().
						Str("context", "track").
						Str("name", payload.Name).
						Str("property", payload.Property).
						Str("kind", payload.Kind).
						Str("value", payload.Value).
						Msg("client_fx_control")
				}()
			}
		case "client_video_resolution_updated":
			ps.logDebug().Str("context", "track").Str("source", "client").Str("value", m.Payload).Str("unit", "pixels").Msg(m.Kind)
			if env.GeneratePlots {
				ps.videoSlice.plot.AddResolution(m.Payload)
			}
		case "client_video_fps_updated":
			ps.logDebug().Str("context", "track").Str("source", "client").Str("value", m.Payload).Msg(m.Kind)
			if env.GeneratePlots {
				ps.videoSlice.plot.AddFramerate(m.Payload)
			}
		case "client_keyframe_encoded_count_updated":
			ps.logDebug().Str("context", "track").Str("source", "client").Str("value", m.Payload).Msg(m.Kind)
			if env.GeneratePlots {
				ps.videoSlice.plot.AddKeyFrame()
			}
		case "stop":
			ps.close("client_stop_request")
		default:
			if strings.HasPrefix(m.Kind, "client_") {
				if strings.Contains(m.Kind, "count") {
					if count, err := strconv.ParseInt(m.Payload, 10, 64); err == nil {
						// "count" logs refer to track context
						ps.logDebug().Str("context", "track").Str("source", "client").Int64("value", count).Msg(m.Kind)
					}
				} else {
					ps.logDebug().Str("context", "peer").Str("source", "client").Str("value", m.Payload).Msg(m.Kind)
				}
			} else if strings.HasPrefix(m.Kind, "ext_") {
				ps.logDebug().
					Str("context", "ext").
					Str("source", "client").
					Str("payload", m.Payload).
					Msg(m.Kind)
			}
		}
	}
}

// API

// handle incoming websockets
func RunPeerServer(origin, href string, unsafeConn ws.IGorilla) {

	ws := newWsConn(unsafeConn)
	defer ws.Close()

	// first message must be a join request
	log.Info().Str("context", "peer").Msg("peer_server_waiting_for_join_payload")

	joinCh := make(chan types.JoinPayload)
	go func() {
		joinPayload, err := ws.readJoin(origin)
		if err != nil {
			log.Error().Str("context", "signaling").Err(err).Msg("join_payload_corrupted")
			return
		}
		log.Info().Str("context", "peer").Str("href", href).Str("userId", joinPayload.UserId).Msg("join_payload_ok")
		joinCh <- joinPayload
	}()

	select {
	case <-time.After(maxWaitingForJoin):
		log.Error().Str("context", "signaling").Msg("join_payload_too_late")
		ws.Close()
	case joinPayload := <-joinCh:
		// user might have disconnected
		userId := joinPayload.UserId
		namespace := joinPayload.Namespace
		interactionName := joinPayload.InteractionName

		i, msg, err := interactionStoreSingleton.join(joinPayload)
		if err != nil {
			// joinInteraction err is meaningful to client
			ws.send(fmt.Sprintf("error-%s", err))
			log.Error().Str("context", "signaling").Err(err).Str("namespace", namespace).Str("interaction", interactionName).Str("user", userId).Msg("join_failed")
			return
		}
		uniqueUserId := i.id + "#" + userId
		iceServers := iceservers.GetICEServers(uniqueUserId)
		ws.sendWithPayload("joined", struct {
			Context    string             `json:"context"`
			IceServers []webrtc.ICEServer `json:"iceServers"`
		}{
			msg,
			iceServers,
		})

		pc, err := newPeerConn(joinPayload, i)

		if err != nil {
			ws.send("error-peer-connection")
			i.logger.Error().Str("context", "peer").Err(err).Str("namespace", namespace).Str("interaction", interactionName).Str("user", userId).Msg("create_pc_failed")
			return
		}

		ps := newPeerServer(joinPayload, i, pc, ws)
		ws.setLogger(i.logger)
		ps.loop() // blocking
	}
}
