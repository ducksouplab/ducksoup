package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
	"webrtc-transform/gst"

	"github.com/gorilla/websocket"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

var (
	addr     = flag.String("addr", ":8080", "http service address")
	cert     = flag.String("cert", "", "cert file")
	key      = flag.String("key", "", "key file")
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	// lock for peerConnections and trackLocals
	tracksLock      sync.RWMutex
	peerConnections []peerConnectionState
	videoTracks     map[string]*webrtc.TrackLocalStaticRTP
	audioTracks     map[string]*webrtc.TrackLocalStaticSample
)

type websocketMessage struct {
	Event string `json:"event"`
	Data  string `json:"data"`
}

type peerConnectionState struct {
	peerConnection *webrtc.PeerConnection
	websocket      *threadSafeWriter
}

func main() {
	// Parse the flags passed to program
	flag.Parse()

	// Init other state
	log.SetFlags(0)
	videoTracks = map[string]*webrtc.TrackLocalStaticRTP{}
	audioTracks = map[string]*webrtc.TrackLocalStaticSample{}

	// Server static files
	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)

	// websocket handler
	http.HandleFunc("/signaling", websocketHandler)

	// request a keyframe every 3 seconds
	go func() {
		for range time.NewTicker(time.Second * 3).C {
			dispatchKeyFrame()
		}
	}()

	// start HTTP server
	if *key != "" && *cert != "" {
		log.Println("Listening on https://", *addr)
		log.Fatal(http.ListenAndServeTLS(*addr, *cert, *key, nil))
	} else {
		log.Println("Listening on http://", *addr)
		log.Fatal(http.ListenAndServe(*addr, nil))
	}

}

// Add to list of tracks and fire renegotation for all PeerConnections
func addVideoTrack(t *webrtc.TrackRemote) *webrtc.TrackLocalStaticRTP {
	tracksLock.Lock()
	defer func() {
		tracksLock.Unlock()
		signalPeerConnections()
	}()

	// Create a new TrackLocal with the same codec as our incoming
	track, err := webrtc.NewTrackLocalStaticRTP(t.Codec().RTPCodecCapability, t.ID(), t.StreamID())
	if err != nil {
		panic(err)
	}

	videoTracks[t.ID()] = track
	return track
}

// Remove from list of tracks and fire renegotation for all PeerConnections
func removeVideoTrack(t *webrtc.TrackLocalStaticRTP) {
	tracksLock.Lock()
	defer func() {
		tracksLock.Unlock()
		signalPeerConnections()
	}()

	delete(videoTracks, t.ID())
}

func addAudioTrack(t *webrtc.TrackRemote) *webrtc.TrackLocalStaticSample {
	tracksLock.Lock()
	defer func() {
		tracksLock.Unlock()
		signalPeerConnections()
	}()

	// Create a new TrackLocal with the same codec as our incoming
	track, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: "audio/opus"}, t.ID(), t.StreamID())
	if err != nil {
		panic(err)
	}

	audioTracks[t.ID()] = track
	return track
}

func removeAudioTrack(t *webrtc.TrackLocalStaticSample) {
	tracksLock.Lock()
	defer func() {
		tracksLock.Unlock()
		signalPeerConnections()
	}()

	delete(audioTracks, t.ID())
}

// signalPeerConnections updates each PeerConnection so that it is getting all the expected media tracks
func signalPeerConnections() {
	tracksLock.Lock()
	defer func() {
		tracksLock.Unlock()
		dispatchKeyFrame()
	}()

	attemptSync := func() (tryAgain bool) {
		for i := range peerConnections {
			if peerConnections[i].peerConnection.ConnectionState() == webrtc.PeerConnectionStateClosed {
				peerConnections = append(peerConnections[:i], peerConnections[i+1:]...)
				return true // We modified the slice, start from the beginning
			}

			// map of sender we already are sending, so we don't double send
			existingSenders := map[string]bool{}

			for _, sender := range peerConnections[i].peerConnection.GetSenders() {
				if sender.Track() == nil {
					continue
				}

				existingSenders[sender.Track().ID()] = true

				// If we have a RTPSender that doesn't map to a existing track remove and signal
				_, videoOk := videoTracks[sender.Track().ID()]
				_, audioOk := audioTracks[sender.Track().ID()]
				if !videoOk && !audioOk {
					if err := peerConnections[i].peerConnection.RemoveTrack(sender); err != nil {
						return true
					}
				}
			}

			// Don't receive videos we are sending, make sure we don't have loopback (remote peer point of view)
			for _, receiver := range peerConnections[i].peerConnection.GetReceivers() {
				if receiver.Track() == nil {
					continue
				}

				existingSenders[receiver.Track().ID()] = true
			}

			// Add all track we aren't sending yet to the PeerConnection
			for trackID := range videoTracks {
				if _, ok := existingSenders[trackID]; !ok {
					if _, err := peerConnections[i].peerConnection.AddTrack(videoTracks[trackID]); err != nil {
						return true
					}
				}
			}
			for trackID := range audioTracks {
				if _, ok := existingSenders[trackID]; !ok {
					if _, err := peerConnections[i].peerConnection.AddTrack(audioTracks[trackID]); err != nil {
						return true
					}
				}
			}

			offer, err := peerConnections[i].peerConnection.CreateOffer(nil)
			if err != nil {
				return true
			}

			if err = peerConnections[i].peerConnection.SetLocalDescription(offer); err != nil {
				return true
			}

			offerString, err := json.Marshal(offer)
			if err != nil {
				return true
			}

			if err = peerConnections[i].websocket.WriteJSON(&websocketMessage{
				Event: "offer",
				Data:  string(offerString),
			}); err != nil {
				return true
			}
		}

		return
	}

	for syncAttempt := 0; ; syncAttempt++ {
		if syncAttempt == 25 {
			// Release the lock and attempt a sync in 3 seconds. We might be blocking a RemoveTrack or AddTrack
			go func() {
				time.Sleep(time.Second * 3)
				signalPeerConnections()
			}()
			return
		}

		if !attemptSync() {
			break
		}
	}
}

