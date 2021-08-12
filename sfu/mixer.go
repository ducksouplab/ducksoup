package sfu

import (
	"encoding/json"
	"io"
	"log"
	"sync"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

type mixer struct {
	sync.RWMutex
	room            *trialRoom
	shortId         string                 // room's shortId used for logging
	mixerSliceIndex map[string]*mixerSlice // per track id
}

type mixerSlice struct {
	sync.Mutex
	outputTrack           *localTrack
	receivingPC           *webrtc.PeerConnection // peer connection holding the incoming/remote track
	receiver              *webrtc.RTPReceiver
	remoteSSRC            webrtc.SSRC
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

type needsSignaling bool

// senderController

func (sc *senderController) updateRateFromREMB(remb uint64) {
	sc.Lock()
	defer sc.Unlock()

	sc.maxREMB = remb
	if sc.maxRate > remb {
		sc.maxRate = remb
	}
}

// see https://datatracker.ietf.org/doc/html/draft-ietf-rmcat-gcc-02
func (sc *senderController) updateRateFromLoss(loss uint8) {
	sc.Lock()
	defer sc.Unlock()

	var newMaxRate uint64
	prevMaxRate := sc.maxRate

	if loss < 5 {
		// loss < 0.02, multiply by 1.05
		newMaxRate = prevMaxRate * 269 / 256
		if sc.kind == "audio" {
			if newMaxRate > maxAudioBitrate {
				newMaxRate = maxAudioBitrate
			}
		} else {
			if newMaxRate > maxVideoBitrate {
				newMaxRate = maxVideoBitrate
			}
		}
	} else if loss > 25 {
		// loss > 0.1, multiply by (1 - loss/2)
		newMaxRate = prevMaxRate * (512 - uint64(loss)) / 512

		if sc.kind == "audio" {
			if newMaxRate < minAudioBitrate {
				newMaxRate = minAudioBitrate
			}
		} else {
			if newMaxRate < minVideoBitrate {
				newMaxRate = minVideoBitrate
			}
		}

	} else {
		newMaxRate = prevMaxRate
	}

	if newMaxRate > sc.maxREMB {
		newMaxRate = sc.maxREMB
	}
	sc.maxRate = newMaxRate
}

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

func newMixerSlice(shortId string, pc *peerConn, outputTrack *localTrack, receiver *webrtc.RTPReceiver, remoteSSRC webrtc.SSRC) *mixerSlice {
	updateTicker := time.NewTicker(1 * time.Second)

	ms := &mixerSlice{
		outputTrack:           outputTrack,
		receivingPC:           pc.PeerConnection,
		receiver:              receiver,
		remoteSSRC:            remoteSSRC,
		senderControllerIndex: map[string]*senderController{},
		updateTicker:          updateTicker,
		endCh:                 make(chan struct{}),
	}

	// update encoding bitrate on tick and according to minimum controller rate
	go func() {
		for range updateTicker.C {
			if len(ms.senderControllerIndex) > 0 {
				rates := []uint64{}
				for _, sc := range ms.senderControllerIndex {
					rates = append(rates, sc.maxRate)
				}
				sliceRate := minUint64Slice(rates)
				// if ms.localTrack.Kind().String() == "video" {
				// 	log.Printf("[debug] %v mixerSlice rate %v\n", ms.localTrack.Kind(), sliceRate)
				// }
				if ms.outputTrack.pipeline != nil && sliceRate > 0 {
					ms.outputTrack.pipeline.SetEncodingRate(sliceRate)
				}
			}
		}
	}()

	// TODO enable later if receiver RTCP packets are useful to parse
	// go ms.runReceiverListener(shortId)

	return ms
}

func (ms *mixerSlice) stop() {
	ms.updateTicker.Stop()
	close(ms.endCh)
}

func (ms *mixerSlice) runSenderListener(sc *senderController, ssrc webrtc.SSRC, shortId string) {
	buf := make([]byte, receiveMTU)

listenerLoop:
	for {
		select {
		case <-ms.endCh:
			break listenerLoop
		default:
			n, _, err := sc.sender.Read(buf)
			if err != nil {
				if err != io.EOF && err != io.ErrClosedPipe {
					log.Printf("[mixer %s error] sender read RTCP: %v\n", shortId, err)
				}
				return
			}
			packets, err := rtcp.Unmarshal(buf[:n])
			if err != nil {
				log.Printf("[mixer %s error] sender unmarshal RTCP: %v\n", shortId, err)
				continue
			}

			for _, packet := range packets {
				switch rtcpPacket := packet.(type) {
				case *rtcp.PictureLossIndication:
					err := ms.receivingPC.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(ms.remoteSSRC)}})
					if err != nil {
						log.Printf("[mixer %s error] WriteRTCP PLI: %v\n", shortId, err)
					}
				case *rtcp.ReceiverEstimatedMaximumBitrate:
					sc.updateRateFromREMB(rtcpPacket.Bitrate)
				case *rtcp.ReceiverReport:
					for _, r := range rtcpPacket.Reports {
						if r.SSRC == uint32(ssrc) {
							sc.updateRateFromLoss(r.FractionLost)
						}
					}
					// default:
					// 	log.Printf("-- RTCP packet on sender: %T", rtcpPacket)
				}
			}
		}
	}
}

// mixer

func newMixer(room *trialRoom) *mixer {
	return &mixer{
		room:            room,
		mixerSliceIndex: map[string]*mixerSlice{},
	}
}

