// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package main implements a simple TURN server
package turnserver

import (
	"net"
	"regexp"
	"sync"

	"github.com/ducksouplab/ducksoup/env"
	"github.com/pion/turn/v2"
	"github.com/rs/zerolog/log"
)

const realm = "ducksoup-realm"

var turnIP net.IP
var server *turn.Server
var started bool
var userStore struct {
	mu    sync.Mutex
	index map[string][]byte
}

func init() {
	userStore.mu.Lock()
	defer userStore.mu.Unlock()

	initialUsers := "ducksoup=" + env.TurnPassword
	// TODO generate password on app start
	// password = helpers.RandomHexString(32)

	turnIP = net.ParseIP(env.PublicIP)
	userStore.index = make(map[string][]byte)

	for _, kv := range regexp.MustCompile(`(\w+)=(\w+)`).FindAllStringSubmatch(initialUsers, -1) {
		userStore.index[kv[1]] = turn.GenerateAuthKey(kv[1], realm, kv[2])
	}
}

func Join(u string) (ok bool, credential string) {
	if started {
		// todo store hash
		return true, env.TurnPassword
	}
	return
}

func Start() {
	if turnIP == nil || len(env.TurnAddress) == 0 || len(env.TurnPort) == 0 {
		log.Info().Str("context", "app").Msg("turn_server_disabled")
		return
	}
	log.Info().Str("context", "app").Msg("turn_server_requested")

	udpListener, err := net.ListenPacket("udp4", "0.0.0.0:"+env.TurnPort)
	if err != nil {
		log.Error().Str("context", "app").Err(err).Msg("turn_server_error")
		return
	}

	server, err = turn.NewServer(turn.ServerConfig{
		Realm: realm,
		// Set AuthHandler callback
		// This is called every time a user tries to authenticate with the TURN server
		// Return the key for that user, or false when no user is found
		AuthHandler: func(username string, realm string, srcAddr net.Addr) ([]byte, bool) {
			if key, ok := userStore.index[username]; ok {
				return key, true
			}
			return nil, false
		},
		// PacketConnConfigs is a list of UDP Listeners and the configuration around them
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn: udpListener,
				RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
					RelayAddress: turnIP,    // Claim that we are listening on IP passed by user (This should be your Public IP)
					Address:      "0.0.0.0", // But actually be listening on every interface
				},
			},
		},
	})
	if err != nil {
		log.Error().Str("context", "app").Err(err).Msg("turn_server_error")
		return
	}
	started = true
	log.Info().Str("context", "app").Msg("turn_server_started")
}

func Stop() {
	if started {
		server.Close()
	}
}
