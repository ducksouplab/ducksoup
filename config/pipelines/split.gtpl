{{.RTPBin}}

appsrc name=audio_rtp_src is-live=true format=GST_FORMAT_TIME do-timestamp=true ! {{.Audio.Rtp.Caps}} ! rtpbin.recv_rtp_sink_0
appsrc name=video_rtp_src is-live=true format=GST_FORMAT_TIME do-timestamp=true ! {{.Video.Rtp.Caps}} ! rtpbin.recv_rtp_sink_1

appsrc name=audio_rtcp_src ! rtpbin.recv_rtcp_sink_0
appsrc name=video_rtcp_src ! rtpbin.recv_rtcp_sink_1

appsink name=audio_rtp_sink
appsink name=video_rtp_sink qos=true

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

rtpbin. !
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

rtpbin. !
{{if .Video.Fx}}
    {{.Video.Rtp.Depay}} ! 

    tee name=tee_video_in ! 
        {{.Queue.Base}} !
        dry_video_muxer.

    tee_video_in. ! 
        {{.Queue.Base}} !
        {{.Video.Decoder}} !
        {{.Queue.Leaky}} !
        {{.Video.ConstraintFormat}} !

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
        {{.Queue.Base}} ! 
        dry_video_muxer.

    tee_video_in. ! 
        {{.Queue.Base}} ! 
        video_rtp_sink.
{{end}}
