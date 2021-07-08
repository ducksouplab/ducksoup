document.addEventListener("DOMContentLoaded", () => {
    console.log("[DuckSoup] v1.0.5")
});

// Use single quote in templace since will be used as an iframe srcdoc value
const TEMPLATE = `<!DOCTYPE html>
<html>
    <head>
        <title>DuckSoup</title>
        <meta charset='utf-8'>
        <link rel='shortcut icon' href='data:image/x-icon;,' type='image/x-icon'>
        <link href='https://cdn.jsdelivr.net/npm/bootstrap@5.0.0-beta3/dist/css/bootstrap.min.css' rel='stylesheet'
            integrity='sha384-eOJMYsd53ii+scO/bJGFsiCZc+5NDVN2yr8+0RDqr0Ql0h+rP48ckxlpbzKgwra6' crossorigin='anonymous'>
        <script src='https://cdn.jsdelivr.net/npm/bootstrap@5.0.0-beta3/dist/js/bootstrap.min.js'
            integrity='sha384-j0CNLUeiqtyaRmlzUHCPZ+Gy5fQu0dQ6eZ/xAww941Ai1SxSY+0EQqNXNE6DZiVc'
            crossorigin='anonymous'></script>
        <style type='text/css'>
            html, body, .placeholder, video {
                width: 100%;
                height: 100%;
                overflow: hidden;
            }
            .settings {
                position: absolute;
                bottom: 0;
                left: 0;
            }
            .settings .trigger {
                position: absolute;
                width: 26px;
                height: 26px;
                top: -40px;
                left: 14px;
                color: white;
                background-color: #bbbbbbaa;
                border-radius: 5px;
                cursor: pointer;
                display: flex;
                justify-content: center;
                align-items: center;
            }
            .ending {
                position: absolute;
                bottom: 0;
                right: 0;
                color: white;
                border-radius: 5px;
            }
            .ending div {
                position: relative;
                text-align: right;
                background-color: #bbbbbbcc;
                height: 26px;
                top: -14px;
                right: 14px;
                opacity: 1;
                padding: 0 8px;
                border-radius: 5px;
            }
            .modal .btn-close {
                position: absolute;
                top: 12px;
                right: 12px;
            } 
            .modal-body {
                padding: 1.5rem 1rem 0.5rem;
            }
        </style>
    </head>
    <body>
        <div class='placeholder'></div>
        <div class='settings'>
            <div class='trigger' data-bs-toggle='modal' data-bs-target='#modal-settings'>
                <svg xmlns='http://www.w3.org/2000/svg' width='20' height='20' fill='currentColor' class='bi bi-gear-fill'
                    viewBox='0 0 16 16'>
                    <path
                        d='M9.405 1.05c-.413-1.4-2.397-1.4-2.81 0l-.1.34a1.464 1.464 0 0 1-2.105.872l-.31-.17c-1.283-.698-2.686.705-1.987 1.987l.169.311c.446.82.023 1.841-.872 2.105l-.34.1c-1.4.413-1.4 2.397 0 2.81l.34.1a1.464 1.464 0 0 1 .872 2.105l-.17.31c-.698 1.283.705 2.686 1.987 1.987l.311-.169a1.464 1.464 0 0 1 2.105.872l.1.34c.413 1.4 2.397 1.4 2.81 0l.1-.34a1.464 1.464 0 0 1 2.105-.872l.31.17c1.283.698 2.686-.705 1.987-1.987l-.169-.311a1.464 1.464 0 0 1 .872-2.105l.34-.1c1.4-.413 1.4-2.397 0-2.81l-.34-.1a1.464 1.464 0 0 1-.872-2.105l.17-.31c.698-1.283-.705-2.686-1.987-1.987l-.311.169a1.464 1.464 0 0 1-2.105-.872l-.1-.34zM8 10.93a2.929 2.929 0 1 1 0-5.86 2.929 2.929 0 0 1 0 5.858z' />
                </svg>
            </div>
        </div>
        <div class='ending' style='display:none'>
            <div>Conversation bientôt terminée</div>
        </div>
        <div class='modal' id='modal-settings' tabindex='-1'>
            <div class='modal-dialog modal-dialog-centered'>
                <div class='modal-content'>
                    <div class='modal-body'>
                        <button type='button' class='btn-close' data-bs-dismiss='modal' aria-label='Close'></button>
                        <div class='row'>
                            <div class='col'>
                                <div class='mb-3'>
                                    <label for='video-source' class='form-label'>Choix de la source vidéo </label>
                                    <select id='video-source' class='video-source form-select form-select-sm'></select>
                                </div>
                                <div class='mb-3'>
                                    <label for='audio-source' class='form-label'>Choix de l'entrée audio</label>
                                    <select id='audio-source' class='audio-source form-select form-select-sm'></select>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </div>

        <script type="text/javascript">
        document.addEventListener("DOMContentLoaded", () => {
            window.parent.postMessage("DuckSoupContainerLoaded");
        });
        </script>
    </body>
</html>`;

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

