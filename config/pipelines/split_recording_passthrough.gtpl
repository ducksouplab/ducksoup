appsrc name=audio_src is-live=true format=GST_FORMAT_TIME do-timestamp=true
appsrc name=video_src is-live=true format=GST_FORMAT_TIME do-timestamp=true min-latency=33333333
appsink name=audio_sink qos=true
appsink name=video_sink qos=true
opusparse name=dry_audio_recorder ! oggmux ! filesink location=data/{{.Namespace}}/{{.FilePrefix}}-audio-dry.ogg 
mpegtsmux name=dry_video_recorder ! filesink location=data/{{.Namespace}}/{{.FilePrefix}}-video-dry.mts

audio_src. !
{{.Audio.Rtp.Caps}} ! 
tee name=tee_audio ! 
queue max-size-buffers=0 max-size-bytes=0 max-size-time=5000000000 ! 
{{.Audio.Rtp.JitterBuffer}} ! 
{{.Audio.Rtp.Depay}} !
dry_audio_recorder.

tee_audio. ! 
queue max-size-buffers=0 max-size-bytes=0 ! 
audio_sink.

video_src. !
{{.Video.Rtp.Caps}} ! 
tee name=tee_video ! 
queue max-size-buffers=0 max-size-bytes=0 max-size-time=5000000000 ! 
{{.Video.Rtp.JitterBuffer}} ! 
{{.Video.Rtp.Depay}} ! 
dry_video_recorder.

tee_video. ! 
queue max-size-buffers=0 max-size-bytes=0 ! 
video_sink.
