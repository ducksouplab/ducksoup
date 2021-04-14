# webrtc-transform

SFU made with [pion](https://github.com/pion/webrtc) with Gstreamer audio transformation.

Inspirations:

- https://github.com/pion/example-webrtc-applications/tree/master/sfu-ws
- https://github.com/pion/example-webrtc-applications/tree/master/gstreamer-receive
- https://github.com/pion/example-webrtc-applications/tree/master/gstreamer-send

## Instructions

Install dependencies:

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

### Run with TLS

```
go build
./webrtc-transform --cert cert-path --key key-path
# for instance
./webrtc-transform --cert certs/cert.pem --key certs/key.pem
```

Open [https://localhost:8080](https://localhost:8080) in several tabs.

### Run without TLS

```
./webrtc-transform
```

Open [http://localhost:8080](https//localhost:8080) in several tabs.

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
docker build -f Dockerfile.no-tls -t webrtc-transform:latest .
docker container run -p 8080:8080 -rm webrtc-transform:latest
```

### TODO

- [ ] assess performance/latency/jitter
- [ ] process video
- [ ] sync audio/video (RTC tracks + GStreamer)

### AV

mkdir -p lib
export PROJECT_BUILD=`pwd`/lib
export GST_PLUGIN_PATH="$GST_PLUGIN_PATH:$PROJECT_BUILD"

### Issues with Docker

`Dockerfile.multi-*` are intended to build multi-layered Docker images, separating building step _and_ dependencies from the final running environment. It currently does not work (INVESTIGATION NEEDED)

Hint for multi-debian: debug go execution, and check for relevant gstreamer runtime dependencies (try to add same apt dependencies in build and run stages, then clean up)

Hint for multi-alpine: apparent missing dependency to be found (https://superuser.com/questions/1176200/no-such-file-when-it-exists). Maybe easier to fix multi-debian first. See https://github.com/pion/ion/blob/master/docker/sfu.Dockerfile
