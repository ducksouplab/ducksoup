
// Config

const DEFAULT_CONSTRAINTS = {
    video: {
        width: { ideal: 800 },
        height: { ideal: 600 },
        frameRate: { ideal: 30 },
        facingMode: { ideal: "user" },
    },
    audio: {
        sampleSize: 16,
        channelCount: 1,
        autoGainControl: false,
        latency: { ideal: 0.003 },
        echoCancellation: false,
        noiseSuppression: false,
    },
};

const DEFAULT_RTC_CONFIG = {
    iceServers: [
        {
            urls: "stun:stun.l.google.com:19302",
        },
    ],
};

const MAX_VIDEO_BITRATE = 1000000;
const MAX_AUDIO_BITRATE = 64000;

// Init

document.addEventListener("DOMContentLoaded", async () => {
    console.log("[DuckSoup] v1.3.3");

    const ua = navigator.userAgent;
    const containsChrome = ua.indexOf("Chrome") > -1;
    const containsSafari = ua.indexOf("Safari") > -1;
    // needed for safari (getUserMedia before enumerateDevices), but could be a problem if constraints change for Chrome
    if(containsSafari && !containsChrome) {
        await navigator.mediaDevices.getUserMedia(DEFAULT_CONSTRAINTS);
    }
});


// Pure functions

const optionsFirstError = ({ mountEl, callback }, { roomId, userId, duration }) => {
    if (!mountEl && !callback) return "invalid embedOptions";
    if (typeof roomId === 'undefined' || typeof userId === 'undefined' || isNaN(duration)) return "invalid peerOptions";
    return null;
};

const clean = (obj) => {
    for (let prop in obj) {
        if (obj[prop] === null || obj[prop] === undefined) delete obj[prop];
    }
    return obj;
};

const parseJoinPayload = (peerOptions) => {
    // explicit list, without origin
    let { roomId, userId, duration, size, width, height, audioFx, videoFx, frameRate, namespace, videoFormat, gpu } = peerOptions;
    if (!["VP8", "H264"].includes(videoFormat)) videoFormat = null;
    if (isNaN(size)) size = null;
    if (isNaN(width)) width = null;
    if (isNaN(height)) height = null;
    if (isNaN(frameRate)) frameRate = null;
    if (!gpu) gpu = null;

    return clean({ roomId, userId, duration, size, width, height, audioFx, videoFx, frameRate, namespace, videoFormat, gpu });
};

const forceMozillaMono = (sdp) => {
    if (!window.navigator.userAgent.includes("Mozilla")) return sdp;
    return sdp
        .split("\r\n")
        .map((line) => {
            if (line.startsWith("a=fmtp:111")) {
                if(line.includes("stereo=")) {
                    return line.replace("stereo=1", "stereo=0");
                } else {
                    return `${line};stereo=0`;
                }
            } else {
                return line;
            }
        })
        .join("\r\n");
};

const addTWCC = (sdp) => {
    // TODO improved parsing/placing/indexing of additional extmap
    return sdp
        .split("\r\n")
        .map((line) => {
            if (line.startsWith("a=extmap:2 ")) {
                return line + "\r\na=extmap:3 http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01"
            } else {
                return line;
            }
        })
        .join("\r\n");
};

const processSDP = (sdp) => {
    let output = forceMozillaMono(sdp);
    // output = addTWCC(output);
    return output;
};

const kbps = (bytes, duration) => {
    const result = (8 * bytes) / duration / 1024;
    return result.toFixed(1);
};

const looseJSONParse = (str) => {
    try {
        return JSON.parse(str);
    } catch (error) {
        console.error(error);
    }
};

// DuckSoup

class DuckSoup {

    // API

    constructor(embedOptions, peerOptions) {
        const err = optionsFirstError(embedOptions, peerOptions);
        if (err) throw new Error(err);

        const { mountEl } = embedOptions;
        if (mountEl) {
            this._mountEl = mountEl;
            // replace mountEl contents
            while (mountEl.firstChild) {
                mountEl.removeChild(mountEl.firstChild);
            }
        }
        this._signalingUrl = peerOptions.signalingUrl;
        this._rtcConfig = peerOptions.rtcConfig || DEFAULT_RTC_CONFIG;
        this._joinPayload = parseJoinPayload(peerOptions);
        this._constraints = {
            audio: { ...DEFAULT_CONSTRAINTS.audio, ...peerOptions.audio },
            video: { ...DEFAULT_CONSTRAINTS.video, ...peerOptions.video },
        };
        this._debug = embedOptions && embedOptions.debug;
        this._callback = embedOptions && embedOptions.callback;
        if (this._debug) {
            this._debugInfo = {
                now: Date.now(),
                audioBytesSent: 0,
                audioBytesReceived: 0,
                videoBytesSent: 0,
                videoBytesReceived: 0
            };
        }
    };

