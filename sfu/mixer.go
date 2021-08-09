package sfu

import (
	"encoding/json"
	"io"
	"log"
	"sync"
	"time"

	"github.com/creamlab/ducksoup/gst"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

// TODO this could belong to a configuration file
const DefaultVideoBitrate = 500 * 1000
const MinVideoBitrate = 125 * 1000
const MaxVideoBitrate = 750 * 1000
const DefaultAudioBitrate = 48 * 1000
const MinAudioBitrate = 16 * 1000
const MaxAudioBitrate = 64 * 1000

// mixer is not guarded by a mutex since all interaction is done through room
type mixer struct {
	shortId         string                 // room's shortId used for logging
	mixerSliceIndex map[string]*mixerSlice // per track id
}

// expanding the mixer metaphor, a mixerSliceholds everything related to a given signal (output track, GStreamer pipeline...)
// there is one remote/in track per mixerSlice, one local/out track, but possibly several senders/recepients of the local track (SFU behavior)
type mixerSlice struct {
	sync.Mutex
	localTrack            *webrtc.TrackLocalStaticRTP
	remotePC              *webrtc.PeerConnection
	remoteSSRC            webrtc.SSRC
	pipeline              *gst.Pipeline
	senderControllerIndex map[string]*senderController // per user id
	updateTicker          *time.Ticker
	endCh                 chan struct{} // stop processing when track is removed
}

type senderController struct {
	sync.Mutex
	ssrc    webrtc.SSRC
	kind    string
	sender  *webrtc.RTPSender
	maxRate uint64
	maxREMB uint64
}

type success bool

// mixerSlice

func minUint64Slice(v []uint64) (min uint64) {
	if len(v) > 0 {
		min = v[0]
	}
	for i := 1; i < len(v); i++ {
		if v[i] < min {
			min = v[i]
		}
	}
	return
}

func newMixerSlice(localTrack *webrtc.TrackLocalStaticRTP, remotePC *webrtc.PeerConnection, remoteSSRC webrtc.SSRC) *mixerSlice {
	updateTicker := time.NewTicker(1 * time.Second)

	ms := &mixerSlice{
		localTrack:            localTrack,
		remotePC:              remotePC,
		remoteSSRC:            remoteSSRC,
		senderControllerIndex: map[string]*senderController{},
		updateTicker:          updateTicker,
		endCh:                 make(chan struct{}),
	}

	// update encoding bitrate on tick and according to minimum controller rate
	go func() {
		for range updateTicker.C {
			if len(ms.senderControllerIndex) > 0 {
				controllerRates := []uint64{}
				for _, controller := range ms.senderControllerIndex {
					controllerRates = append(controllerRates, controller.maxRate)
					sliceRate := minUint64Slice(controllerRates)
					// if ms.localTrack.Kind().String() == "video" {
					// 	log.Printf("[debug] %v mixerSlice rate %v\n", ms.localTrack.Kind(), sliceRate)
					// }
					ms.pipeline.SetEncodingRate(sliceRate)
				}
			}
		}
	}()

	return ms
}

func (controller *senderController) updateRateFromREMB(remb uint64) {
	controller.Lock()
	defer controller.Unlock()

	controller.maxREMB = remb
	if controller.maxRate > remb {
		controller.maxRate = remb
	}
}

func (controller *senderController) updateRateFromLoss(loss uint8) {
	controller.Lock()
	defer controller.Unlock()

	var newMaxRate uint64
	prevMaxRate := controller.maxRate

	if loss < 5 {
		// loss < 0.02, multiply by 1.05
		newMaxRate = prevMaxRate * 269 / 256
		if controller.kind == "audio" {
			if newMaxRate > MaxAudioBitrate {
				newMaxRate = MaxAudioBitrate
			}
		} else {
			if newMaxRate > MaxVideoBitrate {
				newMaxRate = MaxVideoBitrate
			}
		}
	} else if loss > 25 {
		// loss > 0.1, multiply by (1 - loss/2)
		newMaxRate = prevMaxRate * (512 - uint64(loss)) / 512

		if controller.kind == "audio" {
			if newMaxRate < MinAudioBitrate {
				newMaxRate = MinAudioBitrate
			}
		} else {
			if newMaxRate < MinVideoBitrate {
				newMaxRate = MinVideoBitrate
			}
		}

	} else {
		newMaxRate = prevMaxRate
	}

	if newMaxRate > controller.maxREMB {
		newMaxRate = controller.maxREMB
	}
	controller.maxRate = newMaxRate
}

func (ms *mixerSlice) stop() {
	ms.updateTicker.Stop()
	close(ms.endCh)
}

func (ms *mixerSlice) runRTCPListener(controller *senderController, ssrc webrtc.SSRC, shortId string) {
	buf := make([]byte, 1500)

listenerLoop:
	for {
		select {
		case <-ms.endCh:
			break listenerLoop
		default:
			n, _, err := controller.sender.Read(buf)
			if err != nil {
				if err != io.EOF && err != io.ErrClosedPipe {
					log.Printf("[mixer %s error] read RTCP: %v\n", shortId, err)
				}
				return
			}
			packets, err := rtcp.Unmarshal(buf[:n])
			if err != nil {
				log.Printf("[mixer %s error] unmarshal RTCP: %v\n", shortId, err)
				continue
			}

			for _, packet := range packets {
				switch rtcpPacket := packet.(type) {
				case *rtcp.PictureLossIndication:
					err := ms.remotePC.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(ms.remoteSSRC)}})
					if err != nil {
						log.Printf("[mixer %s error] WriteRTCP PLI: %v\n", shortId, err)
					}
				case *rtcp.ReceiverEstimatedMaximumBitrate:
					controller.updateRateFromREMB(rtcpPacket.Bitrate)
				case *rtcp.ReceiverReport:
					for _, r := range rtcpPacket.Reports {
						if r.SSRC == uint32(ssrc) {
							controller.updateRateFromLoss(r.FractionLost)
						}
					}
					// default:
					// 	log.Printf("-- RTCP packet received: %T", packet)
				}
			}
		}
	}
}

