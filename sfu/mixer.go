package sfu

import (
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

// Add to list of tracks
func (m *mixer) newMixerSliceFromRemote(ps *peerServer, remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) (slice *mixerSlice, err error) {
	slice, err = newMixerSlice(ps, remoteTrack, receiver)

	if err == nil {
		m.Lock()
		m.sliceIndex[slice.ID()] = slice
		m.logInfo().Str("track", slice.ID()).Str("from", slice.fromPs.userId).Str("kind", slice.kind).Msg("out_track_indexed")
		m.Unlock()
	}
	return
}

// Remove from list of tracks and fire renegotation for all PeerConnections
func (m *mixer) removeMixerSlice(s *mixerSlice) {
	m.Lock()
	delete(m.sliceIndex, s.ID())
	s.logInfo().Str("track", s.ID()).Str("from", s.fromPs.userId).Str("kind", s.kind).Msg("out_track_unindexed")
	m.Unlock()
}

// Signaling is split in three steps:
// - clean unused tracks on peer connections (other user has disconnected)
// - add tracks from other ushers
// - share offer with client
func (m *mixer) updateSignaling() bool {
	// lock for peerServerIndex
	m.r.Lock()
	defer m.r.Unlock()

	for _, ps := range m.r.peerServerIndex {
		if !ps.updateTracksAndShareOffer() {
			return false
		}
	}
	return true
}

// Update each PeerConnection so that it is getting all the expected media tracks
func (m *mixer) managedGlobalSignaling(cause string, withPLI bool) {
	m.Lock()

	defer func() {
		m.Unlock()
		if withPLI {
			go m.dispatchRoomPLI(cause)
		}
	}()

	m.logInfo().Str("cause", cause).Msg("signaling_update_requested")

	if !m.r.deleted {
		ok := m.updateSignaling()
		if ok {
			return
		} else {
			go func() {
				time.Sleep(time.Second * 2)
				m.managedGlobalSignaling(cause+"_restart_with_delay", withPLI)
			}()
			return
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
