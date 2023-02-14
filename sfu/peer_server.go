package sfu

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ducksouplab/ducksoup/gst"
	"github.com/ducksouplab/ducksoup/sequencing"
	"github.com/ducksouplab/ducksoup/types"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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
	closedCh        chan struct{}
	// processing
	pipeline          *gst.Pipeline
	interpolatorIndex map[string]*sequencing.LinearInterpolator
}

func newPeerServer(
	join types.JoinPayload,
	i *interaction,
	pc *peerConn,
	ws *wsConn) *peerServer {

	pipeline := gst.NewPipeline(join, i.filePrefix(join.UserId))

	ps := &peerServer{
		userId:            join.UserId,
		interactionName:   i.name,
		streamId:          uuid.New().String(),
		join:              join,
		i:                 i,
		pc:                pc,
		ws:                ws,
		closed:            false,
		closedCh:          make(chan struct{}),
		pipeline:          pipeline,
		interpolatorIndex: make(map[string]*sequencing.LinearInterpolator),
	}

	// connect for further communication
	i.connectPeerServer(ps)
	if i.allUsersConnected() {
		// optim: update tracks from others (all are there) and share offer
		ps.updateTracksAndShareOffer()
	} else {
		// ready to share offer, in(coming) tracks already prepared during PC initialization
		ps.shareOffer()
	}
	// some events on pc needs API from ws or interaction
	pc.handleCallbacks(ps)
	return ps
}

func (ps *peerServer) logError() *zerolog.Event {
	return ps.i.logger.Error().Str("context", "signaling").Str("user", ps.userId)
}

func (ps *peerServer) logInfo() *zerolog.Event {
	return ps.i.logger.Info().Str("context", "signaling").Str("user", ps.userId)
}

func (ps *peerServer) logDebug() *zerolog.Event {
	return ps.i.logger.Debug().Str("context", "signaling").Str("user", ps.userId)
}

func (ps *peerServer) setMixerSlice(kind string, slice *mixerSlice) {
	if kind == "audio" {
		ps.audioSlice = slice
	} else if kind == "video" {
		ps.videoSlice = slice
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
				ps.logError().Err(err).Str("user", userId).Str("track", sentTrackId).Msg("remove_track_failed")
			} else {
				ps.logInfo().Str("user", userId).Str("track", sentTrackId).Msg("track_removed")
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
				ps.logError().Err(err).Str("user", userId).Str("from", fromId).Str("track", trackId).Msg("add_out_track_to_pc_failed")
				return false
			} else {
				ps.logInfo().Str("user", userId).Str("from", fromId).Str("track", trackId).Msg("out_track_added_to_pc")
			}
			s.addSender(pc, sender)
		}
	}
	return true
}

func (ps *peerServer) shareOffer() bool {
	userId := ps.userId
	pc := ps.pc

	ps.logInfo().Str("user", userId).Str("current_state", pc.SignalingState().String()).Msg("offer_update_requested")

	if pc.PendingLocalDescription() != nil {
		ps.logError().Str("user", userId).Msg("pending_local_description_blocks_offer")
		return false
	}

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		ps.logError().Str("user", userId).Msg("create_offer_failed")
		return false
	}

	if err = pc.SetLocalDescription(offer); err != nil {
		ps.logError().Str("user", userId).Str("sdp", offer.SDP).Err(err).Msg("set_local_description_failed")
		return false
	} else {
		ps.logDebug().Str("user", userId).Str("offer", fmt.Sprintf("%v", offer)).Msg("set_local_description")
	}

	offerString, err := json.Marshal(offer)
	if err != nil {
		ps.logError().Str("user", userId).Err(err).Msg("marshal_offer_failed")
		return false
	}

	if err = ps.ws.sendWithPayload("offer", string(offerString)); err != nil {
		return false
	}
	return true
}

func (ps *peerServer) updateTracksAndShareOffer() bool {
	ps.cleanOutTracks()
	if state := ps.prepareOutTracks(); !state {
		return state
	}
	if state := ps.shareOffer(); !state {
		return state
	}
	return true
}

func (ps *peerServer) close(cause string) {
	ps.Lock()
	defer ps.Unlock()

	if !ps.closed {
		// ps.closed check ensure closedCh is not closed twice
		ps.closed = true

		// listened by mixerSlices
		close(ps.closedCh)
		// clean up bound components
		ps.pc.Close()
		ps.ws.Close()

		ps.logInfo().Str("context", "peer").Str("cause", cause).Msg("peer_server_ended")
	}
	// cleanup anyway
	ps.i.disconnectUser(ps.userId)
}

func (ps *peerServer) controlFx(payload controlPayload) {
	interpolatorId := payload.Name + payload.Property
	interpolator := ps.interpolatorIndex[interpolatorId]

	if interpolator != nil {
		// an interpolation is already running for this pipeline, effect and property
		interpolator.Stop()
	}

	ps.logInfo().
		Str("context", "track").
		Str("name", payload.Name).
		Str("property", payload.Property).
		Float32("value", payload.Value).
		Int("duration", payload.Duration).
		Msg("client_fx_control")

	duration := payload.Duration
	if duration == 0 {
		ps.pipeline.SetFxProp(payload.Name, payload.Property, payload.Value)
	} else {
		if duration > maxInterpolatorDuration {
			duration = maxInterpolatorDuration
		}
		oldValue := ps.pipeline.GetFxProp(payload.Name, payload.Property)
		newInterpolator := sequencing.NewLinearInterpolator(oldValue, payload.Value, duration, defaultInterpolatorStep)

		ps.Lock()
		ps.interpolatorIndex[interpolatorId] = newInterpolator
		ps.Unlock()

		defer func() {
			ps.Lock()
			delete(ps.interpolatorIndex, interpolatorId)
			ps.Unlock()
		}()

		for {
			select {
			case <-ps.i.endCh:
				return
			case <-ps.closedCh:
				return
			case currentValue, more := <-newInterpolator.C:
				if more {
					ps.pipeline.SetFxProp(payload.Name, payload.Property, currentValue)
				} else {
					return
				}
			}
		}
	}
}

