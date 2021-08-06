package sfu

import (
	"encoding/json"
	"io"
	"log"
	"sync"

	"github.com/creamlab/ducksoup/gst"
	"github.com/pion/webrtc/v3"
)

// expanding the mixer metaphor, a (channel) strip holds everything related to a given signal (output track, GStreamer pipeline...)
type strip struct {
	sync.Mutex
	track        *webrtc.TrackLocalStaticRTP
	pipeline     *gst.Pipeline
	maxRateIndex map[string]uint64 // maxRate per peerConn
}

// mixer is not guarded by a mutex since all interaction is done through room
type mixer struct {
	shortId    string            // room's shortId used for logging
	stripIndex map[string]*strip // per track id
}

type success bool

func newMixer(shortId string) *mixer {
	return &mixer{
		shortId:    shortId,
		stripIndex: map[string]*strip{},
	}
}

// Add to list of tracks and fire renegotation for all PeerConnections
func (m *mixer) newOutTrack(c webrtc.RTPCodecCapability, id, streamID string) *webrtc.TrackLocalStaticRTP {
	// Create a new TrackLocal with the same codec as the incoming one
	track, err := webrtc.NewTrackLocalStaticRTP(c, id, streamID)

	if err != nil {
		log.Printf("[room %s error] NewTrackLocalStaticRTP: %v\n", m.shortId, err)
		panic(err)
	}

	m.stripIndex[id] = &strip{
		track:        track,
		maxRateIndex: map[string]uint64{},
	}
	return track
}

// Remove from list of tracks and fire renegotation for all PeerConnections
func (m *mixer) removeOutTrack(id string) {
	delete(m.stripIndex, id)
}

func (m *mixer) bindPipeline(id string, pipeline *gst.Pipeline) {
	m.stripIndex[id].pipeline = pipeline
}

func (m *mixer) updateTracks(room *trialRoom) success {
	for userId, ps := range room.peerServerIndex {
		// iterate to update peer connections of each PeerServer
		pc := ps.pc

		if pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
			delete(room.peerServerIndex, userId)
			break
		}

		// map of sender we are already sending, so we don't double send
		existingSenders := map[string]bool{}

		for _, sender := range pc.GetSenders() {
			if sender.Track() == nil {
				continue
			}

			existingSenders[sender.Track().ID()] = true

			// if we have a RTPSender that doesn't map to an existing track remove and signal
			_, ok := m.stripIndex[sender.Track().ID()]
			if !ok {
				if err := pc.RemoveTrack(sender); err != nil {
					log.Printf("[room %s error] RemoveTrack: %v\n", m.shortId, err)
				}
			}
		}

		// when room size is 1, it acts as a mirror
		if room.size != 1 {
			// don't receive videos we are sending, make sure we don't have loopback (remote peer point of view)
			for _, receiver := range pc.GetReceivers() {
				if receiver.Track() == nil {
					continue
				}
				existingSenders[receiver.Track().ID()] = true
			}
		}

		// add all track we aren't sending yet to the PeerConnection
		for id, strip := range m.stripIndex {
			if _, exists := existingSenders[id]; !exists {
				rtpSender, err := pc.AddTrack(strip.track)

				if err != nil {
					log.Printf("[room %s error] pc.AddTrack: %v\n", m.shortId, err)
					return false
				}

				// TODO check if needed
				// Read incoming RTCP packets
				// Before these packets are returned they are processed by interceptors. For things
				// like NACK this needs to be called.
				go func() {
					rtcpBuf := make([]byte, 1500)
					for {
						if _, _, err := rtpSender.Read(rtcpBuf); err != nil {
							// EOF is an acceptable termination of this goroutine
							if err != io.EOF {
								log.Printf("[room %s error] read rtpSender: %v\n", m.shortId, err)
							}
							return
						}
					}
				}()
			}
		}
	}
	return true
}

func (m *mixer) updateSignaling(room *trialRoom) success {
	for _, ps := range room.peerServerIndex {

		pc := ps.pc

		offer, err := pc.CreateOffer(nil)
		if err != nil {
			log.Printf("[room %s error] CreateOffer: %v\n", m.shortId, err)
			return false
		}

		if err = pc.SetLocalDescription(offer); err != nil {
			log.Printf("[room %s error] SetLocalDescription: %v\n", m.shortId, err)
			//log.Printf("\n\n\n---- failing local descripting:\n%v\n\n\n", offer)
			return false
		}

		offerString, err := json.Marshal(offer)
		if err != nil {
			log.Printf("[room %s error] marshal offer: %v\n", m.shortId, err)
			return false
		}

		if err = ps.ws.SendWithPayload("offer", string(offerString)); err != nil {
			return false
		}
	}
	return true
}

// does two things (and ask to retry if false is returned):
// - add or remove tracks on peer connections
// - update signaling, a boolean controlling this step not to overdo it till every out track is ready
func (m *mixer) updatePeers(room *trialRoom, withSignaling bool) success {
	if s := m.updateTracks(room); !s {
		return false
	}

	if withSignaling {
		if s := m.updateSignaling(room); !s {
			return false
		}
	}

	return true
}
