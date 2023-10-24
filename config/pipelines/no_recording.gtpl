appsrc name=audio_rtp_src is-live=true format=GST_FORMAT_TIME do-timestamp=true
appsrc name=video_rtp_src is-live=true format=GST_FORMAT_TIME do-timestamp=true

appsink name=audio_rtp_sink
appsink name=video_rtp_sink qos=true

audio_rtp_src. !
{{.Audio.Rtp.Caps}} ! 
{{if .Audio.Fx}}
    {{.Audio.Rtp.JitterBuffer}} ! 
    {{.Audio.Rtp.Depay}} !
    {{.Audio.Decoder}} !
    audioconvert !
    audio/x-raw,channels=1 !
    {{.Audio.Fx}} ! 
    audioconvert !  
    {{.Audio.EncodeWithCache "audio_encoder_wet" .Folder .FilePrefix}} ! 
    {{.Audio.Rtp.Pay}} !
    audio_rtp_sink.
{{else}}
    queue ! 
    audio_rtp_sink.
{{end}}

video_rtp_src. !
{{.Video.Rtp.Caps}} ! 
{{if .Video.Fx}}
    {{.Video.Rtp.JitterBuffer}} ! 
    {{.Video.Rtp.Depay}} ! 
    {{.Video.Decoder}} !
    {{.Queue.Leaky}} ! 
    {{.Video.ConstraintFormatFramerateResolution .Framerate .Width .Height}} !
    videoconvert ! 
    {{.Video.Fx}} ! 
    {{if .Video.Overlay }}
        {{.Video.TimeOverlay }} ! 
    {{end}}
    {{.Video.ConstraintFormat}} !
    {{.Video.EncodeWithCache "video_encoder_wet" .Folder .FilePrefix}} ! 
    queue ! 
    {{.Video.Rtp.Pay}} ! 
    video_rtp_sink.
{{else}}
    queue ! 
    video_rtp_sink.
{{end}}
