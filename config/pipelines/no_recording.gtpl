appsrc name=audio_src is-live=true format=GST_FORMAT_TIME
appsrc name=video_src is-live=true format=GST_FORMAT_TIME min-latency=33333333
appsink name=audio_sink qos=true
appsink name=video_sink qos=true

audio_src. !
{{.Audio.Rtp.Caps}} ! 
{{if .Audio.Fx}}
    {{.Audio.Rtp.JitterBuffer}} ! 
    {{.Audio.Rtp.Depay}} !
    {{.Audio.Decoder}} !
    audioconvert !
    audio/x-raw,channels=1 !
    {{.Audio.Fx}} ! 
    audioconvert !  
    {{.Audio.EncodeWith "audio_encoder_wet" .Namespace .FilePrefix}} ! 
    {{.Audio.Rtp.Pay}} !
    audio_sink.
{{else}}
    queue ! 
    audio_sink.
{{end}}

video_src. !
{{.Video.Rtp.Caps}} ! 
{{if .Video.Fx}}
    {{.Video.Rtp.JitterBuffer}} ! 
    {{.Video.Rtp.Depay}} ! 
    {{.Video.Decoder}} !
    {{.Video.ConstraintFormatFramerateResolution .Framerate .Width .Height}} !
    videoconvert ! 
    {{.Video.Fx}} ! 
    {{if .Video.Overlay }}
        timeoverlay ! 
    {{end}}
    {{.Video.ConstraintFormat}} !
    {{.Video.EncodeWith "video_encoder_wet" .Namespace .FilePrefix}} ! 
    queue ! 
    {{.Video.Rtp.Pay}} ! 
    video_sink.
{{else}}
    queue ! 
    video_sink.
{{end}}
