appsrc name=audio_src is-live=true format=GST_FORMAT_TIME do-timestamp=true
appsrc name=video_src is-live=true format=GST_FORMAT_TIME do-timestamp=true min-latency=33333333
appsink name=audio_sink qos=true
appsink name=video_sink qos=true
{{/* always record dry */}}
opusparse name=dry_audio_recorder ! oggmux ! filesink location=data/{{.Namespace}}/{{.FilePrefix}}-audio-dry.ogg 
{{.Video.Muxer}} name=dry_video_recorder ! filesink location=data/{{.Namespace}}/{{.FilePrefix}}-video-dry.{{.Video.Extension}}
{{if .Audio.Fx }}
    opusparse name=wet_audio_recorder ! oggmux ! filesink location=data/{{.Namespace}}/{{.FilePrefix}}-audio-wet.ogg 
{{end}}
{{if .Video.Fx }}
    {{.Video.Muxer}} name=wet_video_recorder ! filesink location=data/{{.Namespace}}/{{.FilePrefix}}-video-wet.{{.Video.Extension}}
{{end}}

audio_src. !
{{.Audio.Rtp.Caps}} ! 
{{if .Audio.Fx}}
    {{.Audio.Rtp.JitterBuffer}} ! 
    {{.Audio.Rtp.Depay}} !
    tee name=tee_audio_in ! 
        queue ! 
        dry_audio_recorder.

    tee_audio_in. ! 
        queue ! 
        {{.Audio.Decoder}} !
        audioconvert ! 
        audio/x-raw,channels=1 !
        {{.Audio.Fx}} ! 
        audioconvert ! 
        {{.Audio.EncodeWith "audio_encoder_dry" .Namespace .FilePrefix}} !
        tee name=tee_audio_out ! 
            queue ! 
            wet_audio_recorder.

        tee_audio_out. ! 
            queue ! 
            {{.Audio.Rtp.Pay}} !
            audio_sink.
{{else}}
    tee name=tee_audio_in ! 
        queue ! 
        {{.Audio.Rtp.JitterBuffer}} ! 
        {{.Audio.Rtp.Depay}} !
        dry_audio_recorder.
 
    tee_audio_in. ! 
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

    tee name=tee_video_in ! 
        queue ! 
        {{.Video.EncodeWith "video_encoder_dry" .Namespace .FilePrefix}} !
        dry_video_recorder.

    tee_video_in. ! 
        queue ! 
        videoconvert ! 
        {{.Video.Fx}} ! 
        {{if .Video.Overlay }}
            timeoverlay ! 
        {{end}}
        {{.Video.ConstraintFormat}} !
        {{.Video.EncodeWith "video_encoder_wet" .Namespace .FilePrefix}} !

        tee name=tee_video_out ! 
            queue ! 
            wet_video_recorder.

        tee_video_out. ! 
            queue ! 
            {{.Video.Rtp.Pay}} ! 
            video_sink.
{{else}}
        tee name=tee_video_in ! 
        queue ! 
        {{.Video.Rtp.JitterBuffer}} ! 
        {{.Video.Rtp.Depay}} ! 
        {{.Video.Decoder}} !
        {{.Video.ConstraintFormatFramerateResolution .Framerate .Width .Height}} !
        {{if .Video.Overlay }}
            timeoverlay ! 
        {{end}}
        {{.Video.EncodeWith "video_encoder_dry" .Namespace .FilePrefix}} !
        dry_video_recorder.

    tee_video_in. ! 
        queue ! 
        video_sink.
{{end}}
