appsrc name=audio_rtp_src is-live=true format=GST_FORMAT_TIME do-timestamp=true

appsrc name=audio_rtcp_src ! audio_buffer.sink_rtcp

appsink name=audio_rtp_sink

{{/* always record dry */}}
{{.Audio.Muxer}} name=dry_audio_muxer !
filesink name=dry_audio_filesink location={{.Folder}}/recordings/{{.FilePrefix}}-audio-dry.{{.Audio.Extension}} 

{{if .Audio.Fx }}
    {{.Audio.Muxer}} name=wet_audio_muxer !
    filesink name=wet_audio_filesink location={{.Folder}}/recordings/{{.FilePrefix}}-audio-wet.{{.Audio.Extension}} 
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
        {{.Audio.EncodeWith "audio_encoder_dry"}} !

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