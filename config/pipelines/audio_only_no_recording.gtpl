appsrc name=audio_rtp_src is-live=true format=GST_FORMAT_TIME do-timestamp=true

appsrc name=audio_rtcp_src ! audio_buffer.sink_rtcp

appsink name=audio_rtp_sink

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