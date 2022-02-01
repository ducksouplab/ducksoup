module github.com/creamlab/ducksoup

go 1.17

require (
	github.com/evanw/esbuild v0.14.16
	github.com/google/uuid v1.3.0
	github.com/gorilla/mux v1.8.0
	github.com/gorilla/websocket v1.4.2
	github.com/joho/godotenv v1.4.0
	github.com/pion/ice/v2 v2.1.20
	github.com/pion/interceptor v0.1.7
	github.com/pion/rtcp v1.2.9
	github.com/pion/rtp v1.7.4
	github.com/pion/sdp/v3 v3.0.4
	github.com/pion/webrtc/v3 v3.1.19
	github.com/rs/zerolog v1.26.1
	gopkg.in/yaml.v2 v2.4.0
)

// replace github.com/pion/interceptor => ./forks/interceptor

require (
	github.com/pion/datachannel v1.5.2 // indirect
	github.com/pion/dtls/v2 v2.1.1 // indirect
	github.com/pion/logging v0.2.2 // indirect
	github.com/pion/mdns v0.0.5 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/sctp v1.8.2 // indirect
	github.com/pion/srtp/v2 v2.0.5 // indirect
	github.com/pion/stun v0.3.5 // indirect
	github.com/pion/transport v0.13.0 // indirect
	github.com/pion/turn/v2 v2.0.6 // indirect
	github.com/pion/udp v0.1.1 // indirect
	golang.org/x/crypto v0.0.0-20220131195533-30dcbda58838 // indirect
	golang.org/x/net v0.0.0-20220127200216-cd36cc0744dd // indirect
	golang.org/x/sys v0.0.0-20220128215802-99c3d69c2c27 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
)
