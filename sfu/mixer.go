package sfu

import (
	"encoding/json"
	"io"
	"log"
	"sync"

	"github.com/creamlab/ducksoup/gst"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

const DefaultBitrate = 80 * 8 * 1000
const MinBitrate = 20 * 8 * 1000
const MaxBitrate = 160 * 8 * 1000

// mixer is not guarded by a mutex since all interaction is done through room
type mixer struct {
	shortId      string              // room's shortId used for logging
	channelIndex map[string]*channel // per track id
}

// expanding the mixer metaphor, a channel strip holds everything related to a given signal (output track, GStreamer pipeline...)
type channel struct {
	sync.Mutex
	localTrack            *webrtc.TrackLocalStaticRTP
	pipeline              *gst.Pipeline
	maxRate               uint64                       // maxRate per peerConn
	senderControllerIndex map[string]*senderController // per user id
}

type senderController struct {
	ssrc    webrtc.SSRC
	sender  *webrtc.RTPSender
	maxRate uint64
}

type success bool

func newMixer(shortId string) *mixer {
	return &mixer{
		shortId:      shortId,
		channelIndex: map[string]*channel{},
	}
}

// Add to list of tracks and fire renegotation for all PeerConnections
func (m *mixer) newLocalTrack(c webrtc.RTPCodecCapability, id, streamID string) *webrtc.TrackLocalStaticRTP {
	// Create a new TrackLocal with the same codec as the incoming one
	track, err := webrtc.NewTrackLocalStaticRTP(c, id, streamID)

	if err != nil {
		log.Printf("[room %s error] NewTrackLocalStaticRTP: %v\n", m.shortId, err)
		panic(err)
	}

	m.channelIndex[id] = &channel{
		localTrack:            track,
		senderControllerIndex: map[string]*senderController{},
	}
	return track
}

// Remove from list of tracks and fire renegotation for all PeerConnections
func (m *mixer) removeLocalTrack(id string) {
	delete(m.channelIndex, id)
}

func (m *mixer) bindPipeline(id string, pipeline *gst.Pipeline) {
	m.channelIndex[id].pipeline = pipeline
}

func (controller *senderController) updateRate(loss uint8) {
	var newMaxRate uint64
	if loss < 5 {
		// loss < 0.02, multiply by 1.05
		newMaxRate = controller.maxRate * 269 / 256
		if newMaxRate > MaxBitrate {
			newMaxRate = MaxBitrate
		}
	} else if loss > 25 {
		// loss > 0.1, multiply by (1 - loss/2)
		newMaxRate = controller.maxRate * (512 - uint64(loss)) / 512
		if newMaxRate < MinBitrate {
			newMaxRate = MinBitrate
		}
	}
	controller.maxRate = newMaxRate
	log.Println("maxRate", newMaxRate, controller)
}

func runRTCPListener(controller *senderController, ssrc webrtc.SSRC, shortId string) {
	buf := make([]byte, 1500)
	for {
		n, _, err := controller.sender.Read(buf)
		if err != nil {
			if err != io.EOF && err != io.ErrClosedPipe {
				log.Printf("[room %s error] read RTCP: %v\n", shortId, err)
			}
			return
		}
		packets, err := rtcp.Unmarshal(buf[:n])
		if err != nil {
			log.Printf("Unmarshal RTCP: %v", err)
			continue
		}

		for _, packet := range packets {
			switch rtcpPacket := packet.(type) {
			// 	TODO send PLI to pipeline?
			// case *rtcp.PictureLossIndication:
			case *rtcp.ReceiverEstimatedMaximumBitrate:
				log.Printf("-- Estimated Bitrate: %d", rtcpPacket.Bitrate)
				// TODO
			case *rtcp.ReceiverReport:
				log.Println("-- ", rtcpPacket)
				for _, r := range rtcpPacket.Reports {
					if r.SSRC == uint32(ssrc) {
						//rtpState.updateRate(r.FractionLost)
					}
				}
			default:
				log.Printf("-- RTCP packet received: %T", packet)
			}
		}
	}
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
			_, ok := m.channelIndex[sender.Track().ID()]
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
		for id, channel := range m.channelIndex {
			if _, exists := existingSenders[id]; !exists {
				sender, err := pc.AddTrack(channel.localTrack)

				if err != nil {
					log.Printf("[room %s error] pc.AddTrack: %v\n", m.shortId, err)
					return false
				}

				params := sender.GetParameters()
				if len(params.Encodings) == 1 {
					channel.Lock()
					ssrc := params.Encodings[0].SSRC
					controller := senderController{
						ssrc:    ssrc,
						sender:  sender,
						maxRate: DefaultBitrate,
					}
					channel.senderControllerIndex[userId] = &controller
					channel.Unlock()

					go runRTCPListener(&controller, ssrc, m.shortId)
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
