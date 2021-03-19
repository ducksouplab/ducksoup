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

### Run without TLS

```
./webrtc-transform
```

And then open [https://localhost:8080](https://localhost:8080) in several tabs.
