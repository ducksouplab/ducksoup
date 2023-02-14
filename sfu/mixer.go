package sfu

import (
	"sync"
	"time"

	"github.com/rs/zerolog"
)

type mixer struct {
	sync.RWMutex
	sliceIndex map[string]*mixerSlice // per remote track id
	i          *interaction
}

// mixer

func newMixer(i *interaction) *mixer {
	return &mixer{
		sliceIndex: map[string]*mixerSlice{},
		i:          i,
	}
}

func (m *mixer) logError() *zerolog.Event {
	return m.i.logger.Error().Str("context", "signaling")
}

func (m *mixer) logInfo() *zerolog.Event {
	return m.i.logger.Info().Str("context", "signaling")
}

func (m *mixer) logDebug() *zerolog.Event {
	return m.i.logger.Debug().Str("context", "signaling")
}

// Add to list of tracks
func (m *mixer) indexMixerSlice(slice *mixerSlice) {
	m.Lock()
	defer m.Unlock()

	m.sliceIndex[slice.ID()] = slice
	m.logInfo().Str("track", slice.ID()).Str("from", slice.fromPs.userId).Str("kind", slice.kind).Msg("out_track_indexed")
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
	m.i.Lock()
	defer m.i.Unlock()

	for _, ps := range m.i.peerServerIndex {
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
			go m.dispatchInteractionPLI(cause)
		}
	}()

	m.logInfo().Str("cause", cause).Msg("signaling_update_requested")

	if !m.i.deleted {
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
func (m *mixer) dispatchInteractionPLI(cause string) {
	m.RLock()
	defer m.RUnlock()

	for _, ps := range m.i.peerServerIndex {
		ps.pc.throttledPLIRequest(cause)
	}
}
