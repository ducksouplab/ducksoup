# DuckSoup

Videoconferencing tool for social experiments.

From a technical standpoint, DuckSoup is:

* a videoconference server acting as a relay for peers in the same room (more precisely, a SFU made with Go and [pion](https://github.com/pion/webrtc))
* with the possibility to record and optionnally transform video and audio streams thanks to GStreamer

## Embed DuckSoup

Let's assume we have a DuckSoup server installed and running at `ducksoup-host.example.com` and we want to embed a DuckSoup "player" in a website served at `my-experiment.example.com`.

The embedding origin (`my-experiment.example.com`) has to be listed as an authorized origin when starting the DuckSoup  instance available at `ducksoup-host.example.com` (see [Environment variables](#environment-variables)).

Then, on the experiment web page, include the `ducksoup.js` library:

```
<script src="https://ducksoup-host.example.com//scripts/lib/ducksoup.js"></script>
```

And render it (in JavaScript):

```
DuckSoup.render(mountEl, peerOptions, embedOptions);
```

Where:

- `mountEl` (DOM node) is the node where DuckSoup interface and media streams will be mounted (obtained for instance with `document.getElementById("ducksoup-container")`)

- `peerOptions` (object) must contain the following properties:

  - `signalingUrl` (string) the URL of DuckSoup signaling websocket (for instance `wss://ducksoup-host.example.com/ws` for a DuckSoup hosted at `ducksoup-host.example.com`) 
  - `room` (string) the room identifier
  - `uid` (string) a unique user identifier
  - `name` (string) the user display name
  - `duration` (integer) the duration of the experiment in seconds

- `peerOptions` may also contain the following optional properties:

  - `size` (integer, defaults to 2) the size of the room (size == 1 for a mirror effect)
  - `width` (integer, defaults to 800) of the video stream
  - `height` (integer, defaults to 600) of the video stream
  - `frameRate` (integer, defaults to 30) of the video stream
  - `audioFx` (string describing a GStreamer element and its properties, for instance "pitch pitch=0.8") if an audio effect has to be applied
  - `videoFx` (string describing a GStreamer element and its properties, for instance "coloreffects preset=xpro") if video effect has to be applied
  - `audio` (object) merged with DuckSoup default constraints and passed to getUserMedia (see [properties](https://developer.mozilla.org/en-US/docs/Web/API/MediaTrackConstraints#properties_of_audio_tracks))
  - `video` (object) merged with DuckSoup default constraints and passed to getUserMedia (see [properties](https://developer.mozilla.org/en-US/docs/Web/API/MediaTrackConstraints#properties_of_video_tracks))
  - `videoCodec` (string) possible values: "vp8" (default if none), "h264" or "vp9"
  - `rtcConfig` ([RTCConfiguration dictionary](https://developer.mozilla.org/en-US/docs/Web/API/RTCPeerConnection/RTCPeerConnection#rtcconfiguration_dictionary) object) used when creating an RTCPeerConnection, for instance to set iceServers
  - `namespace` (string, defaults to "default") to group recordings under the same namespace (folder)

- `embedOptions` (object) may be empty or contain the following optional properties:

  - `callback` (JavaScript function) to receive messages from DuckSoup (see following paragraph)
  - `debug` (boolean, defaults to false) to enable receiving debug messages (in callback)

The callback function will receive a message as a `{ kind, payload }` object where:

- kind (string) may be: `"start"`, `"end"`, `"error-duplicate"`, `"error-full"`, `"disconnection"` (and `"stats"` if debug is enabled) 
- payload (unrestricted type) is an optional payload

## Build from source

Dependencies:

- [Go](https://golang.org/doc/install)
- [GStreamer](https://gstreamer.freedesktop.org/documentation/index.html?gi-language=c)

Regarding GStreamer on Debian you may:

```
apt-get install libgstreamer1.0-0 gstreamer1.0-plugins-base gstreamer1.0-plugins-good gstreamer1.0-plugins-bad gstreamer1.0-plugins-ugly gstreamer1.0-libav gstreamer1.0-doc gstreamer1.0-tools gstreamer1.0-x gstreamer1.0-alsa gstreamer1.0-gl gstreamer1.0-gtk3 gstreamer1.0-qt5 gstreamer1.0-pulseaudio
apt-get install libgstreamer1.0-dev libgstreamer-plugins-base1.0-dev
```

To serve with TLS in a local setup, you may consider:

- [mkcert](https://github.com/FiloSottile/mkcert) to generate certificates

```
mkdir certs && cd certs && mkcert -key-file key.pem -cert-file cert.pem localhost 
```

Then build:

```
go build
```

## Environment variables

- DS_PORT=9000 (8000 is the default value) to set port listen by server
- DS_ORIGINS=https://origin1,https://origin2:8080 declares comma separated allowed origins for WebSocket connections
- DS_ENV=DEV enables automatic front-end assets build with esbuild + adds a few allowed origins for WebSocket connections
- GST_PLUGIN_PATH to declare additional GStreamer plugin paths (prefer appending to the existing GST_PLUGIN_PATH: GST_PLUGIN_PATH="$GST_PLUGIN_PATH:$PROJECT_BUILD")

## Run DuckSoup

Run (without DS_ENV=DEV nor DS_ORIGINS, signaling can't work since no accepted WebSocket origin is declared):

```
DS_ENV=DEV ./ducksoup
DS_ORIGINS=https://website-calling-ducksoup.example.com ./ducksoup
```

With TLS:

```
DS_ENV=DEV ./ducksoup --cert certs/cert.pem --key certs/key.pem
DS_ORIGINS=https://website-calling-ducksoup.example.com ./ducksoup --cert certs/cert.pem --key certs/key.pem
```

## Add custom GStreamer plugins

```
mkdir -p plugins
export PROJECT_BUILD=`pwd`/plugins
export GST_PLUGIN_PATH="$GST_PLUGIN_PATH:$PROJECT_BUILD"
```

These plugins (`libxyz.so` files) enable additional elements to be used in DuckSoup GStreamer pipelines. They have to be built against the same GStreamer version than the one running with DuckSoup (1.18.4 at the time of writing this documentation, check with `gst-inspect-1.0 --version`).

## Code within a Docker container

One may develop DuckSoup in a container based from `docker/Dockerfile.code` (for instance using VSCode containers integration).

This Dockerfile prefers specifying a debian version and installing go from source (rather than using the golang base image) so it's possible to choose the same OS version than in production and control gstreamer (apt) packages versions.

`docker/Dockerfile.code.golang_image` is an alternate Dockerfile relying on the golang base image.

## Build with Docker for production

The image build starts with the container root user (for apt dependencies) but then switch to a different appuser:appgroup to run the app:

```
docker build --build-arg appuser=$(id deploy -u) --build-arg appgroup=$(id deploy -g) -f docker/Dockerfile.build -t ducksoup:latest .
```

Set permissions to enabled writing in `logs` mounted volume:

```
sudo chown deploy:deploy logs
```

Run:

```
# bind port, mount volumes, set environment variable and remove container when stopped
docker run --name ducksoup_1 \
  -p 8000:8000 \
  --mount type=bind,source="$(pwd)"/logs,target=/app/logs \
  --mount type=bind,source="$(pwd)"/plugins,target=/app/plugins,readonly \
  --env DS_ORIGINS=http://localhost:8000 \
  --rm \
  ducksoup:latest

# and if needed enter the running ducksoup_1 container
docker exec -it ducksoup_1 /bin/bash
```

Run with docker-compose, thus binding volumes and persisting logs data (in `docker/data/logs`):

```
DS_USER=$(id deploy -u) DS_GROUP=$(id deploy -g) docker-compose -f docker/docker-compose.yml up --build
# and if needed enter the running ducksoup_1 container
docker exec -it docker_ducksoup_1 /bin/bash
```

### Multistage Dockerfile

If the goal is to distribute and minimize the image size, consider the multistage build:

```
# debian
docker build --build-arg appuser=$(id deploy -u) --build-arg appgroup=$(id deploy -g) -f docker/Dockerfile.build.multi_debian -t ducksoup_multi_debian:latest .
# alpine
docker build --build-arg appuser=$(id deploy -u) --build-arg appgroup=$(id deploy -g) -f docker/Dockerfile.build.multi_alpine -t ducksoup_multi_alpine:latest .
```

Deploy multi debian image to docker hub:

```
docker tag ducksoup_multi_debian altg/ducksoup
docker push altg/ducksoup:latest
```

Run multistage debian:

```
docker run --name ducksoup_multi_1 \
  -p 8000:8000 \
  --mount type=bind,source="$(pwd)"/logs,target=/app/logs \
  --mount type=bind,source="$(pwd)"/plugins,target=/app/plugins,readonly \
  --env DS_ORIGINS=http://localhost:8000 \
  --rm \
  ducksoup_multi:latest

# and if needed enter the running ducksoup_1 container
docker exec -it ducksoup_multi_1 /bin/bash
```

Run alpine:

```
docker run --name ducksoup_multi_2 \
  -p 8000:8000 \
  --mount type=bind,source="$(pwd)"/logs,target=/app/logs \
  --mount type=bind,source="$(pwd)"/plugins,target=/app/plugins,readonly \
  --env DS_ORIGINS=http://localhost:8000 \
  --rm \
  ducksoup_multi_alpine:latest
```

## Test front-ends

Several test front-ends are available:

- static/test_embed showcases how to embed DuckSoup in a iframe and receive messages from it
- static/test_standalone is a sample project not relying on static/embed

Once the app is running, you may try it with:

- http://localhost:8000/test_embed/ (two users -> two tabs)
- http://localhost:8000/test_standalone/ (two users -> two tabs)
- http://localhost:8000/test_mirror/ (one user)

## Websocket messages

Messages from server (Go) to client (JS):

- kind `offer` and `candidate` for signaling (with payloads)
- kind `start` when all peers and tracks are ready
- kind `ending` when the room will soon be destroyed
- kind `end` when time is over (payload contains an index of media files recorded for this experiment)
- kind `error-full` when room limit has been reached and user can't enter room
- kind `error-duplicate` when same user is already in room

## Front-ends build

Building js files (useful at least for bundling and browser improved compatibility, also for minification) is done with esbuild and triggered from go.

When `./ducksoup` is launched (see `front/build.go` to configure and build new front-ends), some js files are processed (from `front/src` to `front/static`).

It's also possible to watch changes and rebuild those files by adding this environment variable:

```
DS_ENV=DEV ./ducksoup
```

## Concepts in Go code

On each connection to the websocket endpoint in `server.go` a new PeerServer (see `peer_server.go`) is created:

- it manages further client communication through websocket (see `ws_conn.go`) and RTC (see `peer_conn.go`)
- join (create if necessary) room which manages the logical part (if room is full, if there is a disconnect/reconnect from same peer...)

Thus PeerServer struct holds a reference to a Room, and each Room has references to several PeerServers.