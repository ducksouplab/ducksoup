{{.RTPBin}}

appsrc name=audio_rtp_src is-live=true format=GST_FORMAT_TIME do-timestamp=true ! {{.Audio.Rtp.Caps}} ! rtpbin.recv_rtp_sink_0
appsrc name=video_rtp_src is-live=true format=GST_FORMAT_TIME do-timestamp=true ! {{.Video.Rtp.Caps}} ! rtpbin.recv_rtp_sink_1

appsrc name=audio_rtcp_src ! rtpbin.recv_rtcp_sink_0
appsrc name=video_rtcp_src ! rtpbin.recv_rtcp_sink_1

appsink name=audio_rtp_sink
appsink name=video_rtp_sink qos=true

{{.Video.Muxer}} name=dry_muxer faststart=true faststart-file={{.Folder}}/cache/{{.FilePrefix}}-dry.mp4mux.faststart !
filesink name=dry_filesink location={{.Folder}}/recordings/{{.FilePrefix}}-dry.{{.Video.Extension}}

rtpbin. !
  tee name=tee_audio ! 
    {{.Queue.Leaky}} ! 
    {{.Audio.Rtp.Depay}} !
    dry_muxer.

  tee_audio. ! 
    {{.Queue.Leaky}} ! 
    audio_rtp_sink.

rtpbin. !
  tee name=tee_video ! 
    {{.Queue.Base}} ! 
    {{.Video.Rtp.Depay}} ! 
    dry_muxer.

  tee_video. ! 
    {{.Queue.Base}} ! 
    video_rtp_sink.
