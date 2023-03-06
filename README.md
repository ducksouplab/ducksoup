# DuckSoup

Videoconferencing tool for social experiments.

From a technical standpoint, DuckSoup is:

- a videoconference server acting as a relay for peers (more precisely, a SFU made with Go and [pion](https://github.com/pion/webrtc))
- with the possibility to record and optionnally transform video and audio streams thanks to [GStreamer](https://gstreamer.freedesktop.org/)


*The companion repository [deploy-ducksoup](https://github.com/ducksouplab/deploy-ducksoup) documents a possible DuckSoup deployment workflow relying on Docker Compose.*

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
<script src="https://ducksoup-host.com/assets/scripts/lib/ducksoup.js"></script>
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
    - `"files"` with a list of recording files for this peer. This event is emitted when recording is over and may be treated as an `"end"` event.
    - `"closed"` (no payload) when websocket is closed
    - `"error-join"` (no payload) when `peerOptions` (see below) are incorrect
    - `"error-duplicate"` (no payload) when a user with same `userId` (see `peerOptions` below) is already connected
    - `"error-full"` (no payload) when the videoconference interaction is full
    - `"error-aborted"` (no payload) when other peers have not joined the room after too long (timeout)
    - `"error` with more information in payload
    - `"stats"` (payload contains bandwidth usage information) periodically triggered (fired only when `stats` is set to true)
  - `stats` (boolean, defaults to false) to enable `"stats"` messages sent to client callback (please note that stats are polled every second)

- `peerOptions` (object) must contain the following properties:

  - `signalingUrl` (string) the URL of DuckSoup signaling websocket (for instance `wss://ducksoup-host.com/ws` for a DuckSoup hosted at `ducksoup-host.com`) 
  - `interactionName` (string) the interaction identifier
  - `userId` (string) a unique user identifier

- `peerOptions` may contain the following optional properties:

  - `duration` (integer, defaults to 30) the duration of the experiment in seconds
  - `size` (integer, defaults to 2) the number of participants (size == 1 for a mirror effect)
  - `width` (integer, defaults to 800) of the video stream
  - `height` (integer, defaults to 600) of the video stream
  - `frameRate` (integer, defaults to 30) of the video stream
  - `audioFx` (string, see format in [Gstreamer effects](#gstreamer-effects)) if an audio effect has to be applied
  - `videoFx` (string, see format in [Gstreamer effects](#gstreamer-effects)) if video effect has to be applied
  - `audio` (object) merged with DuckSoup default constraints and passed to getUserMedia (see [properties](https://developer.mozilla.org/en-US/docs/Web/API/MediaTrackConstraints#properties_of_audio_tracks))
  - `video` (object) merged with DuckSoup default constraints and passed to getUserMedia (see [properties](https://developer.mozilla.org/en-US/docs/Web/API/MediaTrackConstraints#properties_of_video_tracks))
  - `videoFormat` (string) possible values: "H264" (default if none) or "VP8"
  - `recordingMode` (string) possible values: `muxed` (default if none, records audio/video in the same muxed file), `split` (records separate files for audio and video), `passthrough` (records input streams and sends them back, without applying any fx or reencoding) or `none` (no recording)
  - `rtcConfig` ([RTCConfiguration dictionary](https://developer.mozilla.org/en-US/docs/Web/API/RTCPeerConnection/RTCPeerConnection#rtcconfiguration_dictionary) object) used when creating an RTCPeerConnection, for instance to set iceServers
  - `namespace` (string, defaults to "default") to group recordings under the same namespace (folder)
  - `gpu` (boolean, defaults to false) enable hardware accelarated h264 encoding and decoding (and other cuda accelerated plugins like raw video [conversions](https://gstreamer.freedesktop.org/documentation/nvcodec/cudaconvertscale.html)), if relevant hardware is available on host and if DuckSoup is launched with the `DUCKSOUP_NVCODEC=true` environment variable (see [Environment variables](#environment-variables))
  - `logLevel` (int, defaults to 1):
    - 0: no client logs sent to server
    - 1: logs related to RTP stats (bitrates, fps, keyframes...) are sent to server
    - 2: above + logs related to signaling are sent to server
    - please note that logs relying on WebRTC stats data are only polled every second, meaning some data samples may be missing
  - `overlay` (boolean, defaults to false) add text overlay on top of the video (mainly for debugging purposes)

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

- to name the effect described in `audioFx` or `videoFx` by adding a unique `name` property, for instance `"element property1=1.0 name=fx"`
- call the player `controlFx` method, for instance `ds.controlFx("fx", "property1", 1.2, 500)`

In this example, `proprety1` has an initial value of `1.0` and is updated to `1.2`, with a linear interpolation over 500 ms. If the last parameter is ommitted (transition duration), the update is instantaneous.

For the time being only float values are allowed when controlling properties.

### Player API

Instantiation is an async operation : `const dsPlayer = await DuckSoup.render(mountEl, peerOptions, embedOptions);`

The following methods are available on a DuckSoup player:

- `controlFx(effectName, property, value, transitionDuration)` to update the property of the effect named in `peerOptions#audioFx`. For instance with an `audioFx` of `"element property1=1.0 name=fx"`:
  - `effectName` (string) is `fx`
  - `property` (string) is `property1`
  - `value` (float) sets a new value, for instance `1.1`
  - `transitionDuration` (integer counting ms, defaults to 0, expect better results for 200 and above) is the optional duration of the interpolation between the old and new values
- `stop()` to stop media streams and close communication with server. Note that players are running for a limited duration (set by `peerOptions#duration` which is capped server-side) and most of the time you don't need to use this method
- `log(kind, payload)` to generate a server-side log (`kind` and `payload` will be stringified, `payload` is optional)

### Front-ends

If DuckSoup is running and accessible for instance at http://localhost:8100, there are a few available test front-ends:

- http://localhost:8100/test/play/ one user reflection with live UX for effects
- http://localhost:8100/test/mirror/ one user reflection with peerOptions control and debug information
- http://localhost:8100/test/interaction/ choose a user name, interaction name and size and open in multiple tabs (same number as interaction size)

A stats page displaying raw information about current interactions and bandwidth stats is accessible at (currently under work):

- http://localhost:8100/stats/

## DuckSoup server

### Build

If you are using one of the provided Docker images, you don't need to install binary dependencies (Go, GStreamer, yarn).

To build DuckSoup:

```
go build
```

If you're not using Docker, you have to install those dependencies first:

- [Go](https://golang.org/doc/install)
- [GStreamer](https://gstreamer.freedesktop.org/documentation/index.html?gi-language=c)
- [yarn](https://yarnpkg.com/)

Regarding GStreamer on Debian you may:

```
apt-get install libgstreamer1.0-0 gstreamer1.0-plugins-base gstreamer1.0-plugins-good gstreamer1.0-plugins-bad gstreamer1.0-plugins-ugly gstreamer1.0-libav gstreamer1.0-doc gstreamer1.0-tools gstreamer1.0-x gstreamer1.0-alsa gstreamer1.0-gl gstreamer1.0-gtk3 gstreamer1.0-qt5 gstreamer1.0-pulseaudio
apt-get install libgstreamer1.0-dev libgstreamer-plugins-base1.0-dev
```

Depending on the GStreamer plugins used, additional dependencies may be needed (opencv, dlib...).

### Front-end dependencies

If you launch DuckSoup server with one of these options (see more in next paragraph):

```
DUCKSOUP_MODE=DEV ./ducksoup
DUCKSOUP_MODE=FRONT_BUILD ./ducksoup
```

Then DuckSoup will rebuild/bundle/minify JS assets (thanks to [esbuild](https://esbuild.github.io/)) needed by the different [Front-ends](#front-ends). The effect is to process js files from `front/src` to `front/static`.

Since the `/test/play/` front-end requires additional JS modules (React for instance), it is required that you fetch them before launching DuckSoup. Fetch front-end dependencies with:

```
yarn
```

In particular, if DuckSoup instantaneously crashes with a `JS build fatal error: Could not resolve "..."` error message, it means front-end dependencies need to be installed with yarn.

### Settings

When changing settings (either as environment variables or defined in `config/*.yml` files) one needs to restart the DuckSoup server so that changes are taken into account.

Security related settings and settings defining how DuckSoup is run on host are controlled by environment variables:

- `DUCKSOUP_MODE=DEV` enables automatic front-end assets build + adds a few allowed origins for WebSocket connections + changes log format (adds the `file:line` of caller) + print logs to Stdout
- `DUCKSOUP_PORT=8000` (defaults to 8100) to set port listen by server
- `DUCKSOUP_WEB_PREFIX=/path` (defaults to none) if DuckSoup server is behind a proxy and reachable at https://ducksoup-host.com/path
- `DUCKSOUP_PUBLIC_IP` (defaults to none) if set, will be used to add a Host candidate during signaling (not necessary if ICE servers are used)
- `DUCKSOUP_ALLOWED_WS_ORIGINS=https://origin1,https://origin2:8180` (defaults to none) declares comma separated allowed origins for WebSocket connections
- `DUCKSOUP_TEST_LOGIN` (defaults to "ducksoup") to protect test and stats pages with HTTP authentitcation
- `DUCKSOUP_TEST_PASSWORD` (defaults to "ducksoup") to protect test and stats pages with HTTP authentitcation
- `DUCKSOUP_MODE=FRONT_BUILD` builds front-end assets but do not start server
- `DUCKSOUP_NVCODEC` (default to false) set to true if NVIDIA GPU is enabled on the host (see [GPU-enabled Docker containers](#gpu-enabled-docker-containers)) and relevant GStreamer [nvcodec](https://gstreamer.freedesktop.org/documentation/nvcodec/index.html) plugin is preferred for video encoding (rather than relying on the CPU)
- `DUCKSOUP_GENERATE_TWCC=true` (default to false) enables RTCP TWCC reports generated by DuckSoup and sent to browser. As as side effect, 
- `DUCKSOUP_GCC=true` (default to false, meaning bandwith estimation is done relying on RTCP Receiver Reports) enables GCC bandwidth estimation
- `DUCKSOUP_GST_TRACKING=true` (default to false) enabled GStreamer log processing (you are most likely not interested in that option)
- `DUCKSOUP_LOG_FILE=log/ducksoup.log` (defaults to none) to declare a file to write logs to (fails silently if file can't be opened)
- `DUCKSOUP_LOG_STDOUT=true` (defaults to false, except when `DUCKSOUP_MODE=DEV`) to print logs to Stdout:
  - if `DUCKSOUP_LOG_FILE` is also set, logs are written to both
  - if neither are set, logs are written to Stderr 
- `DUCKSOUP_LOG_LEVEL` (defaults to 3) to select log level display (see next section)
- `DUCKSOUP_FORCE_OVERLAY` (defaults to false) set to true to display a time overlay in videos (recorded)
- `DUCKSOUP_ICE_SERVERS=false` (defaults to `stun:stun.l.google.com:19302`) declares comma separated allowed STUN servers to be used to find ICE candidates (or false to disable STUN)

Since DuckSoup relies on GStreamer, GStreamer environment variables may be useful, for instance:

- `GST_PLUGIN_PATH` to declare additional GStreamer plugin paths (prefer appending to the existing GST_PLUGIN_PATH: GST_PLUGIN_PATH="$GST_PLUGIN_PATH:/additional/plugins/path")
- `GST_DEBUG` to control GStreamer debug output

Ducksoup settings related to GStreamer pipelines are defined in `config/gst.yml`:

- `rtpjitterbuffer` defines properties passed to the [rtpjitterbuffer](https://gstreamer.freedesktop.org/documentation/rtpmanager/rtpjitterbuffer.html#properties) plugin
- `vp8`, `x264`, `nv264` and `opus` define codec settings, `nv264` being preferred to `x264` depending on `DUCKSOUP_NVCODEC` (and `gpu` on `peerOptions`).

DuckSoup server settings are defined in `config/server.yml`:

- `generateStats` set to `true` to generate and expose server stats (see [Front-ends](#front-ends))

DuckSoup SFU settings are defined in `config/sfu.yml`:

- `audio` defines min/max/default values of target bitrates for output (reencoded) audio tracks
- `video` defines min/max/default values of target bitrates for output (reencoded) video tracks

### DUCKSOUP_MODE=DEV and .env file

If you have a `.env` file at the root of the project (you may copy/paste/edit the provided `env.example`) and **if `DUCKSOUP_MODE=DEV`**, then all the variables defined in `.env` will be accessible to DuckSoup.

Indeed, you may prefer editing this `.env` file (over defining all the environment variables in the command line) and then run:

```
go build && DUCKSOUP_MODE=DEV GST_DEBUG=2,videodecoder:1 ./ducksoup
```

A few important remarks:

- this feature is only enabled when `DUCKSOUP_MODE=DEV` (meaning `DUCKSOUP_MODE` is defined before/independently from `.env`)
- it only works for DuckSoup (not for GStreamer, that's why `GST_DEBUG` is still set in the example above)
- `.env` is loaded by `helpers/init.go`, that's why the `helpers` package is imported by other packages that use environment variable
- `.env` is not bundled in the Docker images documented below and is only meant as a development feature

Nevertheless, using `.env` files in production may also be interesting. A solution relying on Docker Compose is documented [here](https://github.com/ducksouplab/deploy-ducksoup#environment-variables).

### Logs configuration

Logs are managed with [zerolog](https://github.com/rs/zerolog):

- they are pretty-printed to stdout if `DUCKSOUP_LOG_STDOUT=true` or `DUCKSOUP_MODE=DEV`
- if you define a log file (`DUCKSOUP_LOG_FILE=log/ducksoup.log` for instance) they are appended to this file as JSON entries

Depending on `DUCKSOUP_LOG_LEVEL`, here are the generated logs (the default value is `2`):

- `0` no log
- `1` errors (and GStreamer warnings)
- `2` server related info and above
- `3` client related info, in/out/encoding bitrates info and above
- `4` debug logs (including TWCC reports) and above

Please note that while we rely on zerolog, we don't use the same semantics regarding levels, their index and meaning.

GStreamer logs are intercepted and sent to DuckSoup in order to have them centralized, facilitating further analysis. Nevertheless, what logs GStreamer generates is still controlled by the `GST_DEBUG` environement variable (independent from `DUCKSOUP_LOG_LEVEL`). Here is an example to hide video decoding warnings: `GST_DEBUG=2,videodecoder:1`.

### Logs format

This section details how logs can be parsed, each entry being stored as a JSON object.

First of all, each log has the following properties:
- `level`: useful to separate errors (`level: "error"`) from other types (`"info"`, `"debug"`, `"trace"`)
- `time`: the log timestamp (`"20060102-150405.000"`)
- `context`: used to categorize logs (see more below)
- `message`: a unique string that describes the event that generated the log (`"interaction_end"` for instance, see [Log message reference](#logs-message-reference))

Here are some optional but frequent properties:
- `value`: depending on the log, a `value` may convey additional data
- `unit`: sometimes needed to illustrate `value`'s meaning 
- `error`: Go error's string explaining `level: "error"` logs
- `source: "client"`: present only if log has been generated as is by the client (ducksoup.js). Please note that when a log has a `message` property that starts with `client_`, it means the log is related to the client/remote peer. But this log may be generated either client or server-side. In that case, the `source` property helps distinguish between the two.

Now let's list all the possible `context`s:

- `"peer"`: related to overall peer communication (websocket, peer connection) 
- `"interaction"`: related to interaction (creation, end, adding tracks...) 
- `"track"`: related to peer media tracks
- `"pipeline"`: related to processing pipelines attached to tracks
- `"signaling"`: related to peer webrtc signaling
- `"gstreamer"`: GStreamer logs
- `"init"`: logs occuring when app initializes
- `"app"`: app-level logs
- `"server"`: HTTP server logs
- `"js_build"`: for esbuild messages (happen only when building [Front-end dependencies](#front-end-dependencies))
- `"ext"`: logs generated by external/client app that uses DuckSoup

Logs whose context is either `"peer"`, `"interaction"`, `"track"`, `"pipeline"` or `"signaling"` embed the following properties:

- `"namespace"`: a namespace used by DuckSoup client to categorize the experiment
- `"interaction"`: interaction id
- `"user"`: user id
- `"sinceCreation"`: elapsed time since interaction creation
- `"sinceStart"`: elapsed time since interaction start

### Logs message reference

Here is a reference of all log messages, grouped by context:

`peer` context:

- `message: "websocket_upgraded"`: websocket upgrade granted for the given `origin` property
- `message: "peer_server_started"`: peer server (websocket and RTC peer connection) started (after a websocket join event)
- `message: "peer_server_ended"`: peer server ended (additional `cause` property)
- `message: "interaction_ending_sent"`: interaction "ending" websocket message sent to peer

`interaction` context:

- `message: "interaction_created"`: interaction created by given user (additional `origin` property)
- `message: "peer_joined"`: user joined interaction (additional `payload` property)
- `message: "in_track_added"`: incoming peer track added to interaction (when enough tracks have been added, interaction is ready to start)
- `message: "interaction_started"`: when all peers and tracks are ready
- `message: "interaction_ended"`: interaction ended (interaction time limit has been reached)
- `message: "interaction_deleted"`: occurs after interaction has ended and all users have disconnected. Or occur even if interaction was not started (not enough users)

`track` context:

- `message: "in_track_received"`: remote/incoming audio track added to server peer connection (additional properties: `track`'s ID, `ssrc`, `mime`, `type`: `audio` or `video`)
- `message: "client_fx_control"`: JS client has requested an update of a GStreamer fx (identified by `name`, updated with `property` and `value`) 
- `message: "audio_in_bitrate"`: estimated input bitrate of incoming track as described by `value` and `unit` propeties
- `message: "video_in_bitrate"`: same for video
- `message: "audio_target_bitrate_updated"`: new target bitrate of encoder for outgoing track as described by `value` and `unit` propeties
- `message: "video_target_bitrate_updated"`: same for video
- `message: "audio_out_bitrate"`: estimated output bitrate of outgoing track as described by `value` and `unit` propeties
- `message: "video_out_bitrate"`: same for video
- `message: "loss_threshold_exceeded"`: too many lost packets (property `value` reflects ReceiverReport loss count)
- `message: "out_track_stopped"`: processed track (server-side, with given `track` ID and `kind` properties) stopped after pipeline stopped
- `message: "pli_sent"`: Picture Loss Indication sent to client (additional `cause` property)
- `message: "pli_skipped"`: Picture Loss Indication skipped (throttling, additional `cause` property)
- `message: "audio_in_report"`: describe audio `lost` RTP packets (coming from client) among `count` (total) RTP packets emitted by client (since last report)
- `message: "video_in_report"`: same for video
- `message: "client_video_resolution_updated"`
- `message: "client_pli_received_count_updated"`
- `message: "client_fir_received_count_updated"`
- `message: "client_keyframe_encoded_count_updated"`
- `message: "client_keyframe_decoded_count_updated"`
- `message: "client_message"`: free message sent by JS client

`pipeline` context:

- `message: "pipeline_created"`: pipeline (associated to track) has been created
- `message: "pipeline_started"`: pipeline started (additional property `recording_prefix` giving recorded files prefixes)
- `message: "pipeline_stopped"`: pipeline stopped (for instance when interaction ends)
- `message: "pipeline_deleted"`: pipeline deleted
- `message: "gstreamer_pli_requested"`: Picture Loss Indication emitted by GStreamer pipeline associated to the track

`signaling` context, mostly used to debug signaling, among:

- `message: "server_signaling_state_changed"`: see possible [values](https://pkg.go.dev/github.com/pion/webrtc/v3@v3.1.56#SignalingState)
- `message: "{client_or_server}_selected_candidate_pair"`: logs ice candidate selected pair when signaling is stable (for server) or when `selectedcandidatepairchange` (client)
- `message: "server_create_offer_requested"`: signaling update (additional `cause` property)
- `message: "duplicate_track_skipped"`: track already added to peer connection
- `message: "own_track_skipped"`: own track not to be sent back to originating peer (except for mirror interaction)

`app` context:

- `message: "app_started"`
- `message: "app_ended"` (main function has ended)
- `message: "app_panicked"` (panic recovered in main function), additional information in the `message` property

`server` context:

- `message: "not_found"`

Regarding `gstreamer` context, logs are forwarded from GStreamer to DuckSoup and `message`s are free text generated by GStreamer.

A few additional messages exist, they should not occur (they imply a DuckSoup bug or a GStreamer error):

- `message: "pipeline_not_found"`: GStreamer processing can't be mapped to a Go pipeline
- `message: "track_write_failed"`: can't write to RTP output track
- `message: "gstreamer_pipeline_error"`: a GStreamer error associated to the given Go pipeline

Finally, `ext` context: free-form messages generated by outer webapp that uses DuckSoup (through ducksoup.js). Whenever the `log` method of the DuckSoup player is called, a log is created. For instance :

- calling `dsPlayer.log("user_event", "inactive");` (in JS client app)...
- ...will generate a log with the following properties:
    - `context: "ext"`
    - `source: "client"`
    - `message: "ext_user_event"` (`ext_` prefix is added to avoid nameclashes with other declared messages)
    - `payload: "inactive"`

### Run DuckSoup server

Note: please read the [Front-end dependencies](#front-end-dependencies) section first. It explains why installing front-end dependencies with yarn is required depending on `DUCKSOUP_MODE`.

Run (without DUCKSOUP_MODE=DEV nor DUCKSOUP_ALLOWED_WS_ORIGINS, signaling can't work since no accepted WebSocket origin is declared):

```
DUCKSOUP_MODE=DEV ./ducksoup
DUCKSOUP_ALLOWED_WS_ORIGINS=https://ducksoup-caller-host.com ./ducksoup
```

An example to build and run with a few settings:

```
go build && GST_DEBUG=2,videodecoder:1 DUCKSOUP_NVCODEC=true DUCKSOUP_MODE=DEV ./ducksoup
```

The following shortcut build and run DuckSoup with useful development settings:

```
make dev
```

To serve with TLS in a local setup, you may consider [mkcert](https://github.com/FiloSottile/mkcert) to generate certificates. With mkcert installed:

```
mkdir certs && cd certs && mkcert -key-file key.pem -cert-file cert.pem localhost 
```

Run with TLS:

```
DUCKSOUP_MODE=DEV ./ducksoup --cert certs/cert.pem --key certs/key.pem
DUCKSOUP_ALLOWED_WS_ORIGINS=https://ducksoup-caller-host.com ./ducksoup --cert certs/cert.pem --key certs/key.pem
```

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
- then it joins (or creates if necessary) an interaction (`interaction.go`). Interactions manage the user logic (accept/reject user, deal with reconnections) and the trial sequencing and hold necessary data (for instance the list of recorded files)
- each interaction holds one reference to a mixer (`mixer.go`) that implements the SFU part: the mixer manages tracks attached to peer connections, handles signaling and RTCP feedback

For a given user connected to the interaction (abbreviated `i` in the code), there is one peerServer (`ps`), one wsConn (`ws`) and one peerConn (`pc`).

Depending on its size, an interaction may hold references to several peerServers.

Each peerConn has several tracks:

- remote: 2 (audio and video) client->server tracks
- local: 2*(n-1) server->client tracks for an interaction of size n (peers don't receive back their own streams)

When an interaction is done (only if aborted or successfully ended), the following resources are released too:

- mixer, mixerSlices, senderControllers
- peerServer, peerConn, wsConn

There are several ways a peerServer may end:

- the room is done
- an error occured on wsConn
- the peer connection has been closed

When releasing resources related to a peer, it's possible to test only for the peerServer to be done, since the room being done implies peerServers to be done.

### Step by step description of a run

Here is an overview of what is happening from connecting to videoconferencing:

- peer P1 connects to the signaling endpoint of DuckSoup, specifying a `interaction` ID
- the `interaction` is created (or joined) and a `peerServer` is launched to deal with further communication between the server and P1
- in particular `peerServer` creates a `peerConn` initiliazed with 2 transceivers for P1 audio and video tracks
- a first signaling round (S0) occurs to negotiate these tracks
- the `interaction` (in charge of users/peers) initializes a `mixer` (~ the SFU, in charge of peer connections, tracks, processing and signaling)
- at some point following S0, an incoming/remote track for P1 is received (see `OnTrack` in `peer_conn.go` ), then a resulting (processed) `mixerSlice` is created
- the `mixerSlice` struct contains a GStreamer pipeline and a few methods to control the processing of the pipeline
- the `mixerSlice` is added to the `mixer` of the `interaction` containing other peers. Each peer is represented by two `mixerSlice`s (one for audio, one for video), the `mixer` contains the `mixerSlice`s of all peers
- once all mixerSlices expected for all peers are ready (2 tracks * number of peers) , the `interaction` asks the `mixer` to update signaling:
  1. P1 output tracks are added to the other peers connections (and vice versa)
	2. new offers are created and sent to update remote peer connections (in the browser)
- a by-product of this signaling step is the initialization of `senderControllers` needed by `mixerSlices` to inspect network conditions and estimate optimal bitrates

### Websocket messages

Messages from server (Go) to client (JS):

- kind `offer` and `candidate` for signaling (with payloads)
- kind `start` when all peers and tracks are ready
- kind `ending` when the interaction will soon be destroyed
- kind `end` when time is over (payload contains an index of media files recorded for this experiment)
- kind `error-full` when interaction limit has been reached and user can't enter interaction
- kind `error-duplicate` when same user is already in interaction
- kind `error-join` when `peerOptions` passed to DuckSoup player are incorrect
- kind `error-aborted` when other peers have not joined the room after too long (timeout)
- kind `error-peer-connection` when server-side peer connection can't be established

### Code within a Docker container

One may develop DuckSoup in a container based from `docker/Dockerfile.code` (for instance using VSCode containers integration).

This Dockerfile prefers specifying a Debian version and installing go from source (rather than using the golang base image) so it's possible to choose the same OS version than in production and control gstreamer (apt) packages versions.

If you want to disable dlib compilation within the vscode Docker container, change the `build.args` property of `.devcontainer/devcontainer.json`.

### Run tests

Launch with:

```
make test
# verbose
make testv
```

It triggers tests in the project subfolders, setting appropriate environment variables for specific test behavior.

### Update all go deps

```
go get -t -u ./...
go mod tidy
# or use Makefile
make deps
```

## Using Docker

It is possible to build DuckSoup server from source within your preferred environment as long as you install the dependencies described in [Build from source](#build-from-source).

One may prefer relying on Docker to provide images with everything needed to build and run DuckSoup. Two options are suggested:

1. start from a debian image and install dependencies using apt: `docker/from-packages/Dockerfile.code` is provided as such an example
2. use the custom [ducksouplab/debian-gstreamer](https://hub.docker.com/repository/docker/ducksouplab/debian-gstreamer) image published on Docker Hub and whose definition is available [here](https://github.com/ducksouplab/docker-gstreamer)

The first option is good enough to work, and one may prefer it to have a simple installation process but with package manager versions of GStreamer and Go.

The second option relies on the `ducksouplab/debian-gstreamer` base image, managed in a [separate repository](https://github.com/ducksouplab/docker-gstreamer), with the advantage of coming with a recompiled GStreamer (enabling NVIDIA enabled nvcodec plugin), opencv and dlib, and possibly more recent versions of GStreamer and Go.

In this project, we use `ducksouplab/debian-gstreamer` as a base for:

- `docker/Dockerfile.code` defines the image used to run a container within vscode (Go is installed, but DuckSoup remains to be compiled by the developer when needed)
- `docker/Dockerfile.build` defines an image with Go installed and DuckSoup compiled

Please note that the official [DuckSoup image](https://hub.docker.com/r/ducksouplab/ducksoup) is built from `docker/Dockerfile.build`. This image is used in particular by [deploy-ducksoup](https://github.com/ducksouplab/deploy-ducksoup), a project that showcases a possible DuckSoup deployment workflow relying on Docker Compose.

### DuckSoup Docker image

Build image:

```
docker build -f docker/Dockerfile.build -t ducksoup:latest .
```

Supposing we use a `deploy` user to run the container, prepare `data` and `log` folders, to be mounted as volumes in the container:

```
mkdir data log
chown -R deploy:deploy data log
```

Run by binding to port *8101* (as an example), setting user and environment variables, mounting volumes and removing the container when stopped:

```
docker run --name ducksoup_1 \
  -p 8101:8100 \
  -u $(id deploy -u):$(id deploy -g) \
  -e GST_DEBUG=2 \
  -e DUCKSOUP_ALLOWED_WS_ORIGINS=http://localhost:8101 \
  -v $(pwd)/plugins:/app/plugins:ro \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/log:/app/log \
  --rm \
  ducksoup:latest
```

To enter the container:

```
docker exec -it ducksoup_1 bash
```

As an aside, this image is published on Docker Hub as `ducksouplab/ducksoup`, let's tag it and push it:

```
docker tag ducksoup ducksouplab/ducksoup
docker push ducksouplab/ducksoup:latest
```

With this image, `root` is the user that launches and owns files in the Docker container. The project [deploy-ducksoup](https://github.com/ducksouplab/deploy-ducksoup) shows a way to build a lightweight image on top of this one with another user.

### GPU-enabled Docker containers

The nvcodec GStreamer plugin enables NVIDIA GPU accelerated encoding and decoding of H264 video streams.

Here are a few considerations regarding Docker and NVIDIA:

- to start with, please check the list of Docker [host platforms supported by NVIDIA](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html#supported-platforms)

- regarding needed installations on the host:

  1. NVIDIA driver: first check if already installed (try `nvidia-smi`), if not search for available versions (`apt-cache search nvidia-driver`) and install (for instance with `apt-get install nvidia-driver-460`)
  
  2. NVIDIA and Docker: this [description](https://github.com/NVIDIA/nvidia-docker/issues/1268#issuecomment-632692949) tends to show that installing `nvidia-container-runtime` is sufficient to have Docker containers benefit from NVIDIA GPUs. To do so, update your host repository configuration following [these instructions](https://nvidia.github.io/nvidia-container-runtime/) and `apt-get install nvidia-container-runtime`

  3. Restart Docker (`systemctl restart docker`)

- set the desired NVIDIA capabilities within the container thanks to a few [environment variables](https://github.com/NVIDIA/nvidia-container-runtime#environment-variables-oci-spec). Regarding DuckSoup, the `ducksouplab/debian-gstreamer` base image has these already set (so this step should not be necessary)

- run the container with GPU enabled:

```
docker run --name ducksoup_gpu_1 \
  --gpus all \
  -u $(id deploy -u):$(id deploy -g) \
  -e GST_DEBUG=2 \
  -e DUCKSOUP_NVCODEC=true \
  -v $(pwd)/plugins:/app/plugins:ro \
  -v $(pwd)/data:/app/data \
  --rm \
  ducksoup:latest
```

## Credits

Parts of DuckSoup result from interactions within the [pion](https://pion.ly/) commmunity in general, and from [Galène](https://github.com/jech/galene) in particular.

The following STUN servers are used by the project: stun.l.google.com:19302 and stun:stun3.l.google.com:19302 (previously stun.stunprotocol.org:3478).