// Add to list of tracks and fire renegotation for all PeerConnections
func (m *mixer) newLocalTrackFromRemote(ps *peerServer, remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) (outputTrack *localTrack, err error) {
	outputTrack, err = newLocalTrack(ps, remoteTrack)

	if err == nil {
		m.Lock()
		m.mixerSliceIndex[remoteTrack.ID()] = newMixerSlice(m.shortId, ps.pc, outputTrack, receiver, remoteTrack.SSRC())
		m.Unlock()
	}
	return
}

// Remove from list of tracks and fire renegotation for all PeerConnections
func (m *mixer) removeLocalTrack(id string, signalingTrigger needsSignaling) {
	m.Lock()
	defer func() {
		m.Unlock()
		if signalingTrigger {
			m.managedUpdateSignaling("removed track")
		}
	}()

	if ms, exists := m.mixerSliceIndex[id]; exists {
		ms.stop()
		delete(m.mixerSliceIndex, id)
	}
}

func (m *mixer) updateTracks() success {
	for userId, ps := range m.room.peerServerIndex {
		// iterate to update peer connections of each PeerServer
		pc := ps.pc

		if pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
			delete(m.room.peerServerIndex, userId)
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
		if m.room.size != 1 {
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
				sender, err := pc.AddTrack(ms.outputTrack.track)

				if err != nil {
					log.Printf("[mixer %s error] pc.AddTrack: %v\n", m.shortId, err)
					return false
				}

				params := sender.GetParameters()
				if len(params.Encodings) == 1 {
					kind := ms.outputTrack.track.Kind().String()
					ssrc := params.Encodings[0].SSRC
					defaultBitrate := defaultVideoBitrate
					if kind == "audio" {
						defaultBitrate = defaultAudioBitrate
					}
					sc := senderController{
						ssrc:    ssrc,
						kind:    kind,
						sender:  sender,
						maxRate: uint64(defaultBitrate),
						maxREMB: uint64(defaultBitrate),
					}
					ms.Lock()
					ms.senderControllerIndex[userId] = &sc
					ms.Unlock()

					go ms.runSenderListener(&sc, ssrc, m.shortId)
				} else {
					log.Printf("[mixer %s error] wrong number of encoding parameters: %v\n", m.shortId, err)
				}

			}
		}
	}
	return true
}

func (m *mixer) updateOffers() success {
	for _, ps := range m.room.peerServerIndex {
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

		if err = ps.ws.sendWithPayload("offer", string(offerString)); err != nil {
			return false
		}
	}
	return true
}

// Signaling is split in two steps:
// - add or remove tracks on peer connections
// - update and send offers, if the boolean withSignaling is true
// Returning false means: please retry later
func (m *mixer) updateSignaling() success {
	if s := m.updateTracks(); !s {
		return false
	}
	if s := m.updateOffers(); !s {
		return false
	}
	return true
}

// Update each PeerConnection so that it is getting all the expected media tracks
func (m *mixer) managedUpdateSignaling(message string) {
	m.Lock()
	defer func() {
		m.Unlock()
		go m.dispatchKeyFrame()
	}()

	log.Printf("[mixer %s] signaling update: %s\n", m.room.shortId, message)

signalingLoop:
	for {
		select {
		case <-m.room.endCh:
			break signalingLoop
		default:
			for tries := 0; ; tries++ {
				switch m.updateSignaling() {
				case true:
					// signaling succeeded
					break signalingLoop
				case false:
					if tries >= 20 {
						// signaling failed too many times
						// release the lock and attempt a sync in 3 seconds. We might be blocking a RemoveTrack or AddTrack
						go func() {
							time.Sleep(time.Second * 3)
							m.managedUpdateSignaling("restarted after too many tries")
						}()
						return
					} else {
						// signaling failed
						time.Sleep(time.Second * 1)
					}
				}
			}
		}
	}
}

// sends a keyframe to all PeerConnections, used everytime a new user joins the call
func (m *mixer) dispatchKeyFrame() {
	m.RLock()
	defer m.RUnlock()

	for _, ps := range m.room.peerServerIndex {
		ps.pc.requestPLI()
	}
}

// func (ms *mixerSlice) runReceiverListener(shortId string) {
// 	buf := make([]byte, receiveMTU)

// listenerLoop:
// 	for {
// 		select {
// 		case <-ms.endCh:
// 			break listenerLoop
// 		default:
// 			n, _, err := ms.receiver.Read(buf)
// 			if err != nil {
// 				if err != io.EOF && err != io.ErrClosedPipe {
// 					log.Printf("[mixer %s error] receiver read RTCP: %v\n", shortId, err)
// 				}
// 				return
// 			}
// 			packets, err := rtcp.Unmarshal(buf[:n])
// 			if err != nil {
// 				log.Printf("[mixer %s error] receiver unmarshal RTCP: %v\n", shortId, err)
// 				continue
// 			}

// 			for _, packet := range packets {
// 				switch rtcpPacket := packet.(type) {
// 				case *rtcp.ReceiverEstimatedMaximumBitrate:
// 					log.Println(rtcpPacket)
// 				case *rtcp.SenderReport:
// 					log.Println(rtcpPacket)
// 					// default:
// 					// 	log.Printf("-- RTCP packet on receiver: %T", rtcpPacket)
// 				}
// 			}
// 		}
// 	}
// }
