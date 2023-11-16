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

func (m *mixer) logInfo() *zerolog.Event {
	return m.i.logger.Info().Str("context", "signaling")
}

// Add to list of tracks
func (m *mixer) indexMixerSlice(ms *mixerSlice) {
	m.Lock()
	defer m.Unlock()

	m.sliceIndex[ms.ID()] = ms
	m.logInfo().Str("track", ms.ID()).Str("from", ms.fromPs.userId).Str("kind", ms.kind).Msg("out_track_indexed")
}

// Remove from list of tracks and fire renegotation for all PeerConnections
func (m *mixer) removeMixerSlice(ms *mixerSlice) {
	m.Lock()
	delete(m.sliceIndex, ms.ID())
	ms.logInfo().Str("track", ms.ID()).Str("from", ms.fromPs.userId).Str("kind", ms.kind).Msg("out_track_unindexed")
	m.Unlock()
}

// Signaling is split in three steps:
// - clean unused tracks on peer connections (other user has disconnected)
// - add tracks from other users
// - share offer with client
func (m *mixer) updateSignaling(cause string) bool {
	// lock for peerServerIndex
	m.i.RLock()
	defer m.i.RUnlock()

	for _, ps := range m.i.peerServerIndex {
		if !ps.updateTracksAndShareOffer(cause) {
			return false
		}
	}
	return true
}

// Update each PeerConnection so that it is getting all the expected media tracks
func (m *mixer) managedSignalingForEveryone(cause string, withPLI bool) {
	m.Lock()

	defer func() {
		m.Unlock()
		if withPLI {
			go m.dispatchInteractionPLI(cause)
		}
	}()

	m.logInfo().Str("cause", cause).Msg("managed_signaling_for_everyone_requested")

	if !m.i.deleted {
		ok := m.updateSignaling(cause)
		if ok {
			return
		} else {
			go func() {
				time.Sleep(time.Second * 2)
				m.managedSignalingForEveryone(cause+"_restart_with_delay", withPLI)
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
		ps.pc.managedPLIRequest(cause)
	}
}
