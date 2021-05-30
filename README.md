# webrtc-transform

SFU made with [pion](https://github.com/pion/webrtc) with Gstreamer audio transformation.

## Installation

Dependencies:

- [GStreamer](https://gstreamer.freedesktop.org/documentation/index.html?gi-language=c)

For instance on Debian you may:

```
apt-get install libgstreamer1.0-0 gstreamer1.0-plugins-base gstreamer1.0-plugins-good gstreamer1.0-plugins-bad gstreamer1.0-plugins-ugly gstreamer1.0-libav gstreamer1.0-doc gstreamer1.0-tools gstreamer1.0-x gstreamer1.0-alsa gstreamer1.0-gl gstreamer1.0-gtk3 gstreamer1.0-qt5 gstreamer1.0-pulseaudio
apt-get install libgstreamer1.0-dev libgstreamer-plugins-base1.0-dev
```

To serve with TLS, you may consider:

- [mkcert](https://github.com/FiloSottile/mkcert) to generate certificates

```
mkdir certs && cd certs && mkcert localhost -key-file key.pem -cert-file cert.pem
```

### Run

Environment variables:

- APP_ENV=DEV enables automatic front-end assets build with esbuild + adds http://localhost:8080 to allowed origins for WebSocket connections
- ORIGINS=https://origin1,https://origin2:8000 adds comma separated allowed origins for WebSocket connections

Then build:

```
go build
```

And run with/out environment variables:

```
./webrtc-transform
APP_ENV=DEV ./webrtc-transform
ORIGINS=http://localhost ./webrtc-transform
```

Run with TLS:

```
./webrtc-transform --cert cert-path --key key-path
# for instance
./webrtc-transform --cert certs/cert.pem --key certs/key.pem
```

### Try (front-ends)

Several front-ends are available:

- static/test is a generic project intended to test the back-end behavior
- static/1on1 is intended to be embedded in a iframe (the website serving the page with the iframe has to be added to ORIGINS)
- static/embed is an example of a project that embeds 1on1

Once the app is running, you may try it with:

- http://localhost:8080/test/ (in several tabs)
- http://localhost:8080/embed/ (in several tabs)

# Front-ends build

Building js files (useful at least for bundling and browser improved compatibility, also for minification) is done with esbuild and triggered from go.

When `./webrtc-transform` is launched (see `front/build.go` to configure and build new front-ends), some js files are processed (from `front/src` to `front/static`).

It's also possible to watch changes and rebuild those files by adding this environment variable:

```
APP_ENV=DEV ./webrtc-transform
```

### Run with Docker

Generate certs (see above) and then:

```
docker build -t webrtc-transform:latest .
docker container run -p 8080:8080 --rm webrtc-transform:latest
# or enter the container
docker container run -p 8080:8080 -it --entrypoint /bin/bash webrtc-transform:latest
```

To try without certs:

```
docker build -f docker/Dockerfile.no-tls -t webrtc-transform:latest .
docker container run -p 8080:8080 -rm webrtc-transform:latest
```

### Add custom GStreamer plugins

mkdir -p lib
export PROJECT_BUILD=`pwd`/lib
export GST_PLUGIN_PATH="$GST_PLUGIN_PATH:$PROJECT_BUILD"

### Issues with Docker

`Dockerfile.multi-*` are intended to build multi-layered Docker images, separating building step _and_ dependencies from the final running environment. It currently does not work (INVESTIGATION NEEDED)

Hint for multi-debian: debug go execution, and check for relevant gstreamer runtime dependencies (try to add same apt dependencies in build and run stages, then clean up)

Hint for multi-alpine: apparent missing dependency to be found (https://superuser.com/questions/1176200/no-such-file-when-it-exists). Maybe easier to fix multi-debian first. See https://github.com/pion/ion/blob/master/docker/sfu.Dockerfile

### Concepts in Go code

On each connection to the websocket endpoint in `server.go` a new PeerServer (see `peer_server.go`) is created:

- it manages further client communication through websocket (see `ws_conn.go`) and RTC (see `peer_conn.go`)
- join (create if necessary) room which manages the logical part (if room is full, if there is a disconnect/reconnect from same peer...)

Thus PeerServer struct holds a reference to a Room, and each Room has references to several PeerServers.


### ws-protocol

Events from server to client:

- kind `offer` and `candidate` for signaling (with payloads)
- kind `start` when all peers and tracks are ready
- kind `finishing` when the room will soon be destroyed
- kind `finish` when time is over (payload contains a concatenated list of media files recorded for this experiment)
