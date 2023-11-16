{{.RTPBin}}

appsrc name=audio_rtp_src is-live=true format=GST_FORMAT_TIME do-timestamp=true ! {{.Audio.Rtp.Caps}} ! rtpbin.recv_rtp_sink_0
appsrc name=video_rtp_src is-live=true format=GST_FORMAT_TIME do-timestamp=true ! {{.Video.Rtp.Caps}} ! rtpbin.recv_rtp_sink_1

appsrc name=audio_rtcp_src ! rtpbin.recv_rtcp_sink_0
appsrc name=video_rtcp_src ! rtpbin.recv_rtcp_sink_1

appsink name=audio_rtp_sink
appsink name=video_rtp_sink qos=true

{{.Video.Muxer}} name=dry_muxer faststart=true faststart-file={{.Folder}}/cache/{{.FilePrefix}}-dry.mp4mux.faststart !
filesink name=dry_filesink location={{.Folder}}/recordings/{{.FilePrefix}}-dry.{{.Video.Extension}}

{{if or .Video.Fx .Audio.Fx }}{{/* record fx if one on audio or video */}}
    {{.Video.Muxer}} name=wet_muxer faststart=true faststart-file={{.Folder}}/cache/{{.FilePrefix}}-wet.mp4mux.faststart !
    filesink name=wet_filesink location={{.Folder}}/recordings/{{.FilePrefix}}-wet.{{.Video.Extension}}
{{end}}

rtpbin. !
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
        {{.Audio.EncodeWith "audio_encoder_wet"}} ! 

        tee name=tee_audio_out ! 
            {{.Queue.Leaky}} ! 
            wet_muxer.

        tee_audio_out. ! 
            {{.FinalQueue}} leaky=2 ! 
            {{.Audio.Rtp.Pay}} !
            audio_rtp_sink.
{{else}}
    tee name=tee_audio_in ! 
        {{.Queue.Leaky}} ! 
        {{.Audio.Rtp.Depay}} !
        {{if .Video.Fx }}{{/* audio stream has to be written to two files if there is a video fx*/}}
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
        {{.FinalQueue}} leaky=2 ! 
        audio_rtp_sink.
{{end}}

rtpbin. !
{{if .Video.Fx}}
    {{.Queue.Base}} name=video_queue_bef_depay !
    {{.Video.Rtp.Depay}} ! 

    tee name=tee_video_in ! 
        {{.Queue.Base}} name=video_queue_bef_drymux ! 
        dry_muxer.

    tee_video_in. ! 
        {{.Queue.Base}} name=video_queue_bef_dec ! 
        {{.Video.Decoder}} !
        {{.Queue.Leaky}} name=video_queue_aft_dec ! 
        {{.Video.ConstraintFormat}} !

        videoconvert ! 
        {{.Queue.Base}} name=video_queue_bef_fx !
        {{.Video.Fx}} !
        {{.Queue.Base}} name=video_queue_aft_fx !
        {{if .Video.Overlay }}
            {{.Video.TimeOverlay }} ! 
        {{end}}

        {{.Video.ConstraintFormat}} !
        {{.Video.EncodeWithCache "video_encoder_wet" .Folder .FilePrefix}} ! 

        tee name=tee_video_out ! 
            {{.Queue.Base}} name=video_queue_bef_wetmux ! 
            wet_muxer.

        tee_video_out. ! 
            {{.FinalQueue}} name=video_queue_bef_sink ! 
            {{.Video.Rtp.Pay}} ! 
            video_rtp_sink.
{{else}}
    tee name=tee_video_in ! 
        {{.Queue.Base}} name=video_queue_bef_depay ! 
        {{.Video.Rtp.Depay}} ! 
        {{if .Audio.Fx }}{{/* video stream has to be written to two files if there is an aufio fx*/}}
            tee name=tee_video_out !
                {{.Queue.Base}} name=video_queue_bef_drymux ! 
                dry_muxer.
            tee_video_out. !
                {{.Queue.Base}} name=video_queue_bef_wetmux ! 
                wet_muxer.
        {{else}}
            dry_muxer.
        {{end}}
    tee_video_in. ! 
        {{.FinalQueue}} name=video_queue_bef_sink ! 
        video_rtp_sink.
{{end}}
