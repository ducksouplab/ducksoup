package sfu

import (
	"encoding/json"
	"sync"
	"time"

	_ "github.com/creamlab/ducksoup/helpers" // rely on helpers logger init side-effect
	"github.com/pion/webrtc/v3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type mixer struct {
	sync.RWMutex
	sliceIndex map[string]*mixerSlice // per remote track id
	// room
	r *room
	// log
	logger zerolog.Logger
}

type signalingState int

const (
	signalingOk signalingState = iota
	signalingRetryNow
	signalingRetryWithDelay
)

// mixer

func newMixer(r *room) *mixer {
	logger := log.With().
		Str("room", r.id).
		Logger()

	return &mixer{
		sliceIndex: map[string]*mixerSlice{},
		r:          r,
		logger:     logger,
	}
}

// Add to list of tracks and fire renegotation for all PeerConnections
func (m *mixer) newMixerSliceFromRemote(ps *peerServer, remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) (slice *mixerSlice, err error) {
	slice, err = newMixerSlice(ps, remoteTrack, receiver)

	if err == nil {
		m.Lock()
		m.sliceIndex[slice.ID()] = slice
		m.Unlock()
	}
	return
}

// Remove from list of tracks and fire renegotation for all PeerConnections
func (m *mixer) removeMixerSlice(s *mixerSlice) {
	m.Lock()
	delete(m.sliceIndex, s.ID())
	m.Unlock()
}

func (m *mixer) updateTracks() signalingState {
	for userId, ps := range m.r.peerServerIndex {
		// iterate to update peer connections of each PeerServer
		pc := ps.pc

		if pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
			m.r.disconnectUser(userId)
			break
		}

		// map of sender we are already sending, so we don't double send
		alreadySentIndex := map[string]bool{}

		for _, sender := range pc.GetSenders() {
			if sender.Track() == nil {
				continue
			}

			sentTrackId := sender.Track().ID()
			alreadySentIndex[sentTrackId] = true

			// if we have a RTPSender that doesn't map to an existing track remove and signal
			_, ok := m.sliceIndex[sentTrackId]
			if !ok {
				if err := pc.RemoveTrack(sender); err != nil {
					m.logger.Error().Err(err).Str("user", userId).Str("track", sentTrackId).Msg("[mixer] can't remove sent track")
				} else {
					m.logger.Info().Str("user", userId).Str("track", sentTrackId).Msg("[mixer] removed sent track")
				}
			}
		}

		// add all necessary track (not yet to the PeerConnection or not coming from same peer)
		for id, s := range m.sliceIndex {
			_, alreadySent := alreadySentIndex[id]

			trackId := s.ID()
			if alreadySent {
				// don't double send
				m.logger.Info().Str("user", userId).Str("track", trackId).Msg("[mixer] skip adding already sent track")
			} else if m.r.size != 1 && s.fromPs.userId == userId {
				// don't send own tracks, except when room size is 1 (room then acts as a mirror)
				m.logger.Info().Str("user", userId).Str("track", trackId).Msg("[mixer] skip adding own track")
			} else {
				sender, err := pc.AddTrack(s.output)
				if err != nil {
					m.logger.Error().Err(err).Str("user", userId).Str("track", trackId).Msg("[mixer] can't add track")
					return signalingRetryNow
				} else {
					m.logger.Info().Str("user", userId).Str("track", trackId).Msg("[mixer] track added")
				}

				s.addSender(sender, userId)
			}
		}
	}
	return signalingOk
}

func (m *mixer) updateOffers() signalingState {
	for _, ps := range m.r.peerServerIndex {
		userId := ps.userId
		pc := ps.pc

		m.logger.Info().Str("user", userId).Msgf("[mixer] signaling state: %v", pc.SignalingState())

		offer, err := pc.CreateOffer(nil)
		if err != nil {
			m.logger.Error().Str("user", userId).Msg("[mixer] can't create offer")
			return signalingRetryNow
		}

		if pc.PendingLocalDescription() != nil {
			m.logger.Error().Str("user", userId).Msg("[mixer] pending local description")
		}

		if err = pc.SetLocalDescription(offer); err != nil {
			m.logger.Error().Str("user", userId).Str("sdp", offer.SDP).Err(err).Msg("[mixer] can't set local description")
			return signalingRetryWithDelay
		}

		offerString, err := json.Marshal(offer)
		if err != nil {
			m.logger.Error().Str("user", userId).Err(err).Msg("[mixer] can't marshal offer")
			return signalingRetryNow
		}

		if err = ps.ws.sendWithPayload("offer", string(offerString)); err != nil {
			return signalingRetryNow
		}
	}
	return signalingOk
}

// Signaling is split in two steps:
// - add or remove tracks on peer connections
// - update and send offers
func (m *mixer) updateSignaling() signalingState {
	if s := m.updateTracks(); s != signalingOk {
		return s
	}
	return m.updateOffers()
}

// Update each PeerConnection so that it is getting all the expected media tracks
func (m *mixer) managedUpdateSignaling(reason string) {
	m.Lock()
	defer func() {
		m.Unlock()
		go m.dispatchKeyFrame()
	}()

	m.logger.Info().Str("reason", reason).Msg("[mixer] signaling update")

	for {
		select {
		case <-m.r.endCh:
			return
		default:
			for tries := 0; ; tries++ {
				state := m.updateSignaling()

				if state == signalingOk {
					// signaling is done
					return
				} else if state == signalingRetryWithDelay {
					go func() {
						time.Sleep(time.Second * 2)
						m.managedUpdateSignaling("asked restart with delay")
					}()
					return
				} else if state == signalingRetryNow {
					if tries < 20 {
						// redo signaling / for loop
						break
					} else {
						// we might be blocking a RemoveTrack or AddTrack
						go func() {
							time.Sleep(time.Second * 3)
							m.managedUpdateSignaling("restarted after too many tries")
						}()
						return
					}
				}
			}
		}
	}
}

// sends a keyframe to all PeerConnections, used everytime a new user joins the call
// (in that case, requesting a FullIntraRequest may be preferred/more accurate, over a PictureLossIndicator
// but the effect is probably the same)
func (m *mixer) dispatchKeyFrame() {
	m.RLock()
	defer m.RUnlock()

	for _, ps := range m.r.peerServerIndex {
		ps.pc.throttledPLIRequest()
	}
}