func (ps *peerServer) loop() {

	// sends "ending" message before interaction does end
	go func() {
		<-ps.i.waitForAllCh
		select {
		case <-time.After(time.Duration(ps.i.endingDelay()) * time.Second):
			// user might have reconnected and this ps could be
			ps.logInfo().Str("context", "peer").Msg("interaction_ending_sent")
			ps.ws.send("ending")
		case <-ps.closedCh:
			// user might have disconnected
			return
		}
	}()

	// wait for interaction end
	go func() {
		select {
		case <-ps.i.endCh:
			ps.ws.sendWithPayload("files", ps.i.files()) // peer could have left (ws closed) but interaction is still running
			ps.close("interaction ended")
		case <-ps.closedCh:
			// user might have disconnected
			return
		}
	}()

	for {
		m, err := ps.ws.read()
		if err != nil {
			ps.close(err.Error())
			return
		}

		switch m.Kind {
		case "client_candidate":
			if ps.pc.RemoteDescription() == nil {
				ps.pc.logError().Msg("remote_description_should_come_first")
			}

			candidate := webrtc.ICECandidateInit{}
			if err := json.Unmarshal([]byte(m.Payload), &candidate); err != nil {
				ps.logError().Err(err).Msg("unmarshal_candidate_failed")
				return
			}

			if err := ps.pc.AddICECandidate(candidate); err != nil {
				ps.logError().Err(err).Msg("add_candidate_failed")
				return
			}
			ps.logDebug().Str("value", fmt.Sprintf("%+v", candidate)).Msg("client_candidate_added")
		case "client_answer":
			answer := webrtc.SessionDescription{}
			if err := json.Unmarshal([]byte(m.Payload), &answer); err != nil {
				ps.logError().Err(err).Msg("unmarshal_answer_failed")
				return
			}

			if err := ps.pc.SetRemoteDescription(answer); err != nil {
				ps.logError().Err(err).Msg("set_remote_description_failed")
				return
			}
			ps.logDebug().Str("user", ps.userId).Str("answer", fmt.Sprintf("%v", answer)).Msg("set_remote_description")
		case "client_control":
			payload := controlPayload{}
			if err := json.Unmarshal([]byte(m.Payload), &payload); err != nil {
				ps.logError().Err(err).Msg("unmarshal_control_failed")
			} else {
				go func() {
					ps.controlFx(payload)
				}()
			}
		case "client_polycontrol":
			payload := polyControlPayload{}
			if err := json.Unmarshal([]byte(m.Payload), &payload); err != nil {
				ps.logError().Err(err).Msg("unmarshal_polycontrol_failed")
			} else {
				go func() {
					ps.pipeline.SetFxPolyProp(payload.Name, payload.Property, payload.Kind, payload.Value)
					ps.logInfo().
						Str("context", "track").
						Str("name", payload.Name).
						Str("property", payload.Property).
						Str("value", payload.Value).
						Int("duration", payload.Duration).
						Msg("client_fx_control")
				}()
			}
		case "client_video_resolution_updated":
			ps.logDebug().Str("source", "client").Str("value", m.Payload).Str("unit", "pixels").Msg(m.Kind)
		default:
			if strings.HasPrefix(m.Kind, "client_") {
				if strings.Contains(m.Kind, "count") {
					if count, err := strconv.ParseInt(m.Payload, 10, 64); err == nil {
						// "count" logs refer to track context
						ps.logDebug().Str("context", "track").Str("source", "client").Int64("value", count).Msg(m.Kind)
					}
				} else {
					ps.logDebug().Str("source", "client").Str("value", m.Payload).Msg(m.Kind)
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
func RunPeerServer(origin string, unsafeConn *websocket.Conn) {

	ws := newWsConn(unsafeConn)
	defer ws.Close()

	// first message must be a join request
	joinPayload, err := ws.readJoin(origin)
	if err != nil {
		ws.send("error-join")
		log.Error().Str("context", "signaling").Err(err).Msg("join payload corrupted")
		return
	}

	userId := joinPayload.UserId
	namespace := joinPayload.Namespace
	interactionName := joinPayload.Name

	r, err := interactionStoreSingleton.join(joinPayload)
	if err != nil {
		// joinInteraction err is meaningful to client
		ws.send(fmt.Sprintf("error-%s", err))
		log.Error().Str("context", "signaling").Err(err).Str("namespace", namespace).Str("interaction", interactionName).Str("user", userId).Msg("join failed")
		return
	}

	pc, err := newPeerConn(joinPayload, r)
	if err != nil {
		ws.send("error-peer-connection")
		log.Error().Str("context", "peer").Err(err).Str("namespace", namespace).Str("interaction", interactionName).Str("user", userId).Msg("create_pc_failed")
		return
	}

	ps := newPeerServer(joinPayload, r, pc, ws)

	log.Info().Str("context", "peer").Str("namespace", namespace).Str("interaction", interactionName).Str("user", userId).Msg("peer_server_started")

	ps.loop() // blocking
}