// dispatchKeyFrame sends a keyframe to all PeerConnections, used everytime a new user joins the call
func dispatchKeyFrame() {
	tracksLock.Lock()
	defer tracksLock.Unlock()

	for i := range peerConnections {
		for _, receiver := range peerConnections[i].peerConnection.GetReceivers() {
			if receiver.Track() == nil {
				continue
			}

			_ = peerConnections[i].peerConnection.WriteRTCP([]rtcp.Packet{
				&rtcp.PictureLossIndication{
					MediaSSRC: uint32(receiver.Track().SSRC()),
				},
			})
		}
	}
}

// Handle incoming websockets
func websocketHandler(w http.ResponseWriter, r *http.Request) {
	// Upgrade HTTP request to Websocket
	unsafeConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}

	conn := &threadSafeWriter{unsafeConn, sync.Mutex{}}

	// When this frame returns close the Websocket
	defer conn.Close() //nolint

	// Create new PeerConnection
	peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		log.Print(err)
		return
	}

	// When this frame returns close the PeerConnection
	defer peerConnection.Close() //nolint

	// Accept one audio and one video track incoming
	for _, typ := range []webrtc.RTPCodecType{webrtc.RTPCodecTypeVideo, webrtc.RTPCodecTypeAudio} {
		if _, err := peerConnection.AddTransceiverFromKind(typ, webrtc.RTPTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionRecvonly,
		}); err != nil {
			log.Print(err)
			return
		}
	}

	// Add our new PeerConnection to global list
	tracksLock.Lock()
	peerConnections = append(peerConnections, peerConnectionState{peerConnection, conn})
	tracksLock.Unlock()

	// This will notify you when the peer has connected/disconnected
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("Connection State has changed %s \n", connectionState.String())
	})

	// Trickle ICE. Emit server candidate to client
	peerConnection.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i == nil {
			return
		}

		candidateString, err := json.Marshal(i.ToJSON())
		if err != nil {
			log.Println(err)
			return
		}

		if writeErr := conn.WriteJSON(&websocketMessage{
			Event: "candidate",
			Data:  string(candidateString),
		}); writeErr != nil {
			log.Println(writeErr)
		}
	})

	// If PeerConnection is closed remove it from global list
	peerConnection.OnConnectionStateChange(func(p webrtc.PeerConnectionState) {
		switch p {
		case webrtc.PeerConnectionStateFailed:
			if err := peerConnection.Close(); err != nil {
				log.Print(err)
			}
		case webrtc.PeerConnectionStateClosed:
			signalPeerConnections()
		}
	})

	peerConnection.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		buf := make([]byte, 1500)
		if track.Kind().String() == "audio" {
			// Create a track to fan out our incoming video to all peers
			audioTrack := addAudioTrack(track)
			defer removeAudioTrack(audioTrack)

			for {
				codecName := strings.Split(track.Codec().RTPCodecCapability.MimeType, "/")[1]
				pipelineAudio := gst.CreatePipeline(codecName, []*webrtc.TrackLocalStaticSample{audioTrack})
				pipelineAudio.Start()

				for {
					i, _, readErr := track.Read(buf)
					if readErr != nil {
						return
					}

					pipelineAudio.Push(buf[:i])
				}
			}

		} else {
			// Create a track to fan out our incoming video to all peers
			videoTrack := addVideoTrack(track)
			defer removeVideoTrack(videoTrack)

			for {
				i, _, err := track.Read(buf)
				if err != nil {
					return
				}

				if _, err = videoTrack.Write(buf[:i]); err != nil {
					return
				}
			}

		}
	})

	// Signal for the new PeerConnection
	signalPeerConnections()

	message := &websocketMessage{}
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			log.Println(err)
			return
		} else if err := json.Unmarshal(raw, &message); err != nil {
			log.Println(err)
			return
		}

		switch message.Event {
		case "candidate":
			candidate := webrtc.ICECandidateInit{}
			if err := json.Unmarshal([]byte(message.Data), &candidate); err != nil {
				log.Println(err)
				return
			}

			if err := peerConnection.AddICECandidate(candidate); err != nil {
				log.Println(err)
				return
			}
		case "answer":
			answer := webrtc.SessionDescription{}
			if err := json.Unmarshal([]byte(message.Data), &answer); err != nil {
				log.Println(err)
				return
			}

			if err := peerConnection.SetRemoteDescription(answer); err != nil {
				log.Println(err)
				return
			}
		}
	}
}

// Helper to make Gorilla Websockets threadsafe
type threadSafeWriter struct {
	*websocket.Conn
	sync.Mutex
}

func (t *threadSafeWriter) WriteJSON(v interface{}) error {
	t.Lock()
	defer t.Unlock()

	return t.Conn.WriteJSON(v)
}
