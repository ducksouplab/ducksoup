appsrc name=audio_rtp_src is-live=true format=GST_FORMAT_TIME do-timestamp=true
appsrc name=video_rtp_src is-live=true format=GST_FORMAT_TIME do-timestamp=true

appsrc name=audio_rtcp_src ! audio_buffer.sink_rtcp
appsrc name=video_rtcp_src ! video_buffer.sink_rtcp

appsink name=audio_rtp_sink
appsink name=video_rtp_sink qos=true

{{.Audio.Muxer}} name=dry_audio_muxer !
filesink name=dry_audio_filesink location={{.Folder}}/recordings/{{.FilePrefix}}-audio-dry.{{.Audio.Extension}} 

{{.Video.Muxer}} name=dry_video_muxer !
filesink name=dry_video_filesink location={{.Folder}}/recordings/{{.FilePrefix}}-video-dry.{{.Video.Extension}}

audio_rtp_src. !
{{.Audio.Rtp.Caps}} ! 
{{.Audio.Rtp.JitterBuffer}} ! 
tee name=tee_audio ! 
  {{.Queue.Leaky}} ! 
  {{.Audio.Rtp.Depay}} !
  dry_audio_muxer.

tee_audio. ! 
  {{.Queue.Leaky}} ! 
  audio_rtp_sink.

video_rtp_src. !
{{.Video.Rtp.Caps}} ! 
{{.Video.Rtp.JitterBuffer}} ! 
tee name=tee_video ! 
  {{.Queue.Base}} ! 
  {{.Video.Rtp.Depay}} ! 
  dry_video_muxer.

tee_video. ! 
  {{.Queue.Base}} ! 
  video_rtp_sink.
