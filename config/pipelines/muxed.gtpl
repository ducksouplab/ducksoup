appsrc name=audio_src is-live=true format=GST_FORMAT_TIME
appsrc name=video_src is-live=true format=GST_FORMAT_TIME min-latency=33333333

appsrc name=audio_rtcp_src ! audio_buffer.sink_rtcp
appsrc name=video_rtcp_src ! video_buffer.sink_rtcp

appsink name=audio_sink qos=true
appsink name=video_sink qos=true

{{/* always record dry */}}
{{.Video.Muxer}} name=dry_recorder presentation-time=true faststart=true faststart-file={{.Folder}}/cache/{{.FilePrefix}}-dry.mp4mux.faststart ! {{.Queue.Base}} ! filesink location={{.Folder}}/recordings/{{.FilePrefix}}-dry.{{.Video.Extension}}
{{/* record fx if one on audio or video */}}
{{if or .Video.Fx .Audio.Fx }}
    {{.Video.Muxer}} name=wet_recorder presentation-time=true faststart=true faststart-file={{.Folder}}/cache/{{.FilePrefix}}-wet.mp4mux.faststart ! {{.Queue.Base}} ! filesink location={{.Folder}}/recordings/{{.FilePrefix}}-wet.{{.Video.Extension}}
{{end}}

audio_src. !
{{.Audio.Rtp.Caps}} ! 
{{.Audio.Rtp.JitterBuffer}} ! 
{{if .Audio.Fx}}
    {{.Audio.Rtp.Depay}} !

    tee name=tee_audio_in ! 
        {{.Queue.Base}} ! 
        dry_recorder.

    tee_audio_in. ! 
        {{.Queue.Base}} ! 
        {{.Audio.Decoder}} !
        audioconvert !
        audio/x-raw,channels=1 !
        {{.Audio.Fx}} ! 
        audioconvert ! 
        {{.Audio.EncodeWith "audio_encoder_wet"}} ! 

        tee name=tee_audio_out ! 
            {{.Queue.Base}} ! 
            wet_recorder.

        tee_audio_out. ! 
            {{.Queue.Base}} ! 
            {{.Audio.Rtp.Pay}} !
            audio_sink.
{{else}}
    tee name=tee_audio_in ! 
        {{.Queue.Base}} ! 
        {{.Audio.Rtp.Depay}} !
        {{/* audio stream has to be written to two files if there is a video fx*/}}
        {{if .Video.Fx }}
            tee name=tee_audio_out !
                {{.Queue.Base}} ! 
                dry_recorder.

            tee_audio_out. !
                {{.Queue.Base}} ! 
                wet_recorder.
        {{else}}
            dry_recorder.
        {{end}}

    tee_audio_in. ! 
        {{.Queue.Base}} ! 
        audio_sink.
{{end}}

video_src. !
{{.Video.Rtp.Caps}} ! 
{{.Video.Rtp.JitterBuffer}} ! 
{{if .Video.Fx}}
    {{.Video.Rtp.Depay}} ! 

    tee name=tee_video_in ! 
        {{.Queue.Leaky}} ! 
        dry_recorder.

    tee_video_in. ! 
        {{.Queue.Base}} ! 
        {{.Video.Decoder}} !
        {{.Video.ConstraintFormatFramerate .Framerate}} !

        videoconvert ! 
        {{.Video.Fx}} !
        {{if .Video.Overlay }}
            timeoverlay time-mode=1 ! 
        {{end}}

        {{.Queue.Base}} ! 
        {{.Video.ConstraintFormat}} !
        {{.Video.EncodeWithCache "video_encoder_wet" .Folder .FilePrefix}} ! 

        tee name=tee_video_out ! 
            {{.Queue.Base}} ! 
            wet_recorder.

        tee_video_out. ! 
            {{.Queue.Base}} ! 
            {{.Video.Rtp.Pay}} ! 
            video_sink.
{{else}}
    tee name=tee_video_in ! 
        {{.Queue.Base}} ! 
        {{.Video.Rtp.Depay}} ! 
        
        {{/* video stream has to be written to two files if there is an aufio fx*/}}
        {{if .Audio.Fx }}
            tee name=tee_video_out !
                {{.Queue.Base}} ! 
                dry_recorder.

            tee_video_out. !
                {{.Queue.Base}} ! 
                wet_recorder.
        {{else}}
            dry_recorder.
        {{end}}

    tee_video_in. ! 
        {{.Queue.Base}} ! 
        video_sink.
{{end}}