// mixer

func newMixer(shortId string) *mixer {
	return &mixer{
		shortId:         shortId,
		mixerSliceIndex: map[string]*mixerSlice{},
	}
}

// Add to list of tracks and fire renegotation for all PeerConnections
func (m *mixer) newLocalTrackFromRemote(remoteTrack *webrtc.TrackRemote, remotePC *webrtc.PeerConnection) *webrtc.TrackLocalStaticRTP {
	// Create a new TrackLocal with the same codec as the incoming one
	track, err := webrtc.NewTrackLocalStaticRTP(remoteTrack.Codec().RTPCodecCapability, remoteTrack.ID(), remoteTrack.StreamID())

	if err != nil {
		log.Printf("[mixer %s error] NewTrackLocalStaticRTP: %v\n", m.shortId, err)
		panic(err)
	}

	m.mixerSliceIndex[remoteTrack.ID()] = newMixerSlice(track, remotePC, remoteTrack.SSRC())
	return track
}

// Remove from list of tracks and fire renegotation for all PeerConnections
func (m *mixer) removeLocalTrack(id string) {
	if ms, exists := m.mixerSliceIndex[id]; exists {
		ms.stop()
		delete(m.mixerSliceIndex, id)
	}
}

func (m *mixer) bindPipeline(id string, pipeline *gst.Pipeline) {
	m.mixerSliceIndex[id].pipeline = pipeline
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
			_, ok := m.mixerSliceIndex[sender.Track().ID()]
			if !ok {
				if err := pc.RemoveTrack(sender); err != nil {
					log.Printf("[mixer %s error] RemoveTrack: %v\n", m.shortId, err)
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
		for id, ms := range m.mixerSliceIndex {
			if _, exists := existingSenders[id]; !exists {
				sender, err := pc.AddTrack(ms.localTrack)

				if err != nil {
					log.Printf("[mixer %s error] pc.AddTrack: %v\n", m.shortId, err)
					return false
				}

				params := sender.GetParameters()
				if len(params.Encodings) == 1 {
					kind := ms.localTrack.Kind().String()
					ssrc := params.Encodings[0].SSRC
					defaultBitrate := DefaultVideoBitrate
					if kind == "audio" {
						defaultBitrate = DefaultAudioBitrate
					}
					controller := senderController{
						ssrc:    ssrc,
						kind:    kind,
						sender:  sender,
						maxRate: uint64(defaultBitrate),
						maxREMB: uint64(defaultBitrate),
					}
					ms.Lock()
					ms.senderControllerIndex[userId] = &controller
					ms.Unlock()

					go ms.runRTCPListener(&controller, ssrc, m.shortId)
				} else {
					log.Printf("[mixer %s error] wrong number of encoding parameters: %v\n", m.shortId, err)
				}

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
			log.Printf("[mixer %s error] CreateOffer: %v\n", m.shortId, err)
			return false
		}

		if err = pc.SetLocalDescription(offer); err != nil {
			log.Printf("[mixer %s error] SetLocalDescription: %v\n", m.shortId, err)
			//log.Printf("\n\n\n---- failing local descripting:\n%v\n\n\n", offer)
			return false
		}

		offerString, err := json.Marshal(offer)
		if err != nil {
			log.Printf("[mixer %s error] marshal offer: %v\n", m.shortId, err)
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
