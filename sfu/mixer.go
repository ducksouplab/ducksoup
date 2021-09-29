package sfu

import (
	"encoding/json"
	"fmt"
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
	mixerSliceIndex map[string]*mixerSlice // per remote track id
}

type mixerSlice struct {
	sync.Mutex
	outputTrack           *localTrack
	receivingPC           *peerConn // peer connection holding the incoming/remote track
	receiver              *webrtc.RTPReceiver
	senderControllerIndex map[string]*senderController // per user id
	updateTicker          *time.Ticker
	logTicker             *time.Ticker
	endCh                 chan struct{} // stop processing when track is removed
	optimalBitrate        uint64
}

type senderController struct {
	sync.Mutex
	ssrc           webrtc.SSRC
	kind           string
	sender         *webrtc.RTPSender
	optimalBitrate uint64
	maxBitrate     uint64
	// for logging
	shortId    string
	fromUserId string
	toUserId   string
}

type signalingState int

const (
	signalingOk signalingState = iota
	signalingRetryNow
	signalingRetryWithDelay
)

type needsSignaling bool

// senderController

// see https://datatracker.ietf.org/doc/html/draft-ietf-rmcat-gcc-02
// credits to https://github.com/jech/galene
func (sc *senderController) updateRateFromLoss(loss uint8) {
	sc.Lock()
	defer sc.Unlock()

	var newOptimalBitrate uint64
	prevOptimalBitrate := sc.optimalBitrate

	if loss < 5 {
		// loss < 0.02, multiply by 1.05
		newOptimalBitrate = prevOptimalBitrate * 269 / 256

		if sc.kind == "audio" {
			if newOptimalBitrate > maxAudioBitrate {
				newOptimalBitrate = maxAudioBitrate
			}
		} else {
			if newOptimalBitrate > maxVideoBitrate {
				newOptimalBitrate = maxVideoBitrate
			}
		}
	} else if loss > 25 {
		// loss > 0.1, multiply by (1 - loss/2)
		newOptimalBitrate = prevOptimalBitrate * (512 - uint64(loss)) / 512

		if sc.kind == "audio" {
			if newOptimalBitrate < minAudioBitrate {
				newOptimalBitrate = minAudioBitrate
			}
		} else {
			if newOptimalBitrate < minVideoBitrate {
				newOptimalBitrate = minVideoBitrate
			}
		}
		log.Printf("[info] [room#%s] [mixer] [from user#%s to user#%s] %d packets lost, previous bitrate %d, new bitrate %d\n",
			sc.shortId, sc.fromUserId, sc.toUserId, loss, prevOptimalBitrate/1000, newOptimalBitrate/1000)
	} else {
		newOptimalBitrate = prevOptimalBitrate
	}

	if newOptimalBitrate > sc.maxBitrate {
		newOptimalBitrate = sc.maxBitrate
	}
	sc.optimalBitrate = newOptimalBitrate
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

func newMixerSlice(pc *peerConn, outputTrack *localTrack, receiver *webrtc.RTPReceiver, room *trialRoom) *mixerSlice {
	updateTicker := time.NewTicker(1 * time.Second)
	logTicker := time.NewTicker(7300 * time.Millisecond)

	ms := &mixerSlice{
		outputTrack:           outputTrack,
		receivingPC:           pc,
		receiver:              receiver,
		senderControllerIndex: map[string]*senderController{},
		updateTicker:          updateTicker,
		logTicker:             logTicker,
		endCh:                 make(chan struct{}),
	}

	// update encoding bitrate on tick and according to minimum controller rate
	go func() {
		for range updateTicker.C {
			if len(ms.senderControllerIndex) > 0 {
				rates := []uint64{}
				for _, sc := range ms.senderControllerIndex {
					rates = append(rates, sc.optimalBitrate)
				}
				sliceRate := minUint64Slice(rates)
				if ms.outputTrack.pipeline != nil && sliceRate > 0 {
					ms.Lock()
					ms.optimalBitrate = sliceRate
					ms.Unlock()
					ms.outputTrack.pipeline.SetEncodingRate(sliceRate)
				}
			}
		}
	}()

	// periodical log for video
	if outputTrack.track.Kind().String() == "video" {
		go func() {
			for range logTicker.C {
				display := fmt.Sprintf("%v kbit/s", ms.optimalBitrate/1000)
				log.Printf("[info] [room#%s] [mixer] [user#%s] new video broadcasted bitrate: %s\n", room.shortId, pc.userId, display)
			}
		}()
	}

	return ms
}

func (ms *mixerSlice) stop() {
	ms.updateTicker.Stop()
	ms.logTicker.Stop()
	close(ms.endCh)
}

func (ms *mixerSlice) runSenderListener(sc *senderController, ssrc webrtc.SSRC, shortId string) {
	buf := make([]byte, receiveMTU)

	for {
		select {
		case <-ms.endCh:
			return
		default:
			n, _, err := sc.sender.Read(buf)
			if err != nil {
				if err != io.EOF && err != io.ErrClosedPipe {
					log.Printf("[error] [room#%s] [mixer] [from user#%s to user#%s] read RTCP: %v\n", shortId, sc.fromUserId, sc.toUserId, err)
				}
				return
			}
			packets, err := rtcp.Unmarshal(buf[:n])
			if err != nil {
				log.Printf("[error] [room#%s] [mixer] [from user#%s to user#%s] sender unmarshal RTCP: %v\n", shortId, sc.fromUserId, sc.toUserId, err)
				continue
			}

			for _, packet := range packets {
				switch rtcpPacket := packet.(type) {
				case *rtcp.PictureLossIndication:
					log.Printf("[info] [room#%s] [mixer] [from user#%s to user#%s] PLI received\n", shortId, sc.fromUserId, sc.toUserId)
					ms.receivingPC.throttledPLIRequest()
				case *rtcp.ReceiverReport:
					for _, r := range rtcpPacket.Reports {
						if r.SSRC == uint32(ssrc) {
							sc.updateRateFromLoss(r.FractionLost)
						}
					}
					// default:
					// 	log.Printf("[info] [room#%s] [mixer] [from user#%s to user#%s] RTCP packet %T:\n%v\n", shortId, sc.fromUserId, sc.toUserId, rtcpPacket, rtcpPacket)
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
		m.mixerSliceIndex[remoteTrack.ID()] = newMixerSlice(ps.pc, outputTrack, receiver, m.room)
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
			m.managedUpdateSignaling("removed track#" + id)
		}
	}()

	if ms, exists := m.mixerSliceIndex[id]; exists {
		ms.stop()
		delete(m.mixerSliceIndex, id)
	}
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
		ownTrackIndex := map[string]bool{}

		for _, sender := range pc.GetSenders() {
			if sender.Track() == nil {
				continue
			}

			alreadySentIndex[sender.Track().ID()] = true

			// if we have a RTPSender that doesn't map to an existing track remove and signal
			_, ok := m.mixerSliceIndex[sender.Track().ID()]
			if !ok {
				if err := pc.RemoveTrack(sender); err != nil {
					log.Printf("[error] [room#%s] [mixer] can't RemoveTrack: %v\n", m.room.shortId, err)
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
				ownTrackIndex[receiver.Track().ID()] = true
			}
		}

		// add all necessary track (not yet to the PeerConnection or not coming from same peer)
		for id, ms := range m.mixerSliceIndex {
			_, alreadySent := alreadySentIndex[id]
			_, ownTrack := ownTrackIndex[id]

			outputTrackId := ms.outputTrack.track.ID()
			if alreadySent {
				log.Printf("[info] [room#%s] [mixer] [user#%s] [already] skip add output track to pc: %s\n", m.room.shortId, userId, outputTrackId)
			} else if ownTrack {
				log.Printf("[info] [room#%s] [mixer] [user#%s] [own] skip add local track to pc: %s\n", m.room.shortId, userId, outputTrackId)
			} else {
				sender, err := pc.AddTrack(ms.outputTrack.track)
				if err != nil {
					log.Printf("[error] [room#%s] [user#%s] [mixer] can't AddTrack %s: %v\n", m.room.shortId, userId, id, err)
					return signalingRetryNow
				} else {
					log.Printf("[info] [room#%s] [user#%s] [mixer] added local track to pc: %s\n", m.room.shortId, userId, outputTrackId)
				}

				params := sender.GetParameters()
				if len(params.Encodings) == 1 {
					kind := ms.outputTrack.track.Kind().String()
					ssrc := params.Encodings[0].SSRC
					defaultBitrate := defaultVideoBitrate
					if kind == "audio" {
						defaultBitrate = defaultAudioBitrate
					}
					maxBitrate := maxVideoBitrate
					if kind == "audio" {
						maxBitrate = maxAudioBitrate
					}
					sc := senderController{
						ssrc:           ssrc,
						kind:           kind,
						sender:         sender,
						optimalBitrate: uint64(defaultBitrate),
						maxBitrate:     uint64(maxBitrate),
						shortId:        m.room.shortId,
						fromUserId:     ms.receivingPC.userId,
						toUserId:       userId,
					}
					ms.Lock()
					ms.senderControllerIndex[userId] = &sc
					ms.Unlock()

					go ms.runSenderListener(&sc, ssrc, m.room.shortId)
				} else {
					log.Printf("[error] [room#%s] [mixer] [user#%s] wrong number of encoding parameters: %v\n", m.room.shortId, userId, err)
				}
			}
		}
	}
	return signalingOk
}

func (m *mixer) updateOffers() signalingState {
	for _, ps := range m.room.peerServerIndex {
		pc := ps.pc

		log.Printf("[info] [room#%s] [mixer] [user#%s] signaling state: %v\n", m.room.shortId, ps.userId, pc.SignalingState())

		offer, err := pc.CreateOffer(nil)
		if err != nil {
			log.Printf("[error] [room#%s] [mixer] [user#%s] can't CreateOffer: %v\n", m.room.shortId, ps.userId, err)
			return signalingRetryNow
		}

		if pc.PendingLocalDescription() != nil {
			log.Printf("[error] [room#%s] [mixer] [user#%s] pending local description\n", m.room.shortId, ps.userId)
		}

		if err = pc.SetLocalDescription(offer); err != nil {
			log.Printf("[error] [room#%s] [mixer] [user#%s] can't SetLocalDescription: %v\n", m.room.shortId, ps.userId, err)
			//log.Printf("\n\n\n---- failing local descripting:\n%v\n\n\n", offer)
			return signalingRetryWithDelay
		}

		offerString, err := json.Marshal(offer)
		if err != nil {
			log.Printf("[error] [room#%s] [mixer] [user#%s] can't marshal offer: %v\n", m.room.shortId, ps.userId, err)
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
				} else if (state == signalingRetryNow) && (tries < 20) {
					// redo signaling / for loop
					break
				} else {
					// signalingRetryWithDelay OR signaling failed too many times
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

// func (ms *mixerSlice) runReceiverListener(shortId string) {
// 	buf := make([]byte, receiveMTU)

// 	for {
// 		select {
// 		case <-ms.endCh:
// 			return
// 		default:
// 			n, _, err := ms.receiver.Read(buf)
// 			if err != nil {
// 				if err != io.EOF && err != io.ErrClosedPipe {
// 					log.Printf("[error] [mixer room#%s] receiver read RTCP: %v\n", shortId, err)
// 				}
// 				return
// 			}
// 			packets, err := rtcp.Unmarshal(buf[:n])
// 			if err != nil {
// 				log.Printf("[error] [mixer room#%s] receiver unmarshal RTCP: %v\n", shortId, err)
// 				continue
// 			}

// 			for _, packet := range packets {
// 				switch rtcpPacket := packet.(type) {
// 				case *rtcp.ReceiverEstimatedMaximumBitrate:
// 					log.Println(rtcpPacket)
// 				case *rtcp.SenderReport:
// 					log.Println(rtcpPacket)
// 					// default:
// 					// 	log.Printf("-- RTCP packet on receiver: %T\n", rtcpPacket)
// 				}
// 			}
// 		}
// 	}
// }