    audioControl(effectName, property, value, transitionDuration) {
        this._control("audio", effectName, property, value, transitionDuration);
    }

    videoControl(effectName, property, value, transitionDuration) {
        this._control("video", effectName, property, value, transitionDuration);
    }

    // https://datatracker.ietf.org/doc/html/rfc6455#section-7.4.1
    stop(wsStatusCode = 1000) {
        this._stopRTC()
        this._ws.close(wsStatusCode);
    }

    get stream() {
        return this._stream;
    }

    // Inner methods

    async _initialize() {
        try {
            // async calls
            await this._startRTC();
            this._running = true;
        } catch (err) {
            this._sendStop({ kind: "error", payload: err });
        }
    }

    _checkControl(name, property, value, duration) {
        const durationValid = typeof duration === "undefined" || typeof duration === "number";
        return typeof name === "string" && typeof property === "string" && typeof value === "number" && durationValid;
    }

    _control(kind, name, property, value, duration) {
        if(!this._checkControl(name, property, value, duration)) return;
        this._ws.send(
            JSON.stringify({
                kind: "control",
                payload: JSON.stringify({ kind, name, property, value, ...(duration && { duration }) }),
            })
        );
    }


    _sendEvent(message, force) {
        if (this._callback && (this._running || force)) this._callback(message);
    }

    _sendStop(reason) {
        const message = typeof reason === "string" ? { kind: reason } : reason;
        this._sendEvent(message);
        if (this._debugIntervalId) clearInterval(this._debugIntervalId);
    }

    _stopRTC() {
        //console.log("[DuckSoup] _stopRTC");
        if(this._stream) {
            this._stream.getTracks().forEach((track) => track.stop());
        }
        if(this._pc) {
            this._pc.close();
        }
    }

    async _startRTC() {
        // RTCPeerConnection
        const pc = new RTCPeerConnection(this._rtcConfig);
        this._pc = pc;

        // Add local tracks before signaling
        const stream = await navigator.mediaDevices.getUserMedia(this._constraints);
        stream.getTracks().forEach((track) => {
            // implement a mute-like behavior (with `enabled`) until the room does start
            // see https://developer.mozilla.org/en-US/docs/Web/API/MediaStreamTrack/enabled
            track.enabled = false;
            pc.addTrack(track, stream);
        });
        this._stream = stream;

        // Signaling
        const ws = new WebSocket(this._signalingUrl);
        this._ws = ws;

        ws.onopen = () => {
            ws.send(
                JSON.stringify({
                    kind: "join",
                    payload: JSON.stringify(this._joinPayload),
                })
            );
        };

        ws.onclose = (event) => {
            if(event.code === 1000) {
                this._sendStop("closed");
            } else {
                this._sendStop("error-disconnection");
            }
            this._stopRTC();
        };

        ws.onerror = (event) => {
            //console.log("[DuckSoup] ws.onerror ", event);
            this._sendStop({ kind: "error", payload: event.data });
            this.stop(4000); // used as error
        };

        ws.onmessage = async (event) => {
            //console.log("[DuckSoup] ws.onmessage ", event);
            let message = looseJSONParse(event.data);

            if (message.kind === "offer") {
                const offer = looseJSONParse(message.payload);
                pc.setRemoteDescription(offer);
                const answer = await pc.createAnswer();
                answer.sdp = processSDP(answer.sdp);
                pc.setLocalDescription(answer);
                ws.send(
                    JSON.stringify({
                        kind: "answer",
                        payload: JSON.stringify(answer),
                    })
                );
            } else if (message.kind === "candidate") {
                const candidate = looseJSONParse(message.payload);
                try {
                    pc.addIceCandidate(candidate);
                } catch (error) {
                    console.error(error)
                }
            } else if (message.kind === "start") {
                // set encoding parameters
                for(const sender of pc.getSenders()) {
                    const params = sender.getParameters();
                    if(!params.encodings) params.encodings = [{}];// needed for FF
                    for(const encoding of params.encodings) {
                        encoding.maxBitrate = sender.track.kind === "video" ? MAX_VIDEO_BITRATE : MAX_AUDIO_BITRATE;
                    }
                    await sender.setParameters(params);
                }
                // unmute
                stream.getTracks().forEach((track) => {
                    track.enabled = true;
                });
                this._sendEvent({ kind: "start" }, true); // force with true since player is not already running
            } else if (message.kind === "ending") {
                this._sendEvent({ kind: "ending" });
            } else if (message.kind.startsWith("error") || message.kind === "end") {
                this._sendStop(message);
                this.stop(1000); // Normal Closure
            }
        };

        pc.onicecandidate = (e) => {
            if (!e.candidate) return;
            ws.send(
                JSON.stringify({
                    kind: "candidate",
                    payload: JSON.stringify(e.candidate),
                })
            );
        };

        pc.ontrack = (event) => {
            if(this._mountEl) {
                let el = document.createElement(event.track.kind);
                el.id = event.track.id;
                el.srcObject = event.streams[0];
                el.autoplay = true;
                if(event.track.kind === "video") {
                    if(this._joinPayload.width) {
                        el.style.width = this._joinPayload.width + "px";
                    } else {
                        el.style.width = "100%";
                    }
                    if(this._joinPayload.height) {
                        el.style.height = this._joinPayload.height + "px";
                    } 
                }
                this._mountEl.appendChild(el);
                // on remove
                event.streams[0].onremovetrack = ({ track }) => {
                    const el = document.getElementById(track.id);
                    if (el) el.parentNode.removeChild(el);
                };
            } else {
                this._sendEvent({
                    kind: "track",
                    payload: event
                });
            }
        };

        // Stats
        if (this._debug) {
            this._debugIntervalId = setInterval(() => this._updateStats(), 1000);
        }
    }

