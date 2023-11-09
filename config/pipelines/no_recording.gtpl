{{.RTPBin}}

appsrc name=audio_rtp_src is-live=true format=GST_FORMAT_TIME do-timestamp=true ! {{.Audio.Rtp.Caps}} ! rtpbin.recv_rtp_sink_0
appsrc name=video_rtp_src is-live=true format=GST_FORMAT_TIME do-timestamp=true ! {{.Video.Rtp.Caps}} ! rtpbin.recv_rtp_sink_1

appsrc name=audio_rtcp_src ! rtpbin.recv_rtcp_sink_0
appsrc name=video_rtcp_src ! rtpbin.recv_rtcp_sink_1

appsink name=audio_rtp_sink 
appsink name=video_rtp_sink qos=true

rtpbin. !
{{if .Audio.Fx}}
    {{.Audio.Rtp.Depay}} !
    {{.Audio.Decoder}} !
    audioconvert !
    audio/x-raw,channels=1 !
    {{.Audio.Fx}} ! 
    audioconvert !  
    {{.Audio.EncodeWithCache "audio_encoder_wet" .Folder .FilePrefix}} ! 
    {{.Queue.Leaky}} ! 
    {{.Audio.Rtp.Pay}} !
    audio_rtp_sink.
{{else}}
    {{.Audio.Rtp.Caps}} ! 
    {{.Queue.Leaky}} ! 
    audio_rtp_sink.
{{end}}

rtpbin. !
{{if .Video.Fx}}
    {{.Video.Rtp.Depay}} ! 
    {{.Video.Decoder}} !
    {{.Queue.Leaky}} ! 
    {{.Video.ConstraintFormat}} !
    videoconvert ! 
    {{.Video.Fx}} ! 
    {{if .Video.Overlay }}
        {{.Video.TimeOverlay }} ! 
    {{end}}
    {{.Video.ConstraintFormat}} !
    {{.Video.EncodeWithCache "video_encoder_wet" .Folder .FilePrefix}} ! 

    {{.Queue.Base}} ! 
    {{.Video.Rtp.Pay}} ! 
    video_rtp_sink.
{{else}}
    {{.Video.Rtp.Caps}}
    {{.Queue.Base}} ! 
    video_rtp_sink.
{{end}}
