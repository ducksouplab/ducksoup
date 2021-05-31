# DuckSoup

Videoconferencing tool for social experiments.

From a technical standpoint, DuckSoup is:

* a videoconference server acting as a relay for peers in the same room (more precisely, a SFU made with Go and [pion](https://github.com/pion/webrtc))
* with the possibility to record and optionnally transform video and audio streams thanks to GStreamer

Once DuckSoup is installed and running, it may be configured and embedded in other webpages:

```
<iframe src="https://ducksoup-host.example.com/embed/?params=PARAMS_STRING" allow="camera;microphone"></iframe>
```

Where PARAMS_STRING is obtained by serializing a JS object that contains DuckSoup-related options.

Serializing is done with `encodeURI(btoa(JSON.stringify(params)))` where params:

- must contain:

  - origin (string) the embedding window origin (for instance `https://website-calling-ducksoup.example.com`)
  - uid (string) a unique user identifier
  - name (string) the user display name
  - room (string) the room display name
  - proc (boolean) to ask for media processing
  - duration (integer) the duration of the experiment in seconds

- may contain:

  - h264 (boolean) if h264 encoding should be preferred (vp8 is default)
  - audio (object) merged with DuckSoup default constraints and passed to getUserMedia (see [properties](https://developer.mozilla.org/en-US/docs/Web/API/MediaTrackConstraints#properties_of_audio_tracks))
  - video (object) merged with DuckSoup default constraints and passed to getUserMedia (see [properties](https://developer.mozilla.org/en-US/docs/Web/API/MediaTrackConstraints#properties_of_video_tracks))

Note: the embedding origin (for instance `https://website-calling-ducksoup.example.com` above) has to be listed as a valid origin when starting DuckSoup (see [Environment variables](#environment-variables)).

## Install and build

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
mkdir certs && cd certs && mkcert -key-file key.pem -cert-file cert.pem localhost 
```

Then build:

```
go build
```

## Environment variables

- ORIGINS=https://origin1,https://origin2:8000 declares comma separated allowed origins for WebSocket connections
- APP_ENV=DEV enables automatic front-end assets build with esbuild + adds http://localhost:8080 to allowed origins for WebSocket connections
- GST_PLUGIN_PATH to declare additional GStreamer plugin paths (prefer appending to the existing GST_PLUGIN_PATH: GST_PLUGIN_PATH="$GST_PLUGIN_PATH:$PROJECT_BUILD")

## Run DuckSoup

Run (with or without environment variables):

```
./ducksoup
APP_ENV=DEV ./ducksoup
ORIGINS= https://website-calling-ducksoup.example.com ./ducksoup
```

With TLS:

```
./ducksoup --cert certs/cert.pem --key certs/key.pem
```

## Test front-ends

Several test front-ends are available:

- static/test_embed showcases how to embed DuckSoup in a iframe and receive messages from it
- static/test_standalone is a sample project not relying on static/embed

Once the app is running, you may try it with:

- http://localhost:8080/test_embed/ (in several tabs)
- http://localhost:8080/test_standalone/ (in several tabs)

## Websocket messages

Messages from server (Go) to client (JS):

- kind `offer` and `candidate` for signaling (with payloads)
- kind `start` when all peers and tracks are ready
- kind `finishing` when the room will soon be destroyed
- kind `finish` when time is over (payload contains a concatenated list of media files recorded for this experiment)

## Front-ends build

Building js files (useful at least for bundling and browser improved compatibility, also for minification) is done with esbuild and triggered from go.

When `./ducksoup` is launched (see `front/build.go` to configure and build new front-ends), some js files are processed (from `front/src` to `front/static`).

It's also possible to watch changes and rebuild those files by adding this environment variable:

```
APP_ENV=DEV ./ducksoup
```

## Add custom GStreamer plugins in lib/

```
mkdir -p lib
export PROJECT_BUILD=`pwd`/lib
export GST_PLUGIN_PATH="$GST_PLUGIN_PATH:$PROJECT_BUILD"
```

## Run with Docker

Generate certs (see above) and then:

```
docker build -t ducksoup:latest .
docker container run -p 8080:8080 --rm ducksoup:latest
# or enter the container
docker container run -p 8080:8080 -it --entrypoint /bin/bash ducksoup:latest
```

To try without certs:

```
docker build -f docker/Dockerfile.no-tls -t ducksoup:latest .
docker container run -p 8080:8080 -rm ducksoup:latest
```

## Issues with Docker

`Dockerfile.multi-*` are intended to build multi-layered Docker images, separating building step _and_ dependencies from the final running environment. It currently does not work (INVESTIGATION NEEDED)

Hint for multi-debian: debug go execution, and check for relevant gstreamer runtime dependencies (try to add same apt dependencies in build and run stages, then clean up)

Hint for multi-alpine: apparent missing dependency to be found (https://superuser.com/questions/1176200/no-such-file-when-it-exists). Maybe easier to fix multi-debian first. See https://github.com/pion/ion/blob/master/docker/sfu.Dockerfile

## Concepts in Go code

On each connection to the websocket endpoint in `server.go` a new PeerServer (see `peer_server.go`) is created:

- it manages further client communication through websocket (see `ws_conn.go`) and RTC (see `peer_conn.go`)
- join (create if necessary) room which manages the logical part (if room is full, if there is a disconnect/reconnect from same peer...)

Thus PeerServer struct holds a reference to a Room, and each Room has references to several PeerServers.
