{{.RTPBin}}

appsrc name=audio_rtp_src is-live=true format=GST_FORMAT_TIME do-timestamp=true ! {{.Audio.Rtp.Caps}} ! rtpbin.recv_rtp_sink_0

appsrc name=audio_rtcp_src ! rtpbin.recv_rtcp_sink_0

appsink name=audio_rtp_sink

rtpbin. !
{{if .Audio.Fx}}
    {{.Audio.Rtp.Depay}} !
    {{.Audio.Decoder}} !
    audioconvert !
    audio/x-raw,channels=1 !
    {{.Audio.Fx}} ! 
    audioconvert !  
    {{.Audio.EncodeWith "audio_encoder_wet"}} ! 
    {{.Audio.Rtp.Pay}} !
    {{.FinalQueue}} name=video_queue_bef_sink ! 
    audio_rtp_sink.
{{else}}
    {{.FinalQueue}} name=video_queue_bef_sink ! 
    audio_rtp_sink.
{{end}}