A few notes:

- queue params: `max-size-buffers=0 max-size-bytes=0` disable max-size on buffers and bytes. When teeing, the branch that does recording has an additionnal `max-size-time=5000000000` property. A queue blocks whenever one of the 3 dimensions (buffers, bytes, time) max is reached (unless `leaky`)

- rtpjitterbuffer proves to be necessary for h264, more tests needed (including on its latency value) for other formats (it indeed seems necessary when using the smile effect even with vp8)

- when bandwidth fluctuates, the video caps may be updated (for instance regarding colorimetry or chroma-site) which does not play well with `matroskamux`, which is why video caps are constrained (and `videoconvert` is used) in the pipelines.

- in the pipelines, the `${variable}` notation means a variable that is interpolated by `pipeline.go`

- when an encoder is named (`name=encoder`) it means it will be controlled (by `pipeline.go`) regarding its target bitrate

Encoder settings:

https://gstreamer.freedesktop.org/documentation/x264/index.html?gi-language=c

Old vp8 encoder settings:
vp8enc keyframe-max-dist=64 resize-allowed=true dropframe-threshold=25 max-quantizer=56 cpu-used=5 threads=4 deadline=1 qos=true