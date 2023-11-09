appsrc name=audio_rtp_src is-live=true format=GST_FORMAT_TIME do-timestamp=true
appsrc name=video_rtp_src is-live=true format=GST_FORMAT_TIME do-timestamp=true

appsrc name=audio_rtcp_src ! audio_buffer.sink_rtcp
appsrc name=video_rtcp_src ! video_buffer.sink_rtcp

appsink name=audio_rtp_sink
appsink name=video_rtp_sink qos=true

{{/* always record dry */}}
{{.Video.Muxer}} name=dry_muxer faststart=true faststart-file={{.Folder}}/cache/{{.FilePrefix}}-dry.mp4mux.faststart !
filesink name=dry_filesink location={{.Folder}}/recordings/{{.FilePrefix}}-dry.{{.Video.Extension}}

{{/* record fx if one on audio or video */}}
{{if or .Video.Fx .Audio.Fx }}
    {{.Video.Muxer}} name=wet_muxer faststart=true faststart-file={{.Folder}}/cache/{{.FilePrefix}}-wet.mp4mux.faststart !
    filesink name=wet_filesink location={{.Folder}}/recordings/{{.FilePrefix}}-wet.{{.Video.Extension}}
{{end}}

audio_rtp_src. !
{{.Audio.Rtp.Caps}} ! 
{{.Audio.Rtp.JitterBuffer}} ! 
{{if .Audio.Fx}}
    {{.Audio.Rtp.Depay}} !

    tee name=tee_audio_in ! 
        {{.Queue.Leaky}} !
        dry_muxer.

    tee_audio_in. ! 
        {{.Queue.Leaky}} ! 
        {{.Audio.Decoder}} !
        audioconvert !
        audio/x-raw,channels=1 !
        {{.Audio.Fx}} ! 
        audioconvert ! 
        {{.Audio.EncodeWith "audio_encoder_wet" }} ! 

        tee name=tee_audio_out ! 
            {{.Queue.Leaky}} ! 
            wet_muxer.

        tee_audio_out. ! 
            {{.Queue.Leaky}} ! 
            {{.Audio.Rtp.Pay}} !
            audio_rtp_sink.
{{else}}
    tee name=tee_audio_in ! 
        {{.Queue.Leaky}} ! 
        {{.Audio.Rtp.Depay}} !
        {{/* audio stream has to be written to two files if there is a video fx*/}}
        {{if .Video.Fx }}
            tee name=tee_audio_out !
                {{.Queue.Leaky}} ! 
                dry_muxer.

            tee_audio_out. !
                {{.Queue.Leaky}} ! 
                wet_muxer.
        {{else}}
            dry_muxer.
        {{end}}

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
    {{.Video.ConstraintFormatFramerate .Framerate }} !

    tee name=tee_video_in ! 
        {{.Queue.Leaky}} !  
        {{.Video.EncodeWithCache "video_encoder_dry" .Folder .FilePrefix}} ! 
        dry_muxer.

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
            wet_muxer.

        tee_video_out. ! 
            {{.Queue.Base}} ! 
            {{.Video.Rtp.Pay}} ! 
            video_rtp_sink.
{{else}}
    tee name=tee_video_in ! 
        {{.Queue.Base}} ! 
        {{.Video.Rtp.Depay}} ! 

        {{if not .Video.SkipFixedCaps}}
            {{.Video.Decoder}} !
            {{.Queue.Leaky}} ! 
            {{.Video.ConstraintFormatFramerate .Framerate}} !
            {{if .Video.Overlay }}
                {{.Video.TimeOverlay }} ! 
            {{end}}
            {{.Video.EncodeWithCache "video_encoder_dry" .Folder .FilePrefix}} ! 
        {{end}}
        
        {{/* video stream has to be written to two files if there is an aufio fx*/}}
        {{if .Audio.Fx }}
            tee name=tee_video_out !
                {{.Queue.Base}} ! 
                dry_muxer.

            tee_video_out. !
                {{.Queue.Base}} ! 
                wet_muxer.
        {{else}}
            dry_muxer.
        {{end}}

    tee_video_in. ! 
        {{.Queue.Base}} ! 
        video_rtp_sink.
{{end}}
