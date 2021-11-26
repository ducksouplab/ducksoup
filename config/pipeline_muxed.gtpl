appsrc name=audio_src format=time is-live=true format=GST_FORMAT_TIME
appsrc name=video_src format=time is-live=true format=GST_FORMAT_TIME
appsink name=audio_sink qos=true
appsink name=video_sink qos=true
{{/* always record raw */}}
matroskamux name=raw_recorder ! filesink location=data/{{.Namespace}}/{{.FilePrefix}}-raw.mkv
{{/* record fx if one on audio or video */}}
{{if or .VideoFx .AudioFx }}
    matroskamux name=fx_recorder ! filesink location=data/{{.Namespace}}/{{.FilePrefix}}-fx.mkv
{{end}}

audio_src. !
{{.Audio.Rtp.Caps}} ! 
{{if .AudioFx}}
    rtpjitterbuffer name=audio_buffer latency={{.RTPJitterBuffer.Latency}} do-retransmission={{.RTPJitterBuffer.Retransmission}} ! 
    {{.Audio.Rtp.Depay}} !
    tee name=tee_opus_raw ! 
    queue max-size-buffers=0 max-size-bytes=0 max-size-time=5000000000 ! 
    raw_recorder.

    tee_opus_raw. ! 
    queue max-size-buffers=0 max-size-bytes=0 ! 
    {{.Audio.Decode}} !
    audioconvert ! 
    {{.AudioFx}} ! 
    audioconvert ! 
    {{.Audio.Encode.Fx}} ! 
    tee name=tee_opus_fx ! 
    queue max-size-buffers=0 max-size-bytes=0 max-size-time=5000000000 ! 
    fx_recorder.

    tee_opus_fx. ! 
    queue max-size-buffers=0 max-size-bytes=0 ! 
    {{.Audio.Rtp.Pay}} !
    audio_sink.
{{else}}
    tee name=tee_opus_raw ! 
    queue max-size-buffers=0 max-size-bytes=0 max-size-time=5000000000 ! 
    rtpjitterbuffer name=audio_buffer latency={{.RTPJitterBuffer.Latency}} do-retransmission={{.RTPJitterBuffer.Retransmission}} ! 
    {{.Audio.Rtp.Depay}} !
    {{/* audio stream has to be written to two files if there is a video fx*/}}
    {{if .VideoFx }}
        tee name=tee_opus_record !
        queue max-size-buffers=0 max-size-bytes=0 ! 
        raw_recorder.
        tee_opus_record. !
        queue max-size-buffers=0 max-size-bytes=0 !
        fx_recorder.
    {{else}}
        raw_recorder.
    {{end}}

    tee_opus_raw. ! 
    queue max-size-buffers=0 max-size-bytes=0 ! 
    audio_sink.
{{end}}

video_src. !
{{.Video.Rtp.Caps}} ! 
{{if .VideoFx}}
    rtpjitterbuffer name=video_buffer latency={{.RTPJitterBuffer.Latency}} do-retransmission={{.RTPJitterBuffer.Retransmission}} ! 
    {{.Video.Rtp.Depay}} ! 
    {{.Video.Decode}} !
    videoconvert ! 
    videorate ! 
    videoscale ! 
    video/x-raw{{.FrameRate}}{{.Width}}{{.Height}}, format=I420, colorimetry=bt601, chroma-site=jpeg, pixel-aspect-ratio=1/1 ! 

    tee name=tee_video_raw ! 
    queue max-size-buffers=0 max-size-bytes=0 max-size-time=5000000000 ! 
    {{.Video.Encode.Raw}} !
    raw_recorder.

    tee_video_raw. ! 
    queue max-size-buffers=0 max-size-bytes=0 ! 
    videoconvert ! 
    {{.VideoFx}} ! 
    videoconvert ! 
    video/x-raw, format=I420, colorimetry=bt601, chroma-site=jpeg ! 
    {{.Video.Encode.Fx}} !

    tee name=tee_video_fx ! 
    queue max-size-buffers=0 max-size-bytes=0 max-size-time=5000000000 ! 
    fx_recorder.

    tee_video_fx. ! 
    queue max-size-buffers=0 max-size-bytes=0 ! 
    {{.Video.Rtp.Pay}} ! 
    video_sink.
{{else}}
    tee name=tee_video_raw ! 
    queue max-size-buffers=0 max-size-bytes=0 max-size-time=5000000000 ! 
    rtpjitterbuffer name=video_buffer latency={{.RTPJitterBuffer.Latency}} do-retransmission={{.RTPJitterBuffer.Retransmission}} ! 
    {{.Video.Rtp.Depay}} ! 
    {{.Video.Decode}} !
    videoconvert ! 
    videorate ! 
    videoscale ! 
    video/x-raw{{.FrameRate}}{{.Width}}{{.Height}}, format=I420, colorimetry=bt601, chroma-site=jpeg, pixel-aspect-ratio=1/1 ! 
    {{.Video.Encode.Raw}} !
    {{/* video stream has to be written to two files if there is an aufio fx*/}}
    {{if .AudioFx }}
        tee name=tee_video_record !
        queue max-size-buffers=0 max-size-bytes=0 ! 
        raw_recorder.
        tee_video_record. !
        queue max-size-buffers=0 max-size-bytes=0 !
        fx_recorder.
    {{else}}
        raw_recorder.
    {{end}}

    tee_video_raw. ! 
    queue max-size-buffers=0 max-size-bytes=0 ! 
    video_sink.
{{end}}
