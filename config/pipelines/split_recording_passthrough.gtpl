appsrc name=audio_src is-live=true format=GST_FORMAT_TIME do-timestamp=true
appsrc name=video_src is-live=true format=GST_FORMAT_TIME do-timestamp=true min-latency=33333333
appsink name=audio_sink qos=true
appsink name=video_sink qos=true
opusparse name=dry_audio_recorder ! oggmux ! filesink location=data/{{.Namespace}}/{{.FilePrefix}}-audio-dry.ogg 
mpegtsmux name=dry_video_recorder ! filesink location=data/{{.Namespace}}/{{.FilePrefix}}-video-dry.mts

audio_src. !
{{.Audio.Rtp.Caps}} ! 
tee name=tee_audio ! 
queue ! 
{{.Audio.Rtp.JitterBuffer}} ! 
{{.Audio.Rtp.Depay}} !
dry_audio_recorder.

tee_audio. ! 
queue ! 
audio_sink.

video_src. !
{{.Video.Rtp.Caps}} ! 
tee name=tee_video ! 
queue ! 
{{.Video.Rtp.JitterBuffer}} ! 
{{.Video.Rtp.Depay}} ! 
dry_video_recorder.

tee_video. ! 
queue ! 
video_sink.