const SUPPORT_SET_CODEC = window.RTCRtpTransceiver &&
    'setCodecPreferences' in window.RTCRtpTransceiver.prototype;

const IS_SAFARI = (() => {
    const ua = navigator.userAgent;
    const containsChrome = ua.indexOf("Chrome") > -1;
    const containsSafari = ua.indexOf("Safari") > -1;
    return containsSafari && !containsChrome;
})();

// Pure functions

const areOptionsValid = ({ room, name, duration, uid }) => {
    return typeof room !== 'undefined' &&
        typeof uid !== 'undefined' &&
        typeof name !== 'undefined' &&
        !isNaN(duration);
}

const clean = (obj) => {
    for (let prop in obj) {
        if (obj[prop] === null || obj[prop] === undefined) delete obj[prop];
    }
    return obj;
}

const parseJoinPayload = (peerOptions) => {
    // explicit list, without origin
    let { room, uid, name, duration, size, width, height, audioFx, videoFx, frameRate, namespace, videoCodec } = peerOptions;
    if (!["vp8", "h264", "vp9"].includes(videoCodec)) videoCodec = null;
    if (isNaN(size)) size = null;
    if (isNaN(width)) width = null;
    if (isNaN(height)) height = null;
    if (isNaN(frameRate)) frameRate = null;

    return clean({ room, uid, name, duration, size, width, height, audioFx, videoFx, frameRate, namespace, videoCodec });
}

const forceMozillaMono = (sdp) => {
    if (!window.navigator.userAgent.includes("Mozilla")) return sdp;
    return sdp
        .split("\r\n")
        .map((line) => {
            if (line.startsWith("a=fmtp:111")) {
                return line.replace("stereo=1", "stereo=0");
            } else {
                return line;
            }
        })
        .join("\r\n");
};

const processSDP = (sdp) => {
    const output = forceMozillaMono(sdp);
    return output;
};

const kbps = (bytes, duration) => {
    const result = (8 * bytes) / duration / 1024;
    return result.toFixed(1);
};

// DuckSoup

class DuckSoup {

    // API

    constructor(document, peerOptions, embedOptions) {
        if (!areOptionsValid(peerOptions)) {
            document.querySelector(".placeholder").innerHTML = "Invalid DuckSoup options"
        } else {
            this._document = document;
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
            // prefer specified codec
            if (SUPPORT_SET_CODEC && peerOptions.videoCodec) {
                const { codecs } = RTCRtpSender.getCapabilities('video');
                this._preferredCodecs = [...codecs].sort(({ mimeType: mt1 }, { mimeType: mt2 }) => {
                    if (mt1.includes(peerOptions.videoCodec)) return -1;
                    if (mt2.includes(peerOptions.videoCodec)) return 1;
                    return 0;
                })
            }

            
        }
    };

    audioControl(effectName, property, value, transitionDuration) {
        this._control("audio", effectName, property, value, transitionDuration);
    }

    videoControl(effectName, property, value, transitionDuration) {
        this._control("video", effectName, property, value, transitionDuration);
    }

    stop() {
        this._stream.getTracks().forEach((track) => track.stop());
        this._pc.close();
        this._ws.close();
    }

    get stream() {
        return this._stream;
    }

    // Inner methods

    async _initialize() {
        try {
            // async calls
            await this._renderDevices();
            await this._startRTC();
        } catch (err) {
            this._postStop({ kind: "error", payload: err });
        }
    }

