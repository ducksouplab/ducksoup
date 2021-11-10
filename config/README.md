A few notes about GStreamer

- input media streams are stored with a `-raw.extension` suffix, when an effect is applied to a media stream, the recorded file of this processed stream ends with `-fx.extension`

- when bandwidth fluctuates (or when stream starts or ends), video caps may be changed (for instance regarding colorimetry or chroma-site) which does not play well with `matroskamux` (nor `webmmux`, `mp4mux`). One solution is to constrained caps (and rely on `videoconvert` and the like to ensure caps) but it implies to be done on a video/x-raw stream, meaning the input video stream has to be decoded/capped/reencoded for it to work. It works but is consuming more CPU resources. It's the currently chosen solution (note that decoding/reencoding is only needed for video, not for audio)

- Another solution is to prefer muxers robust to caps updates: `mpegtsmux` (for h264) and `webmmux` (for vp8).

- queue params: `max-size-buffers=0 max-size-bytes=0` disable max-size on buffers and bytes. When teeing, the branch that does recording has an additionnal `max-size-time=5000000000` property. A queue blocks whenever one of the 3 dimensions (buffers, bytes, time) max is reached (unless `leaky`)

- rtpjitterbuffer proves to be necessary for h264, more tests needed (including on its latency value) for other formats (it indeed seems necessary when using the smile effect even with vp8)

- use codec without B-frames (since they rely on future keyframes)

- `nvh264dec` VS `avdec_h264`, to be investigated: when enabling TWCC in engine.go, `nvh264dec` crashes GStreamer (`failed to decode picture`). That's for the time being you use CPU H264 decoding even if GPU acceleration is requested (will be used only for encoding)

To try later:

- do-timestamp=true on appsrc

Encoder settings:

https://gstreamer.freedesktop.org/documentation/x264/index.html
https://gstreamer.freedesktop.org/documentation/nvcodec/nvh264enc.html
https://gstreamer.freedesktop.org/documentation/vpx/vp8enc.html
https://gstreamer.freedesktop.org/documentation/opus/opusenc.html

Old vp8 encoder settings:
vp8enc keyframe-max-dist=64 resize-allowed=true dropframe-threshold=25 max-quantizer=56 cpu-used=5 threads=4 deadline=1 qos=true