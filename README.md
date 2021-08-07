# DuckSoup

Videoconferencing tool for social experiments.

From a technical standpoint, DuckSoup is:

- a videoconference server acting as a relay for peers in the same room (more precisely, a SFU made with Go and [pion](https://github.com/pion/webrtc))
- with the possibility to record and optionnally transform video and audio streams thanks to GStreamer

## DuckSoup server overview

A DuckSoup server exposes the following:

- an HTTP static server for `ducksoup.js` and example front-ends (TCP)
- an HTTP websocket endpoint for signaling (TCP)
- WebRTC (UDP)

Using the client library `ducksoup.js` is the preferred way to interact with DuckSoup server (signaling and WebRTC).

## DuckSoup player

Let's assume we have a DuckSoup server installed and running at `ducksoup-host.com` and we want to embed a DuckSoup "player" in a website served at `my-experiment-host.com`.

The embedding origin (`my-experiment-host.com`) has to be listed as an authorized origin when starting the DuckSoup instance available at `ducksoup-host.com` (see [Environment variables](#environment-variables)).

Then, on the experiment web page, include the `ducksoup.js` library:

```
<script src="https://ducksoup-host.com//scripts/lib/ducksoup.js"></script>
```

And render it (in JavaScript):

```
const dsPlayer = await DuckSoup.render(mountEl, peerOptions, embedOptions);
```

Where:

- assigning to a variable (`dsPlayer` above) is only needed if you want to further control the DuckSoup audio/video player instance (see (Player API)[#player-api])

- `mountEl` (DOM node) is the node where DuckSoup media streams will be rendered (obtained for instance with `document.getElementById("ducksoup-root")`). The video stream is set to fill mountEl: set mountEl width and height and the DuckSoup player will adapt.

- `peerOptions` (object) must contain the following properties:

  - `signalingUrl` (string) the URL of DuckSoup signaling websocket (for instance `wss://ducksoup-host.com/ws` for a DuckSoup hosted at `ducksoup-host.com`) 
  - `roomId` (string) the room identifier
  - `userId` (string) a unique user identifier
  - `name` (string) the user display name

- `peerOptions` may also contain the following optional properties:

  - `duration` (integer, defaults to 30) the duration of the experiment in seconds
  - `size` (integer, defaults to 2) the size of the room (size == 1 for a mirror effect)
  - `width` (integer, defaults to 800) of the video stream
  - `height` (integer, defaults to 600) of the video stream
  - `frameRate` (integer, defaults to 30) of the video stream
  - `audioFx` (string, see format in [Gstreamer effects](#gstreamer-effects)) if an audio effect has to be applied
  - `videoFx` (string, see format in [Gstreamer effects](#gstreamer-effects)) if video effect has to be applied
  - `audio` (object) merged with DuckSoup default constraints and passed to getUserMedia (see [properties](https://developer.mozilla.org/en-US/docs/Web/API/MediaTrackConstraints#properties_of_audio_tracks))
  - `video` (object) merged with DuckSoup default constraints and passed to getUserMedia (see [properties](https://developer.mozilla.org/en-US/docs/Web/API/MediaTrackConstraints#properties_of_video_tracks))
  - `videoCodec` (string) possible values: "VP8" (default if none) or "H264"
  - `rtcConfig` ([RTCConfiguration dictionary](https://developer.mozilla.org/en-US/docs/Web/API/RTCPeerConnection/RTCPeerConnection#rtcconfiguration_dictionary) object) used when creating an RTCPeerConnection, for instance to set iceServers
  - `namespace` (string, defaults to "default") to group recordings under the same namespace (folder)

- `embedOptions` (object) may be empty or contain the following optional properties:

  - `callback` (JavaScript function) to receive messages from DuckSoup (see following paragraph)
  - `debug` (boolean, defaults to false) to enable receiving debug messages (in callback)

The callback function will receive messages as `{ kind, payload }` objects where:

- kind (string) may be: `"start"`, `"end"`, `"error-duplicate"`, `"error-full"`, `"disconnection"` (and `"stats"` if debug is enabled) 
- payload (unrestricted type) is an optional payload

### GStreamer effects

DuckSoup server comes with GStreamer and the ability to apply effects on live video and audio streams. Check some [examples](https://gstreamer.freedesktop.org/documentation/tools/gst-launch.html?gi-language=c#pipeline-examples) from GStreamer documentation to get a glimpse of how to set GStreamer elements and their properties.

From the standpoint of DuckSoup, it is possible to add one audio and one video effect as a GStreamer element, following this syntax:

- generic format: `"element property1=value1 property2=value2 ..."` with 0, 1 or more properties
- audio processing example: `"pitch pitch=0.8"`
- video processing example: `"coloreffects preset=xpro"`

You may browse [available plugins](https://gstreamer.freedesktop.org/documentation/plugins_doc.html?gi-language=c) (each plugin contains one or more elements) to discover elements and their properties.

Please note that, even if the default DuckSoup configuration comes with the "good, bad and ugly" GStreamer plugin packages, some elements in those packages might not be available when running DuckSoup (especially due to hardware limitations).

It is also possible to add custom GStreamer plugins to DuckSoup (check the section [Custom GStreamer plugins](#custom-gstreamer-plugins))

### Controlling effects

If you want to control the properties of a GStreamer effect you need:

- to name the effect described in `audioFx` or `videoFx` by adding a `name` property, for instance `"element property1=1.0 name=fx"`
- call the player `audioControl` or `videoControl` method (depending on the stream the effect is enabled on), for instance `ds.audioControl("fx", "property1", 1.2)`

In this example, `proprety1` has an initial value of `1.0` and is updated to `1.2`.

For the time being only float values are allowed when controlling properties.

### Fake effects

You may use one of these two reserved effect names (either for `audioFx` or `videoFx`):

- `forward` tells DuckSoup to forward RTP packets directly from the incoming track to the outcoming one, without even involving GStreamer
- `passthrough` instantiates a minimal GStreamer that copies source to sink (input to output) without any depaying/decoding nor processing

They may be useful for debugging streaming quality issues, but won't even trigger file recording.

### Player API

Instantiation is an async operation : `const dsPlayer = await DuckSoup.render(mountEl, peerOptions, embedOptions);`

The following methods are available on a DuckSoup player:

- `audioControl(effectName, property, value, transitionDuration)` to update the property of the effect named in `peerOptions#audioFx`. For instance with an `audioFx` of `"element property1=1.0 name=fx"`:
  - `effectName` (string) is `fx`
  - `property` (string) is `property1`
  - `value` (float) sets a new value, for instance `1.1`
  - `transitionDuration` (integer counting ms, defaults to 0, expect better results for 200 and above) is the optional duration of the interpolation between the old and new values
- `videoControl(effectName, property, value, transitionDuration)` is the same as above fort the effect named in `peerOptions#videoFx`
- `stop()` to stop media streams and close communication with server. Note that players are running for a limited duration (set by `peerOptions#duration` which is capped server-side) and most of the time you don't need to use this method

### Front-end examples

There are several ways to use DuckSoup:

- the official and maintained way is to rely on ducksoup.js as described in [DuckSoup Player](#ducksoup-player)
- `static/test/standalone` communicates with DuckSoup server without ducksoup.js, reimplementing signaling and RTC logic (may be later deprecated / unmaintained)
- an alternate implementation relies on a served DuckSoup page meant to be embedded in an iframe. A full example is available in `static/test/embed` which contains an iframe that embeds `static/embed` (may be later deprecated / unmaintained)

Once the app is running, you may try them at:

- http://localhost:8000/test/mirror/ (one user, relies on ducksoup.js)
- http://localhost:8000/test/standalone/ (two users -> two tabs)
- http://localhost:8000/test/embed/ (two users -> two tabs)


## DuckSoup server

### Build from source

Dependencies:

- [Go](https://golang.org/doc/install)
- [GStreamer](https://gstreamer.freedesktop.org/documentation/index.html?gi-language=c)

Regarding GStreamer on Debian you may:

```
apt-get install libgstreamer1.0-0 gstreamer1.0-plugins-base gstreamer1.0-plugins-good gstreamer1.0-plugins-bad gstreamer1.0-plugins-ugly gstreamer1.0-libav gstreamer1.0-doc gstreamer1.0-tools gstreamer1.0-x gstreamer1.0-alsa gstreamer1.0-gl gstreamer1.0-gtk3 gstreamer1.0-qt5 gstreamer1.0-pulseaudio
apt-get install libgstreamer1.0-dev libgstreamer-plugins-base1.0-dev
```

Then build:

```
go build
```

### Environment variables

- DS_PORT=9000 (8000 is the default value) to set port listen by server
- DS_ORIGINS=https://origin1,https://origin2:8080 declares comma separated allowed origins for WebSocket connections
- DS_ENV=DEV enables automatic front-end assets build + adds a few allowed origins for WebSocket connections
- DS_ENV=BUILD_FRONT builds front-end assets but do not start server
- DS_TEST_LOGIN (defaults to "ducksoup") to protect test pages with HTTP authentitcation
- DS_TEST_PASSWORD (defaults to "ducksoup") to protect test pages with HTTP authentitcation
- DS_NVIDIA=true (default to false) if NVIDIA accelerated encoding and decoding is accessible on the host
- GST_PLUGIN_PATH to declare additional GStreamer plugin paths (prefer appending to the existing GST_PLUGIN_PATH: GST_PLUGIN_PATH="$GST_PLUGIN_PATH:/additional/plugins/path")

### Run DuckSoup server

Run (without DS_ENV=DEV nor DS_ORIGINS, signaling can't work since no accepted WebSocket origin is declared):

```
DS_ENV=DEV ./ducksoup
DS_ORIGINS=https://ducksoup-caller-host.com ./ducksoup
```

To serve with TLS in a local setup, you may consider [mkcert](https://github.com/FiloSottile/mkcert) to generate certificates. With mkcert installed:

```
mkdir certs && cd certs && mkcert -key-file key.pem -cert-file cert.pem localhost 
```

Run with TLS:

```
DS_ENV=DEV ./ducksoup --cert certs/cert.pem --key certs/key.pem
DS_ORIGINS=https://ducksoup-caller-host.com ./ducksoup --cert certs/cert.pem --key certs/key.pem
```

### Front-ends build

DuckSoup server comes with a few front-end examples. Building their js sources (useful at least for bundling and browser improved compatibility, also for minification) is done with esbuild and triggered from go.

When `./ducksoup` is launched (see `front/build.go` to configure and build new front-ends), some js files are processed (from `front/src` to `front/static`) depending on the `DS_ENV` environment value (see [Environment variables](#environment-variables)).

### Custom GStreamer plugins

First create a folder dedicated to custom plugins, and update `GST_PLUGIN_PATH` accordingly:

```
mkdir -p plugins
export GST_PLUGIN_PATH="$GST_PLUGIN_PATH:`pwd`/plugins"
```

Then add plugins (`libxyz.so` files) to this folder to enable them in DuckSoup GStreamer pipelines. They have to be built against the same GStreamer version than the one running with DuckSoup (1.18.4 at the time of writing this documentation, check with `gst-inspect-1.0 --version`).

If a plugin depends on an additinal dynamic library, just add the `*.so` file to the same plugins folder and update the `LD_LIBRARY_PATH`:

```
export LD_LIBRARY_PATH="$LD_LIBRARY_PATH:`pwd`/plugins"
```

### Concepts in Go code

On each connection to the websocket endpoint in `server.go` a new peerServer (see `peer_server.go`) is created:

- it manages further client communication through a TCP websocket (`ws_conn.go`) and a RTC Peer Connection (`peer_conn.go`)
- then it joins (creates if necessary) a room (`trial_room.go`) managing the user logic (accept/reject, deal with reconnections) and the trial sequencing
- each room hold a reference to a mixer (`mixer.go`) that implements the SFU part: manages tracks attached to peer connections, handle signaling and RTCP feedback

For a given user connected to the room, there is one peerServer (abbreviated `ps` in the code), one wsConn (`ws`) and one peerConn (`pc`).

Depending on the size of the room, it may hold references to several peerServers.

Each peerConn has several tracks:

- remote: 2 (audio and video) client->server tracks
- local: 2*(n-1) server->client tracks (for a room of size n, since peers don't receive back their own streams)

### Websocket messages

Messages from server (Go) to client (JS):

- kind `offer` and `candidate` for signaling (with payloads)
- kind `start` when all peers and tracks are ready
- kind `ending` when the room will soon be destroyed
- kind `end` when time is over (payload contains an index of media files recorded for this experiment)
- kind `error-full` when room limit has been reached and user can't enter room
- kind `error-duplicate` when same user is already in room

### Docker and dlib

If you want to build the following Docker images with dlib (if you use GStreamer plugins that rely on dlib), choose a dlib version (>=19.22) and add its source to `docker/dlib` as `dlib.tar.bz2`:

```
mkdir -p docker-deps/dlib
curl http://dlib.net/files/dlib-19.22.tar.bz2 --output docker-deps/dlib/dlib.tar.bz2
```

Then build the images with `--build-arg DLIB=true` (used in all examples below).

### Code within a Docker container

One may develop DuckSoup in a container based from `docker/Dockerfile.code` (for instance using VSCode containers integration).

This Dockerfile prefers specifying a Debian version and installing go from source (rather than using the golang base image) so it's possible to choose the same OS version than in production and control gstreamer (apt) packages versions.

If you want to disable dlib compilation within the vscode Docker container, change the `build.args` property of `.devcontainer/devcontainer.json`.

### Build Docker image

The image build starts with the container root user (for apt dependencies) but then switch to a different appuser:appgroup to run the app:

```
docker build -f docker/Dockerfile.build --build-arg DLIB=true -t ducksoup:latest .
```

Supposing we use a `deploy` user for running the container, prepare the volume `data` target:

```
sudo chown -R deploy:deploy data
```

Run (note the `--user` option), mounting `etc` being optional and a convenient way to edit the configuration files it contains without rebuilding the image:

```
# bind port, mount volumes, set environment variable and remove container when stopped
docker run --name ducksoup_1 \
  -p 8000:8000 \
  --user $(id deploy -u):$(id deploy -g) \
  --env GST_DEBUG=2 \
  --mount type=bind,source="$(pwd)"/etc,target=/app/etc \
  --mount type=bind,source="$(pwd)"/data,target=/app/data \
  --mount type=bind,source="$(pwd)"/plugins,target=/app/plugins,readonly \
  --env DS_ORIGINS=http://localhost:8000 \
  --rm \
  ducksoup:latest

# and if needed enter the running ducksoup_1 container
docker exec -it ducksoup_1 bash
```

Or run with docker-compose:

```
DS_USER=$(id deploy -u) DS_GROUP=$(id deploy -g) docker-compose -f docker/docker-compose.yml up --build
```

### Build multistage Docker image

If the goal is to distribute and minimize the image size, consider the (Debian based) multistage build:

```
docker build -f docker/Dockerfile.build.multi --build-arg DLIB=true -t ducksoup_multi:latest .
docker tag ducksoup_multi creamlab/ducksoup
```

Deploy image to docker hub (replace `creamlab` by your Docker Hub login):

```
docker push creamlab/ducksoup:latest
```

Supposing we use a `deploy` user for running the container, prepare the volume `data` target:

```
sudo chown -R deploy:deploy data
```

Run (note the `--user` option, see running `ducksoup:latest` above if you want to mount `etc`):

```
docker run --name ducksoup_multi_1 \
  -p 8000:8000 \
  --user $(id deploy -u):$(id deploy -g) \
  --env GST_DEBUG=2 \
  --mount type=bind,source="$(pwd)"/data,target=/app/data \
  --mount type=bind,source="$(pwd)"/plugins,target=/app/plugins,readonly \
  --env DS_ORIGINS=http://localhost:8000 \
  --rm \
  ducksoup_multi:latest

# and if needed enter the running ducksoup_1 container
docker exec -it ducksoup_multi_1 bash
```

Or run with docker-compose:

```
DS_USER=$(id deploy -u) DS_GROUP=$(id deploy -g) docker-compose -f docker/docker-compose.yml up --build
```

### Run tests

Launch the custom script:

```
./test
```

It triggers tests in the project subfolders, setting appropriate environment variables for specific test behavior.

### Update all go deps

```
go get -t -u ./...
```