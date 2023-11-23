{{.RTPBin}}

appsrc name=audio_rtp_src is-live=true format=GST_FORMAT_TIME do-timestamp=true ! {{.Audio.Rtp.Caps}} ! rtpbin.recv_rtp_sink_0
appsrc name=video_rtp_src is-live=true format=GST_FORMAT_TIME do-timestamp=true ! {{.Video.Rtp.Caps}} ! rtpbin.recv_rtp_sink_1

appsrc name=audio_rtcp_src ! rtpbin.recv_rtcp_sink_0
appsrc name=video_rtcp_src ! rtpbin.recv_rtcp_sink_1

appsink name=audio_rtp_sink
appsink name=video_rtp_sink qos=true

rtpbin. !
  {{.FinalQueue}} leaky=2 ! 
  audio_rtp_sink.

rtpbin. !
  {{.FinalQueue}} ! 
  video_rtp_sink.