appsrc name=audio_src format=time is-live=true format=GST_FORMAT_TIME
appsrc name=video_src format=time is-live=true format=GST_FORMAT_TIME
appsink name=audio_sink qos=true
appsink name=video_sink qos=true

audio_src. !
{{.Audio.Rtp.Caps}} ! 
{{if .Audio.Fx}}
    {{.Audio.Rtp.JitterBuffer}} ! 
    {{.Audio.Rtp.Depay}} !
    {{.Audio.Decode}} !
    {{.Audio.RawCaps}} !
    audioconvert ! 
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
    {{.Video.Decode}} !
    {{.Video.RawCapsWith .Width .Height .FrameRate}} !
    videoconvert ! 
    {{.Video.Fx}} ! 
    videoconvert ! 
    {{.Video.RawCapsLight}} !
    {{.Video.EncodeWith "video_encoder_wet" .Namespace .FilePrefix}} ! 
    {{.Video.Rtp.Pay}} ! 
    video_sink.
{{else}}
    queue ! 
    video_sink.
{{end}}
