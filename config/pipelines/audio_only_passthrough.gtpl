appsrc name=audio_rtp_src is-live=true format=GST_FORMAT_TIME

appsrc name=audio_rtcp_src ! audio_buffer.sink_rtcp

appsink name=audio_rtp_sink qos=true

{{.Audio.Muxer}} name=dry_audio_muxer !
filesink name=dry_audio_filesink location={{.Folder}}/recordings/{{.FilePrefix}}-audio-dry.{{.Audio.Extension}} 

audio_rtp_src. !
{{.Audio.Rtp.Caps}} ! 
{{.Audio.Rtp.JitterBuffer}} ! 
tee name=tee_audio ! 
  {{.Queue.Base}} ! 
  {{.Audio.Rtp.Depay}} !
  dry_audio_muxer.

tee_audio. ! 
  {{.Queue.Base}} ! 
  audio_rtp_sink.
