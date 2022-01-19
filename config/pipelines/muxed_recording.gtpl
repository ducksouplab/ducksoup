appsrc name=audio_src format=time is-live=true format=GST_FORMAT_TIME
appsrc name=video_src format=time is-live=true format=GST_FORMAT_TIME
appsink name=audio_sink qos=true
appsink name=video_sink qos=true
{{/* always record dry */}}
matroskamux name=dry_recorder ! filesink location=data/{{.Namespace}}/{{.FilePrefix}}-dry.mkv
{{/* record fx if one on audio or video */}}
{{if or .Video.Fx .Audio.Fx }}
    matroskamux name=wet_recorder ! filesink location=data/{{.Namespace}}/{{.FilePrefix}}-wet.mkv
{{end}}

audio_src. !
{{.Audio.Rtp.Caps}} ! 
{{if .Audio.Fx}}
    {{.Audio.Rtp.JitterBuffer}} ! 
    {{.Audio.Rtp.Depay}} !
    tee name=tee_audio_in ! 
    queue max-size-buffers=0 max-size-bytes=0 max-size-time=5000000000 ! 
    dry_recorder.

    tee_audio_in. ! 
    queue max-size-buffers=0 max-size-bytes=0 ! 
    {{.Audio.Decode}} !
    {{.Audio.RawCaps}} !
    audioconvert ! 
    {{.Audio.Fx}} ! 
    audioconvert ! 
    {{.Audio.EncodeWith "audio_encoder_wet" .Namespace .FilePrefix}} ! 
    tee name=tee_audio_out ! 
    queue max-size-buffers=0 max-size-bytes=0 max-size-time=5000000000 ! 
    wet_recorder.

    tee_audio_out. ! 
    queue max-size-buffers=0 max-size-bytes=0 ! 
    {{.Audio.Rtp.Pay}} !
    audio_sink.
{{else}}
    tee name=tee_audio_in ! 
    queue max-size-buffers=0 max-size-bytes=0 max-size-time=5000000000 ! 
    {{.Audio.Rtp.JitterBuffer}} ! 
    {{.Audio.Rtp.Depay}} !
    {{/* audio stream has to be written to two files if there is a video fx*/}}
    {{if .Video.Fx }}
        tee name=tee_audio_out !
        queue max-size-buffers=0 max-size-bytes=0 ! 
        dry_recorder.

        tee_audio_out. !
        queue max-size-buffers=0 max-size-bytes=0 !
        wet_recorder.
    {{else}}
        dry_recorder.
    {{end}}

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
    {{.Video.RawCapsLight}} !

    tee name=tee_video_in ! 
    queue max-size-buffers=0 max-size-bytes=0 max-size-time=5000000000 ! 
    {{.Video.EncodeWith "video_encoder_dry" .Namespace .FilePrefix}} ! 
    dry_recorder.

    tee_video_in. ! 
    queue max-size-buffers=0 max-size-bytes=0 ! 
    videoconvert ! 
    {{.Video.Fx}} ! 
    videoconvert ! 
    {{.Video.RawCapsLight}} !
    {{.Video.EncodeWith "video_encoder_wet" .Namespace .FilePrefix}} ! 

    tee name=tee_video_out ! 
    queue max-size-buffers=0 max-size-bytes=0 max-size-time=5000000000 ! 
    wet_recorder.

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
    {{.Video.RawCapsLight}} !
    {{.Video.EncodeWith "video_encoder_dry" .Namespace .FilePrefix}} ! 
    {{/* video stream has to be written to two files if there is an aufio fx*/}}
    {{if .Audio.Fx }}
        tee name=tee_video_out !
        queue max-size-buffers=0 max-size-bytes=0 ! 
        dry_recorder.

        tee_video_out. !
        queue max-size-buffers=0 max-size-bytes=0 !
        wet_recorder.
    {{else}}
        dry_recorder.
    {{end}}

    tee_video_in. ! 
    queue max-size-buffers=0 max-size-bytes=0 ! 
    video_sink.
{{end}}
