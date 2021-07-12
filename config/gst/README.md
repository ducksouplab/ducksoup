A few notes:

- queue params: `max-size-buffers=0 max-size-bytes=0` disable max-size on buffers and bytes. When teeing, the branch that does recording has an additionnal `max-size-time=5000000000` property. A queue blocks whenever one of the 3 dimensions (buffers, bytes, time) max is reached (unless `leaky`)

- rtpjitterbuffer proves to be necessary for h264, more tests needed (including on its latency value) for other formats (it indeed seems necessary when using the smile effect even with vp8)

- when bandwidth fluctuates, the video caps may be updated (for instance regarding colorimetry or chroma-site) which does not play well with `matroskamux`, which is why video caps are constrained (and `videoconvert` is used) in the pipelines.