    async _updateStats() {
        const pc = this._pc;
        const pcStats = await pc.getStats();
        const newNow = Date.now();
        let newAudioBytesSent = 0;
        let newAudioBytesReceived = 0;
        let newVideoBytesSent = 0;
        let newVideoBytesReceived = 0;
        let outboundRTPVideo, inboundRTPVideo, outboundRTPAudio, inboundRTPAudio;
        let remoteOutboundRTPVideo, remoteInboundRTPVideo, remoteOutboundRTPAudio, remoteInboundRTPAudio;

        pcStats.forEach((report) => {
            if (report.type === "outbound-rtp" && report.kind === "audio") {
                newAudioBytesSent += report.bytesSent;
                outboundRTPAudio = report;
            } else if (report.type === "inbound-rtp" && report.kind === "audio") {
                newAudioBytesReceived += report.bytesReceived;
                inboundRTPAudio = report;
            } else if (report.type === "outbound-rtp" && report.kind === "video") {
                newVideoBytesSent += report.bytesSent;
                outboundRTPVideo = report;
            } else if (report.type === "inbound-rtp" && report.kind === "video") {
                newVideoBytesReceived += report.bytesReceived;
                inboundRTPVideo = report;
            } else if (report.type === "remote-outbound-rtp" && report.kind === "audio") {
                remoteOutboundRTPAudio = report;
            } else if (report.type === "remote-inbound-rtp" && report.kind === "audio") {
                remoteInboundRTPAudio = report;
            } else if (report.type === "remote-outbound-rtp" && report.kind === "video") {
                remoteOutboundRTPVideo = report;
            } else if (report.type === "remote-inbound-rtp" && report.kind === "video") {
                remoteInboundRTPVideo = report;
            }
        });

        const elapsed = (newNow - this._debugInfo.now) / 1000;
        const audioUp = kbps(
            newAudioBytesSent - this._debugInfo.audioBytesSent,
            elapsed
        );
        const audioDown = kbps(
            newAudioBytesReceived - this._debugInfo.audioBytesReceived,
            elapsed
        );
        const videoUp = kbps(
            newVideoBytesSent - this._debugInfo.videoBytesSent,
            elapsed
        );
        const videoDown = kbps(
            newVideoBytesReceived - this._debugInfo.videoBytesReceived,
            elapsed
        );
        this._sendEvent({
            kind: "stats",
            payload: {
                audioUp,
                audioDown,
                videoUp,
                videoDown,
                outboundRTPVideo,
                inboundRTPVideo,
                outboundRTPAudio,
                inboundRTPAudio,
                remoteOutboundRTPVideo,
                remoteInboundRTPVideo,
                remoteOutboundRTPAudio,
                remoteInboundRTPAudio
            }
        });

        this._debugInfo = {
            now: newNow,
            audioBytesSent: newAudioBytesSent,
            audioBytesReceived: newAudioBytesReceived,
            videoBytesSent: newVideoBytesSent,
            videoBytesReceived: newVideoBytesReceived
        };
    }
}

// API

window.DuckSoup = {
    render: async (embedOptions, peerOptions) => {
        const player = new DuckSoup(embedOptions, peerOptions);
        await player._initialize();
        return player;
    }
};