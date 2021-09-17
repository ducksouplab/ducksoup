# DuckSoup

Videoconferencing tool for social experiments.

From a technical standpoint, DuckSoup is:

- a videoconference server acting as a relay for peers in the same room (more precisely, a SFU made with Go and [pion](https://github.com/pion/webrtc))
- with the possibility to record and optionnally transform video and audio streams thanks to [GStreamer](https://gstreamer.freedesktop.org/)

## DuckSoup server overview

A DuckSoup server exposes the following:

- an HTTP static server for `ducksoup.js` and example front-ends (TCP)
- an HTTP websocket endpoint for signaling (TCP)
- WebRTC (UDP)

Using the client library `ducksoup.js` is the preferred way to interact with DuckSoup server (regarding signaling and WebRTC).

## DuckSoup player

Let's assume we have a DuckSoup server installed and running at `ducksoup-host.com` and we want to embed a DuckSoup player in a webpage served from `my-experiment-host.com`.

The embedding origin (`my-experiment-host.com`) has to be listed as an authorized origin when starting the DuckSoup instance available at `ducksoup-host.com` (see [Environment variables](#environment-variables)).

Then, on the experiment web page, include the `ducksoup.js` library:

```
<script src="https://ducksoup-host.com//scripts/lib/ducksoup.js"></script>
```

And render it (in JavaScript):

```
const dsPlayer = await DuckSoup.render(embedOptions, peerOptions);
```

Where:

- assigning to a variable (`dsPlayer` above) is only needed if you want to further control the DuckSoup audio/video player instance (see [Player API](#player-api))

- `embedOptions` (object) must define `mountEl` or `callback` (or both):

  - `mountEl` (DOM node, obtained for instance with `document.getElementById("ducksoup-root")`): set this property if you want the player to automatically append `<audio>` and `<video>` HTML elements to `mountEl` for each incoming audio or video stream. If you want to manage how to append and render tracks in the DOM, don't define `mountEl` and prefer `callback` 
  - `callback` (JavaScript function) to receive events from DuckSoup in the form `({ kind, payload }) => { /* callback body */ }`. The different `kind`s of events the player may trigger are:
    - `"track"` (payload: [RTCTrackEvent](https://developer.mozilla.org/en-US/docs/Web/API/RTCTrackEvent)) when a new track sent by the server is available. This event is used to render the track to the DOM, It won't be triggered if you defined `mountEl`
    - `"start"` (no payload) when videoconferencing starts
    - `"ending"` (no payload) when videoconferencing is soon ending
    - `"end"` (no payload) when videoconferencing has ended
    - `"disconnection"` (no payload) when communication with server has stopped
    - `"error-join"` (no payload) when `peerOptions` (see below) are incorrect
    - `"error-duplicate"` (no payload) when a user with same `userId` (see `peerOptions` below) is already connected
    - `"error-full"` (no payload) when the videoconference room is full
    - `"error` with more information in payload
    - `"stats"` (payload contains bandwidth usage information) periodically triggered (fired only when `debug` is set to true)
  - `debug` (boolean, defaults to false) to enable `"stats"` messages to be received by callback

- `peerOptions` (object) must contain the following properties:

  - `signalingUrl` (string) the URL of DuckSoup signaling websocket (for instance `wss://ducksoup-host.com/ws` for a DuckSoup hosted at `ducksoup-host.com`) 
  - `roomId` (string) the room identifier
  - `userId` (string) a unique user identifier

- `peerOptions` may contain the following optional properties:

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

For a usage example, you may have a look at `front/src/test/app.js`

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

If DuckSoup is running and accessible for instance at http://localhost:8000, there are a few available test front-ends:

- http://localhost:8000/test/mirror/ one user reflection with a form to set peerOptions
- http://localhost:8000/test/room/ choose a user name, room name and size and open in multiple tabs (same number as room size)

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

Depending on the GStreamer plugins used, additional dependencies may be needed (opencv, dlib...).

### Environment variables

- DS_PORT=9000 (defaults to 8000) to set port listen by server
- DS_WEB_PREFIX=/path (defaults to none) if DuckSoup server is behind a proxy and reachable at https://ducksoup-host.com/path
- DS_ORIGINS=https://origin1,https://origin2:8080 (defaults to none) declares comma separated allowed origins for WebSocket connections
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

If a plugin can't be found, you may check:

- it's located in `$GST_PLUGIN_PATH`
- it's not been blacklisted (for instance if in a previous version a dynamic dependency is missing) by seeing the output of `gst-inspect-1.0 -b`
- if it has been blacklisted, one solution may be to delete GStreamer cache (possibly under `/root/.cache`)

### Concepts in Go code

On each connection to the websocket endpoint in `server.go` a new peerServer (see `peer_server.go`) is created:

- it manages further client communication through a TCP websocket (`ws_conn.go`) and a RTC Peer Connection (`peer_conn.go`)
- then it joins (or creates if necessary) a room (`trial_room.go`). Rooms manage the user logic (accept/reject user, deal with reconnections) and the trial sequencing and hold necessary data (for instance the list of recorded files)
- each room holds one reference to a mixer (`mixer.go`) that implements the SFU part: the mixer manages tracks attached to peer connections, handles signaling and RTCP feedback

For a given user connected to the room, there is one peerServer (abbreviated `ps` in the code), one wsConn (`ws`) and one peerConn (`pc`).

Depending on its size, a room may hold references to several peerServers.

Each peerConn has several tracks:

- remote: 2 (audio and video) client->server tracks
- local: 2*(n-1) server->client tracks for a room of size n (peers don't receive back their own streams)

### Step by step description of a run

Here is an overview of what is happening from connecting to videoconferencing:

- peer P1 connects to the signaling endpoint of DuckSoup, specifying a `trialRoom` ID
- the `trialRoom` is created (or joined) and a `peerServer` is launched to deal with further communication between the server and P1
- in particular `peerServer` creates a `peerConn` initiliazed with 2 transceivers for P1 audio and video tracks
- a first signaling round (S0) occurs to negotiate these tracks
- the `trialRoom` (in charge of users/peers) initializes a `mixer` (~ the SFU, in charge of peer connections, tracks, processing and signaling)
- at some point following S0, an incoming/remote track for P1 is received (see `OnTrack` in `peer_conn.go` ), then a resulting (processed) `localTrack` is created
- the `localTrack` struct contains a GStreamer pipeline and a few methods to control the processing of the pipeline
- the `localTrack` is then embedded within a `mixerSlice` which implements the network-aware behavior for this track, estimating its optimal encoding bitrate depending on network conditions
- the `mixerSlice` is added to the `mixer` of the `trialRoom` containing other peers. Each peer is represented by two `mixerSlice`s (one for audio, one for video), the `mixer` contains the `mixerSlice`s of all peers
- once all localTracks expected for all peers are ready (2 tracks * number of peers) , the `trialRoom` asks the `mixer` to update signaling:
  1. P1 output tracks are added to the other peers connections (and vice versa)
	2. new offers are created and sent to update remote peer connections (in the browser)
- a by-product of this signaling step is the initialization of `senderControllers` needed by `mixerSlices` to inspect network conditions and estimate optimal bitrates

### Logs

Logs are prefixed by:

- `[fatal]`: can't launch server
- `[error]`: server error
- `[info]`: information about the trial events (initialization of resources, signaling, closing...)
- `[wrong]`: client side error (for instance wrong `peerOptions`)
- `[recov]`: recover from panic

### Websocket messages

Messages from server (Go) to client (JS):

- kind `offer` and `candidate` for signaling (with payloads)
- kind `start` when all peers and tracks are ready
- kind `ending` when the room will soon be destroyed
- kind `end` when time is over (payload contains an index of media files recorded for this experiment)
- kind `error-full` when room limit has been reached and user can't enter room
- kind `error-duplicate` when same user is already in room
- kind `error-join` when `peerOptions` passed to DuckSoup player are incorrect
- kind `error-peer-connection` when server-side peer connection can't be established

### Code within a Docker container

One may develop DuckSoup in a container based from `docker/Dockerfile.code` (for instance using VSCode containers integration).

This Dockerfile prefers specifying a Debian version and installing go from source (rather than using the golang base image) so it's possible to choose the same OS version than in production and control gstreamer (apt) packages versions.

If you want to disable dlib compilation within the vscode Docker container, change the `build.args` property of `.devcontainer/devcontainer.json`.

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

## Using Docker

It is possible to build DuckSoup server from source within your preferred environment as long as you install the dependencies described in [Build from source](#build-from-source).

One may prefer relying on Docker to provide images with everything needed to build and run DuckSoup. Two options are suggested:

1. start from a debian image and install dependencies using apt: `docker/from-packages/Dockerfile.code` is provided as such an example
2. use the custom [creamlab/bullseye-gstreamer](https://hub.docker.com/repository/docker/creamlab/bullseye-gstreamer) image published on Docker Hub and whose definition is available [here](https://github.com/creamlab/docker-gstreamer)

The first option is good enough to work, and one may prefer it to have a simple installation process but with package manager versions of GStreamer and Go.

The second option relies on the `creamlab/bullseye-gstreamer` base image, managed in a [separate repository](https://github.com/creamlab/docker-gstreamer), with the advantage of coming with a recompiled GStreamer (enabling nvidia nvcodec plugin), opencv and dlib, and possibly more recent versions of GStreamer and Go.

In this project, we use `creamlab/bullseye-gstreamer` as a base for:

- `docker/Dockerfile.code` defines the image used to run a container within vscode (Go is installed, but DuckSoup remains to be compiled by the developer when needed)
- `docker/Dockerfile.build.single` defines an image with Go installed and DuckSoup compiled
- `docker/Dockerfile.build.multi` defines a multi-stage build image: in the first stage Go is installed and DuckSoup is compiled, in the final stage we only keep DuckSoup binary

### DuckSoup "single" Docker image

Build image:

```
docker build -f docker/Dockerfile.build.single -t ducksoup:latest .
```

Supposing we use a `deploy` user for running the container, prepare the volume `data` target:

```
sudo chown -R deploy:deploy data
```

Run by binding to port *8100* (as an example), setting user and environment variables, mounting volumes and removing the container when stopped:

```
docker run --name ducksoup_1 \
  -p 8100:8000 \
  -u $(id deploy -u):$(id deploy -g) \
  -e GST_DEBUG=2 \
  -e DS_ORIGINS=http://localhost:8100 \
  -v "$(pwd)"/plugins:/app/plugins:ro \
  -v "$(pwd)"/data:/app/data \
  --rm \
  ducksoup:latest
```

To enter the container:

```
docker exec -it ducksoup_1 bash
```

### DuckSoup multi-stage Docker image

If the goal is to distribute and minimize the image size, consider the multi-stage image built with:

```
docker build -f docker/Dockerfile.build.multi -t ducksoup_multi:latest .
```

Supposing we use a `deploy` user for running the container, prepare the volume `data` target:

```
sudo chown -R deploy:deploy data
```

Run by binding to port *8100* (as an example), setting user and environment variables, mounting volumes and removing the container when stopped:

```
docker run --name ducksoup_multi_1 \
  -p 8100:8000 \
  -u $(id deploy -u):$(id deploy -g) \
  -e GST_DEBUG=2 \
  -e DS_ORIGINS=http://localhost:8100 \
  -v "$(pwd)"/plugins:/app/plugins:ro \
  -v "$(pwd)"/data:/app/data \
  --rm \
  ducksoup_multi:latest
```

To enter the container:

```
docker exec -it ducksoup_multi_1 bash
```

As an aside, this multi-stage image is published on Docker Hub as `creamlab/ducksoup`, let's tag it and push it:

```
docker tag ducksoup_multi creamlab/ducksoup
docker push creamlab/ducksoup:latest
```

The `docker/docker-compose.yml` example relies on `creamlab/ducksoup`, let's to run it with docker-compose:

```
DS_USER=$(id deploy -u) DS_GROUP=$(id deploy -g) docker-compose -f docker/docker-compose.yml up --build
```

### GPU-enabled Docker containers

The nvcodec GStreamer plugin enables NVIDIA GPU accelerated encoding and decoding of H264 video streams.

When GStreamer is used within a Docker container, a few operations are necessary to access the host GPU from the container. One shoud refer to [nvidia-container-runtime](https://github.com/NVIDIA/nvidia-container-runtime) for up-to-date instructions, but as the time of this writing this may be summed up as:

- install `nvidia-container-runtime` on the Docker host (for instance with `sudo apt-get install nvidia-container-runtime`)
- on the host, edit or add a `runtimes` section in `/etc/docker/daemon.json`:
```json
{
    "runtimes": {
        "nvidia": {
            "path": "/usr/bin/nvidia-container-runtime",
            "runtimeArgs": []
        }
    }
}
```
- restart docker:
```
sudo systemctl daemon-reload
sudo systemctl restart docker
```
- set the desired NVIDIA capabilities within the container thanks to a few [environment variables](https://github.com/NVIDIA/nvidia-container-runtime#environment-variables-oci-spec). Regarding DuckSoup, the `creamlab/bullseye-gstreamer` base image has these already set (so this step should not be necessary)
- run the container with GPU enabled:
```
docker run --name ducksoup_multi_1 \
  --gpus all \
  -p 8100:8000 \
  -u $(id deploy -u):$(id deploy -g) \
  -e GST_DEBUG=2 \
  -e DS_ORIGINS=http://localhost:8100 \
  -v "$(pwd)"/plugins:/app/plugins:ro \
  -v "$(pwd)"/data:/app/data \
  --rm \
  ducksoup_multi:latest
```

## Credits

Parts of DuckSoup result from interactions within the [pion](https://pion.ly/) commmunity in general, and from [Galène](https://github.com/jech/galene) in particular.