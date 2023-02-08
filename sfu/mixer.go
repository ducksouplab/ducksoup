package sfu

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/rs/zerolog"
)

type mixer struct {
	sync.RWMutex
	sliceIndex map[string]*mixerSlice // per remote track id
	r          *room
}

type signalingState int

const (
	signalingOk signalingState = iota
	signalingRetryNow
	signalingRetryWithDelay
)

// mixer

func newMixer(r *room) *mixer {
	return &mixer{
		sliceIndex: map[string]*mixerSlice{},
		r:          r,
	}
}

func (m *mixer) logError() *zerolog.Event {
	return m.r.logger.Error().Str("context", "signaling")
}

func (m *mixer) logInfo() *zerolog.Event {
	return m.r.logger.Info().Str("context", "signaling")
}

func (m *mixer) logDebug() *zerolog.Event {
	return m.r.logger.Debug().Str("context", "signaling")
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

func (m *mixer) prepareOutTracks() signalingState {
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
			// if we have a RTPSender that doesn't map to an existing track remove and signal
			_, ok := m.sliceIndex[sentTrackId]
			if !ok {
				if err := pc.RemoveTrack(sender); err != nil {
					m.logError().Err(err).Str("user", userId).Str("track", sentTrackId).Msg("remove_track_failed")
				} else {
					m.logInfo().Str("user", userId).Str("track", sentTrackId).Msg("track_removed")
				}
			} else {
				alreadySentIndex[sentTrackId] = true
			}
		}

		// add all necessary track (not yet to the PeerConnection or not coming from same peer)
		for id, s := range m.sliceIndex {
			_, alreadySent := alreadySentIndex[id]

			trackId := s.ID()
			fromId := s.fromPs.userId
			if alreadySent {
				// don't double send
				m.logInfo().Str("user", userId).Str("from", fromId).Str("track", trackId).Msg("add_dup_track_to_pc_skipped")
			} else if m.r.size != 1 && s.fromPs.userId == userId {
				// don't send own tracks, except when room size is 1 (room then acts as a mirror)
				m.logInfo().Str("user", userId).Str("from", fromId).Str("track", trackId).Msg("add_own_track_to_pc_skipped")
			} else {
				sender, err := pc.AddTrack(s.output)
				if err != nil {
					m.logError().Err(err).Str("user", userId).Str("from", fromId).Str("track", trackId).Msg("add_out_track_to_pc_failed")
					return signalingRetryNow
				} else {
					m.logInfo().Str("user", userId).Str("from", fromId).Str("track", trackId).Msg("out_track_added_to_pc")
				}

				s.addSender(sender, userId)
			}
		}
	}
	return signalingOk
}

func (m *mixer) createOffers() signalingState {
	for _, ps := range m.r.peerServerIndex {
		userId := ps.userId
		pc := ps.pc

		m.logInfo().Str("user", userId).Str("current_state", pc.SignalingState().String()).Msg("offer_update_requested")

		if pc.PendingLocalDescription() != nil {
			m.logError().Str("user", userId).Msg("pending_local_description_blocks_offer")
			return signalingRetryWithDelay
		}

		offer, err := pc.CreateOffer(nil)
		if err != nil {
			m.logError().Str("user", userId).Msg("create_offer_failed")
			return signalingRetryNow
		}

		if err = pc.SetLocalDescription(offer); err != nil {
			m.logError().Str("user", userId).Str("sdp", offer.SDP).Err(err).Msg("set_local_description_failed")
			return signalingRetryWithDelay
		} else {
			m.logDebug().Str("user", userId).Str("offer", fmt.Sprintf("%v", offer)).Msg("set_local_description")
		}

		offerString, err := json.Marshal(offer)
		if err != nil {
			m.logError().Str("user", userId).Err(err).Msg("marshal_offer_failed")
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
	if s := m.prepareOutTracks(); s != signalingOk {
		return s
	}
	return m.createOffers()
}

// Update each PeerConnection so that it is getting all the expected media tracks
func (m *mixer) managedUpdateSignaling(cause string, withPLI bool) {
	m.Lock()
	defer func() {
		m.Unlock()
		if withPLI {
			go m.dispatchRoomPLI(cause)
		}
	}()

	m.logInfo().Str("cause", cause).Msg("signaling_update_requested")

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
						m.managedUpdateSignaling("asked restart with delay", withPLI)
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
							m.managedUpdateSignaling("restarted after too many tries", withPLI)
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
func (m *mixer) dispatchRoomPLI(cause string) {
	m.RLock()
	defer m.RUnlock()

	for _, ps := range m.r.peerServerIndex {
		ps.pc.throttledPLIRequest(cause)
	}
}
