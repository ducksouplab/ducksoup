package turnserver

import (
	"net"

	"github.com/ducksouplab/ducksoup/env"
	"github.com/pion/turn/v2"
	"github.com/rs/zerolog/log"
)

var password string
var realm = "ducksoup-realm"
var username = "ducksoup"
var server *turn.Server
var started bool

// based on https://github.com/pion/turn/blob/master/examples/turn-server/simple/main.go

func init() {
	password = env.TurnPassword
	// TODO generate password on app start
	// password = helpers.RandomHexString(32)
}

func Join(u string) (ok bool, credential string) {
	if started {
		return true, password
	}
	return
}

func Start() {
	if len(env.TurnAddress) == 0 || len(env.TurnPort) == 0 {
		log.Info().Str("context", "app").Msg("turn_server_not_requested")
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
		AuthHandler: func(u string, r string, srcAddr net.Addr) ([]byte, bool) {
			if u == username && r == realm {
				// TODO use u as username
				return turn.GenerateAuthKey(username, r, password), true
			}
			return nil, false
		},
		// UDP Listeners
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn: udpListener,
				RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
					RelayAddress: net.IP(env.TurnAddress), // Claim that we are listening on given IP
					Address:      "0.0.0.0",               // But actually be listening on every interface
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
	server.Close()
}
