{{.RTPBin}}

appsrc name=audio_rtp_src is-live=true format=GST_FORMAT_TIME do-timestamp=true ! {{.Audio.Rtp.Caps}} ! rtpbin.recv_rtp_sink_0

appsrc name=audio_rtcp_src ! rtpbin.recv_rtcp_sink_0

appsink name=audio_rtp_sink

{{.Audio.Muxer}} name=dry_muxer !
filesink location={{.Folder}}/recordings/{{.FilePrefix}}-audio-dry.{{.Audio.Extension}} 

{{if .Audio.Fx }}{{/* record fx if any */}}
    {{.Audio.Muxer}} name=wet_muxer !
    filesink location={{.Folder}}/recordings/{{.FilePrefix}}-audio-wet.{{.Audio.Extension}} 
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
        {{.Audio.EncodeWith "audio_encoder_dry"}} !

        tee name=tee_audio_out ! 
            {{.Queue.Leaky}} ! 
            wet_muxer.

        tee_audio_out. ! 
            {{.Queue.Leaky}} ! 
            {{.Audio.Rtp.Pay}} !
            {{.FinalQueue}} name=video_queue_bef_sink ! 
            audio_rtp_sink.
{{else}}
    tee name=tee_audio_in ! 
        {{.Queue.Leaky}} ! 
        {{.Audio.Rtp.Depay}} !
        dry_muxer.
 
    tee_audio_in. ! 
        {{.FinalQueue}} name=video_queue_bef_sink ! 
        audio_rtp_sink.
{{end}}