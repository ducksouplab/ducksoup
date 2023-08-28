package iceservers

import (
	"github.com/ducksouplab/ducksoup/env"
	"github.com/pion/webrtc/v3"
)

func GetDefaultSTUNServers() (iceServers []webrtc.ICEServer) {
	if len(env.STUNServerURLS) > 0 {
		for _, url := range env.STUNServerURLS {
			iceServers = append(iceServers, webrtc.ICEServer{URLs: []string{url}})
		}
		return iceServers
	}
	return
}