    _checkControl(name, property, value, duration) {
        const durationValid = typeof duration === "undefined" || typeof duration === "number"
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


    _postMessage(message) {
        if (this._callback) this._callback(message);
    }

    _postStop(reason) {
        const message = typeof reason === "string" ? { kind: reason } : reason;
        this.stop();
        this._postMessage(message);
        if (this._debugIntervalId) clearInterval(this._debugIntervalId);
    }

    async _startRTC() {
        // RTCPeerConnection
        const pc = new RTCPeerConnection(this._rtcConfig);
        this._pc = pc;

        // Add local tracks before signaling
        const stream = await navigator.mediaDevices.getUserMedia(this._constraints);
        stream.getTracks().forEach((track) => {
            pc.addTrack(track, stream);
        });
        this._stream = stream;

        if (SUPPORT_SET_CODEC && this._joinPayload && this._joinPayload.videoCodec) {
            const transceiver = pc.getTransceivers().find(t => t.sender && t.sender.track === stream.getVideoTracks()[0]);
            transceiver.setCodecPreferences(this._preferredCodecs);
        }

        // Signaling
        const wsProtocol = window.location.protocol === "https:" ? "wss" : "ws";
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

        ws.onclose = () => {
            this._postStop("disconnection");
        };

        ws.onerror = (event) => {
            this._postStop({ kind: "error", payload: event.data });
        };

        ws.onmessage = async (event) => {
            let message = JSON.parse(event.data);

            if (!message) return this._postStop({ kind: "error", payload: "can't parse message" });

            if (message.kind === "offer") {
                const offer = JSON.parse(message.payload);
                if (!offer) {
                    return this._postStop({ kind: "error", payload: "can't parse offer" });
                }
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
                const candidate = JSON.parse(message.payload);
                if (!candidate) {
                    return this._postStop({ kind: "error", payload: "can't parse candidate" });
                }
                pc.addIceCandidate(candidate);
            } else if (message.kind === "start") {
                this._callback({ kind: "start " });
            } else if (message.kind === "ending") {
                this._document.querySelector(".ending").style.display = 'block';
            } else if (message.kind.startsWith("error") || message.kind === "end") {
                this._document.querySelector("body").style.display = 'none';
                this._postStop(message);
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
            let el = document.createElement(event.track.kind);
            el.id = event.track.id;
            el.srcObject = event.streams[0];
            el.autoplay = true;
            this._document.querySelector(".placeholder").appendChild(el);

            event.streams[0].onremovetrack = ({ track }) => {
                const el = document.getElementById(track.id);
                if (el) el.parentNode.removeChild(el);
            };
        };

        // Stats
        if (this._debug) {
            this._debugIntervalId = setInterval(() => this._updateStats(), 1000);
        }
    }

    async _renderDevices() {
        if (IS_SAFARI) {
            // needed for safari (getUserMedia before enumerateDevices) may be a problem if constraints change for Chrome
            await navigator.mediaDevices.getUserMedia(this._constraints);
        }
        const devices = await navigator.mediaDevices.enumerateDevices();
        const audioSourceEl = this._document.querySelector('.audio-source');
        const videoSourceEl = this._document.querySelector('.video-source');
        for (let i = 0; i !== devices.length; ++i) {
            const device = devices[i];
            const option = document.createElement('option');
            option.value = device.deviceId;
            if (device.kind === 'audioinput') {
                option.text = device.label || `microphone ${audioSourceEl.length + 1}`;
                audioSourceEl.appendChild(option);
            } else if (device.kind === 'videoinput') {
                option.text = device.label || `camera ${videoSourceEl.length + 1}`;
                videoSourceEl.appendChild(option);
            }
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

        pcStats.forEach((report) => {
            if (report.type === "outbound-rtp" && report.kind === "audio") {
                newAudioBytesSent += report.bytesSent;
            } else if (report.type === "inbound-rtp" && report.kind === "audio") {
                newAudioBytesReceived += report.bytesReceived;
            } else if (report.type === "outbound-rtp" && report.kind === "video") {
                newVideoBytesSent += report.bytesSent;
            } else if (report.type === "inbound-rtp" && report.kind === "video") {
                newVideoBytesReceived += report.bytesReceived;
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
        this._postMessage({
            kind: "stats",
            payload: { audioUp, audioDown, videoUp, videoDown }
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
    render: async (mountEl, peerOptions, embedOptions) => {
        const iframe = document.createElement("iframe");
        iframe.srcdoc = TEMPLATE;
        iframe.width = "100%";
        iframe.height = "100%";

        // replace mountEl contents
        while (mountEl.firstChild) {
            mountEl.removeChild(mountEl.firstChild);
        }
        mountEl.appendChild(iframe);
        
        const iframeWindow = iframe.contentWindow;

        const waitForDOMContentLoaded = new Promise((resolve) => {
            iframeWindow.addEventListener('DOMContentLoaded', resolve);
        });
        const waitForDuckSoupContainerLoaded = new Promise((resolve) => {
            window.addEventListener('message', (event) => {
                if (event.data === "DuckSoupContainerLoaded") resolve();
            });
        });

        // on safari, DOMContentLoaded won't be triggered for this iframe, so we wait for the fastest triggered event
        await Promise.race([waitForDOMContentLoaded, waitForDuckSoupContainerLoaded]);
        const player = new DuckSoup(iframeWindow.document, peerOptions, embedOptions);
        await player._initialize();
        return player;
    }
}