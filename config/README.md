Currently different recording modes are offered:

- `muxed` (default) -> audio/video muxed in the same file

- `split` -> audio video recorded in separate files (currently introduces delay on audio stream)

- `passthrough` -> records input streams and sends them back, without applying any fx or reencoding

- `none` -> no recording

A few notes about GStreamer settings:

- input media streams are stored with a `-dry.extension` suffix, when an effect is applied to a media stream, the recorded file of this processed stream ends with `-wet.extension`

- currently, setting `min-force-key-unit-interval` on encoders is disabled (more tests have to be done), it may be an interesting option to limit PLI requests

- disregard of dry paths, on wet paths: we always constrain caps before fx since some may rely on constant sizes

- Another solution is to prefer muxers robust to caps updates: `mpegtsmux` (for h264)

- queue params: `max-size-buffers=0 max-size-bytes=0` disable max-size on buffers and bytes. When teeing, the branch that does recording has an additionnal `max-size-time=5000000000` property. A queue blocks whenever one of the 3 dimensions (buffers, bytes, time) max is reached (unless `leaky`)

- rtpjitterbuffer proves to be necessary for h264, more tests needed (including on its latency value) for other formats (it indeed seems necessary when using the smile effect even with vp8)

- shoould we use codec without B-frames (since they rely on future keyframes)?

- `nvh264enc`: crash if bframes is not zero

- `rc-lookahead` is supposed to improve rate-control accuracy https://docs.nvidia.com/video-technologies/video-codec-sdk/pdf/Using_FFmpeg_with_NVIDIA_GPU_Hardware_Acceleration.pdf BUT image freezes, so has been disabled for nvcodec

- `h264timestamper` triggers an "Unknown frame rate, assume 25/1" that's why we disabled it since framerate should not be defined by this elemend

About cuda

Check how to use cudaupload, cudadownload and where to put caps here: https://gstreamer.freedesktop.org/documentation/nvcodec/cudascale.html?gi-language=c

About muxers

- when bandwidth fluctuates (or when stream starts or ends), video caps may be changed (for instance regarding colorimetry, chroma-site or resolution) and that causes `matroskamux` to crash (but `mp4mux` is doing fine with it in GStreamer 1.22). That's why we prefer using `mp4mux` with H264

- but for VP8, `matroskamux` is needed, that's why need to constrained caps (and rely on `videoconvert` and the like to ensure caps) but it implies to be done on a video/x-raw stream, meaning the input video stream has to be decoded/capped/reencoded. It works but is consuming more computing resources. It's the current solution (note that decoding/reencoding is only needed for video, not for audio), and costs more processing only when there is not FX.

- in short: prefer H264 to compute less

About logging

- we tried forwarding Gstreamer logs to the main app to have all logs at the same place, but in the end disabling the default logger with "gst_debug_remove_log_function(gst_debug_log_default)" hindered some crucial error logs, that's why we removed completely log forwarding

Latest tests:

- matroskamux + h264 + resolution change > KO
- matroskamux + vp8 + resolution change > OK (that's why we disabled fixed caps for vp8)
- mpegtsmux: h264 only and written files are KO
- webmmux: vp8 only and written files bad quality at start (lost initial kf ?)
- mp4mux: h264 only and crashes with warnings 'Sample with zero duration on pad" and "error: Buffer has no PTS" (GST_DEBUG=6 prevents the crash...)

Conclusion: if we could prevent mp4mux from crashing we could use matroskamux for vp8 and mp4mux for h264, and decoding/capping/reencoding would be disabled when no fx is added to the video.

Encoder settings:

Try nvcudah264enc, should be used in conjunction with cudaupload/cudadownload?

https://gstreamer.freedesktop.org/documentation/x264/index.html
https://gstreamer.freedesktop.org/documentation/nvcodec/nvh264enc.html
https://gstreamer.freedesktop.org/documentation/vpx/vp8enc.html
https://gstreamer.freedesktop.org/documentation/opus/opusenc.html

Old vp8 encoder settings:
vp8enc keyframe-max-dist=64 resize-allowed=true dropframe-threshold=25 max-quantizer=56 cpu-used=5 threads=4 deadline=1 qos=true

From Gstreamer 1.22 release notes (about h264timestamper):

    Muxers are often picky and need proper PTS/DTS timestamps set on the input buffers, but that can be a problem if the encoded input media stream comes from a source that doesn't provide proper signalling of DTS, such as is often the case for RTP, RTSP and WebRTC streams or Matroska container files. Theoretically parsers should be able to fix this up, but it would probably require fairly invasive changes in the parsers, so two new elements h264timestamper and h265timestamper bridge the gap in the meantime and can reconstruct missing PTS/DTS.

See https://gstreamer.freedesktop.org/documentation/codectimestamper/h264timestamper.html?gi-language=c#h264timestamper-page

From GStreamer 1.18 release notes (about nvh264sldec):

    "nvdec: add H264 + H265 stateless codec implementation nvh264sldec
    and nvh265sldec with fewer features but improved latency. You can
    set the environment variable GST_USE_NV_STATELESS_CODEC=h264 to use
    the stateless decoder variant as nvh264dec instead of the “normal”
    NVDEC decoder implementatio"

Old

- `nvh264dec` VS `avdec_h264`, to be investigated: when enabling TWCC in engine.go, `nvh264dec` crashes GStreamer (`failed to decode picture`). That's for the time being you use CPU H264 decoding even if GPU acceleration is requested (will be used only for encoding)

- some improvements on matroskamux (this this [issue](https://gitlab.freedesktop.org/gstreamer/gstreamer/-/merge_requests/1657)) should help dealing with h264 (avc3) capped-changing streams, but in our latest tests if it does not crashes and do write correctly files, those files are a bit broken (missing initial keyframe and no way to navigate in file). Indeed "avc3 is not officially supported, only use this format for smart encoding" is seen in https://gitlab.freedesktop.org/gstreamer/gst-plugins-good/-/blob/discontinued-for-monorepo/gst/matroska/matroska-mux.c