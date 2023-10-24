appsrc name=audio_rtp_src is-live=true format=GST_FORMAT_TIME do-timestamp=true
appsrc name=video_rtp_src is-live=true format=GST_FORMAT_TIME do-timestamp=true

appsink name=audio_rtp_sink
appsink name=video_rtp_sink qos=true

{{/* always record dry */}}
{{.Audio.Muxer}} name=dry_audio_muxer !
filesink name=dry_audio_filesink location={{.Folder}}/recordings/{{.FilePrefix}}-audio-dry.{{.Audio.Extension}} 

{{.Video.Muxer}} name=dry_video_muxer !
filesink name=dry_video_filesink location={{.Folder}}/recordings/{{.FilePrefix}}-video-dry.{{.Video.Extension}}

{{if .Audio.Fx }}
    {{.Audio.Muxer}} name=wet_audio_muxer !
    filesink name=wet_audio_filesink location={{.Folder}}/recordings/{{.FilePrefix}}-audio-wet.{{.Audio.Extension}} 
{{end}}

{{if .Video.Fx }}
    {{.Video.Muxer}} name=wet_video_muxer !
    filesink name=wet_video_filesink location={{.Folder}}/recordings/{{.FilePrefix}}-video-wet.{{.Video.Extension}}
{{end}}

audio_rtp_src. !
{{.Audio.Rtp.Caps}} ! 
{{.Audio.Rtp.JitterBuffer}} ! 
{{if .Audio.Fx}}
    {{.Audio.Rtp.Depay}} !
    tee name=tee_audio_in ! 
        {{.Queue.Leaky}} ! 
        dry_audio_muxer.

    tee_audio_in. ! 
        {{.Queue.Leaky}} ! 
        {{.Audio.Decoder}} !
        audioconvert ! 
        audio/x-raw,channels=1 !
        {{.Audio.Fx}} ! 
        audioconvert ! 
        {{.Audio.EncodeWithCache "audio_encoder_dry" .Folder .FilePrefix}} !
        tee name=tee_audio_out ! 
            {{.Queue.Leaky}} ! 
            wet_audio_muxer.

        tee_audio_out. ! 
            {{.Queue.Leaky}} ! 
            {{.Audio.Rtp.Pay}} !
            audio_rtp_sink.
{{else}}
    tee name=tee_audio_in ! 
        {{.Queue.Leaky}} ! 
        {{.Audio.Rtp.Depay}} !
        dry_audio_muxer.
 
    tee_audio_in. ! 
        {{.Queue.Leaky}} ! 
        audio_rtp_sink.
{{end}}

video_rtp_src. !
{{.Video.Rtp.Caps}} ! 
{{.Video.Rtp.JitterBuffer}} ! 
{{if .Video.Fx}}
    {{.Video.Rtp.Depay}} ! 
    {{.Video.Decoder}} !
    {{.Video.ConstraintFormatFramerateResolution .Framerate .Width .Height}} !

    tee name=tee_video_in ! 
        {{.Queue.Leaky}} ! 
        {{.Video.EncodeWithCache "video_encoder_dry" .Folder .FilePrefix}} !
        dry_video_muxer.

    tee_video_in. ! 
        {{.Queue.Leaky}} ! 
        videoconvert ! 
        {{.Video.Fx}} ! 
        {{if .Video.Overlay }}
            {{.Video.TimeOverlay }} ! 
        {{end}}
        {{.Video.ConstraintFormat}} !
        {{.Video.EncodeWithCache "video_encoder_wet" .Folder .FilePrefix}} !

        tee name=tee_video_out ! 
            {{.Queue.Base}} ! 
            wet_video_muxer.

        tee_video_out. ! 
            {{.Queue.Base}} ! 
            {{.Video.Rtp.Pay}} ! 
            video_rtp_sink.
{{else}}
        tee name=tee_video_in ! 
        {{.Queue.Base}} ! 
        {{.Video.Rtp.Depay}} ! 
        {{.Video.Decoder}} !
        {{.Queue.Leaky}} ! 
        {{.Video.ConstraintFormatFramerateResolution .Framerate .Width .Height}} !
        {{if .Video.Overlay }}
            {{.Video.TimeOverlay }} ! 
        {{end}}
        {{.Video.EncodeWithCache "video_encoder_dry" .Folder .FilePrefix}} !
        dry_video_muxer.

    tee_video_in. ! 
        {{.Queue.Base}} ! 
        video_rtp_sink.
{{end}}
