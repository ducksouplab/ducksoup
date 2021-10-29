package sfu

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
)

type mixer struct {
	sync.RWMutex
	room       *trialRoom
	sliceIndex map[string]*mixerSlice // per remote track id
}

type signalingState int

const (
	signalingOk signalingState = iota
	signalingRetryNow
	signalingRetryWithDelay
)

func (m *mixer) inspect() interface{} {
	output := make(map[string]interface{})
	for _, slice := range m.sliceIndex {
		output[slice.ID()] = slice.inspect()
	}
	return output
}

// mixer

func newMixer(room *trialRoom) *mixer {
	return &mixer{
		room:       room,
		sliceIndex: map[string]*mixerSlice{},
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
	for userId, ps := range m.room.peerServerIndex {
		// iterate to update peer connections of each PeerServer
		pc := ps.pc

		if pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
			delete(m.room.peerServerIndex, userId)
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
					log.Printf("[error] [room#%s] [user#%s] [mixer] can't RemoveTrack#%s:\n%v\n", m.room.shortId, userId, sentTrackId, err)
				} else {
					log.Printf("[info] [room#%s] [user#%s] [mixer] RemoveTrack#%s\n", m.room.shortId, userId, sentTrackId)
				}
			}
		}

		// add all necessary track (not yet to the PeerConnection or not coming from same peer)
		for id, s := range m.sliceIndex {
			_, alreadySent := alreadySentIndex[id]

			trackId := s.ID()
			if alreadySent {
				// don't double send
				log.Printf("[info] [room#%s] [user#%s] [mixer] [already] skip AddTrack: %s\n", m.room.shortId, userId, trackId)
			} else if m.room.size != 1 && s.fromPs.userId == userId {
				// don't send own tracks, except when room size is 1 (room then acts as a mirror)
				log.Printf("[info] [room#%s] [user#%s] [mixer] [own] skip AddTrack: %s\n", m.room.shortId, userId, trackId)
			} else {
				sender, err := pc.AddTrack(s.output)
				if err != nil {
					log.Printf("[error] [room#%s] [user#%s] [mixer] can't AddTrack#%s: %v\n", m.room.shortId, userId, id, err)
					return signalingRetryNow
				} else {
					log.Printf("[info] [room#%s] [user#%s] [mixer] AddTrack#%s\n", m.room.shortId, userId, trackId)
				}

				s.addSender(sender, userId)
			}
		}
	}
	return signalingOk
}

func (m *mixer) updateOffers() signalingState {
	for _, ps := range m.room.peerServerIndex {
		pc := ps.pc

		log.Printf("[info] [room#%s] [user#%s] [mixer] signaling state: %v\n", m.room.shortId, ps.userId, pc.SignalingState())

		offer, err := pc.CreateOffer(nil)
		if err != nil {
			log.Printf("[error] [room#%s] [user#%s] [mixer] can't CreateOffer: %v\n", m.room.shortId, ps.userId, err)
			return signalingRetryNow
		}

		if pc.PendingLocalDescription() != nil {
			log.Printf("[error] [room#%s] [user#%s] [mixer] pending local description\n", m.room.shortId, ps.userId)
		}

		if err = pc.SetLocalDescription(offer); err != nil {
			log.Printf("[error] [room#%s] [user#%s] [mixer] can't SetLocalDescription: %v\n", m.room.shortId, ps.userId, err)
			//log.Printf("\n\n\n---- failing local descripting:\n%v\n\n\n", offer)
			return signalingRetryWithDelay
		}

		offerString, err := json.Marshal(offer)
		if err != nil {
			log.Printf("[error] [room#%s] [user#%s] [mixer] can't marshal offer: %v\n", m.room.shortId, ps.userId, err)
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

	log.Printf("[info] [room#%s] [mixer] signaling update, reason: %s\n", m.room.shortId, reason)

	for {
		select {
		case <-m.room.endCh:
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

	for _, ps := range m.room.peerServerIndex {
		ps.pc.forcedPLIRequest()
	}
}
