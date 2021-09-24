A few notes:

- input media streams are stored with an `-in.extension` suffix (for instance in the form `datetime-identifier-audio-raw.ogg`). When an effect is applied to a media stream, the recorded file of this processed stream ends with `-fx.extension`

- when bandwidth fluctuates (or when stream starts or ends), video caps may be changed (for instance regarding colorimetry or chroma-site) which does not play well with `matroskamux` (nor `webmmux`, `mp4mux`). One solution (previously used) is to constrained caps (and rely on `videoconvert` and the like to ensure caps) but it implies to be done on a video/x-raw stream, meaning the input video stream has to be decoded/capped/reencoded for it to work. It works but is consuming more CPU resources. The current solution is to prefer muxers robust to caps updates: `mpegtsmux` (for h264) and `webmmux` (for vp8). `oggmux` is still used for audio streams.

- queue params: `max-size-buffers=0 max-size-bytes=0` disable max-size on buffers and bytes. When teeing, the branch that does recording has an additionnal `max-size-time=5000000000` property. A queue blocks whenever one of the 3 dimensions (buffers, bytes, time) max is reached (unless `leaky`)

- rtpjitterbuffer proves to be necessary for h264, more tests needed (including on its latency value) for other formats (it indeed seems necessary when using the smile effect even with vp8)


- in the pipelines, the `${variable}` notation means a variable that is interpolated by `pipeline.go`

- when an encoder is named (`name=encoder`) it means it will be controlled (by `pipeline.go`) regarding its target bitrate

Encoder settings:

https://gstreamer.freedesktop.org/documentation/x264/index.html
https://gstreamer.freedesktop.org/documentation/nvcodec/nvh264enc.html
https://gstreamer.freedesktop.org/documentation/vpx/vp8enc.html
https://gstreamer.freedesktop.org/documentation/opus/opusenc.html

Old vp8 encoder settings:
vp8enc keyframe-max-dist=64 resize-allowed=true dropframe-threshold=25 max-quantizer=56 cpu-used=5 threads=4 deadline=1 qos=true