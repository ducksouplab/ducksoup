appsrc name=audio_src format=time is-live=true format=GST_FORMAT_TIME
appsrc name=video_src format=time is-live=true format=GST_FORMAT_TIME
appsink name=audio_sink qos=true
appsink name=video_sink qos=true
{{/* always record dry */}}
opusparse name=dry_audio_recorder ! oggmux ! filesink location=data/{{.Namespace}}/{{.FilePrefix}}-audio-dry.ogg 
matroskamux name=dry_video_recorder ! filesink location=data/{{.Namespace}}/{{.FilePrefix}}-video-dry.mkv
{{if .Audio.Fx }}
    opusparse name=wet_audio_recorder ! oggmux ! filesink location=data/{{.Namespace}}/{{.FilePrefix}}-audio-wet.ogg 
{{end}}
{{if .Video.Fx }}
    matroskamux name=wet_video_recorder ! filesink location=data/{{.Namespace}}/{{.FilePrefix}}-video-wet.mkv
{{end}}

audio_src. !
{{.Audio.Rtp.Caps}} ! 
{{if .Audio.Fx}}
    {{.Audio.Rtp.JitterBuffer}} ! 
    {{.Audio.Rtp.Depay}} !
    tee name=tee_audio_in ! 
    queue max-size-buffers=0 max-size-bytes=0 max-size-time=5000000000 ! 
    dry_audio_recorder.

    tee_audio_in. ! 
    queue max-size-buffers=0 max-size-bytes=0 ! 
    {{.Audio.Decode}} !
    audioconvert ! 
    audio/x-raw,channels=1 !
    {{.Audio.Fx}} ! 
    audioconvert ! 
    {{.Audio.EncodeWith "audio_encoder_dry" .Namespace .FilePrefix}} !
    tee name=tee_audio_out ! 
    queue max-size-buffers=0 max-size-bytes=0 max-size-time=5000000000 ! 
    wet_audio_recorder.

    tee_audio_out. ! 
    queue max-size-buffers=0 max-size-bytes=0 ! 
    {{.Audio.Rtp.Pay}} !
    audio_sink.
{{else}}
    tee name=tee_audio_in ! 
    queue max-size-buffers=0 max-size-bytes=0 max-size-time=5000000000 ! 
    {{.Audio.Rtp.JitterBuffer}} ! 
    {{.Audio.Rtp.Depay}} !
    dry_audio_recorder.
 
    tee_audio_in. ! 
    queue max-size-buffers=0 max-size-bytes=0 ! 
    audio_sink.
{{end}}

video_src. !
{{.Video.Rtp.Caps}} ! 
{{if .Video.Fx}}
    {{.Video.Rtp.JitterBuffer}} ! 
    {{.Video.Rtp.Depay}} ! 
    {{.Video.Decode}} !
    {{.Video.CapFormatRateScale .Width .Height .FrameRate}} !

    tee name=tee_video_in ! 
    queue max-size-buffers=0 max-size-bytes=0 max-size-time=5000000000 ! 
    {{.Video.EncodeWith "video_encoder_dry" .Namespace .FilePrefix}} !
    dry_video_recorder.

    tee_video_in. ! 
    queue max-size-buffers=0 max-size-bytes=0 ! 
    videoconvert ! 
    {{.Video.Fx}} ! 
    {{.Video.CapFormatOnly}} !
    {{.Video.EncodeWith "video_encoder_wet" .Namespace .FilePrefix}} !
    tee name=tee_video_out ! 
    queue max-size-buffers=0 max-size-bytes=0 max-size-time=5000000000 ! 
    wet_video_recorder.

    tee_video_out. ! 
    queue max-size-buffers=0 max-size-bytes=0 ! 
    {{.Video.Rtp.Pay}} ! 
    video_sink.
{{else}}
    tee name=tee_video_in ! 
    queue max-size-buffers=0 max-size-bytes=0 max-size-time=5000000000 ! 
    {{.Video.Rtp.JitterBuffer}} ! 
    {{.Video.Rtp.Depay}} ! 
    {{.Video.Decode}} !

    {{.Video.Decode}} !
    {{.Video.CapFormatRateScale .Width .Height .FrameRate}} !
    {{.Video.EncodeWith "video_encoder_dry" .Namespace .FilePrefix}} ! 

    {{.Video.EncodeWith "video_encoder_dry" .Namespace .FilePrefix}} !
    dry_video_recorder.

    tee_video_in. ! 
    queue max-size-buffers=0 max-size-bytes=0 ! 
    video_sink.
{{end}}
