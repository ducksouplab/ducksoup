// State
let state = {};

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

const DEFAULT_PEER_CONFIGURATION = {
    iceServers: [
        {
            urls: "stun:stun.l.google.com:19302",
        },
    ],
};

const SUPPORT_SET_CODEC = window.RTCRtpTransceiver &&
    'setCodecPreferences' in window.RTCRtpTransceiver.prototype;

const isSafari = () => {
    const ua = navigator.userAgent;
    const containsChrome = ua.indexOf("Chrome") > -1;
    const containsSafari = ua.indexOf("Safari") > -1;
    return containsSafari && !containsChrome;
}

const IS_SAFARI = isSafari();

const getQueryVariable = (key, deserializeFunc) => {
    const query = window.location.search.substring(1);
    const vars = query.split("&");
    for (let i = 0; i < vars.length; i++) {
        const pair = vars[i].split("=");
        if (decodeURIComponent(pair[0]) == key) {
            const value = decodeURIComponent(pair[1]);
            return deserializeFunc ? deserializeFunc(value) : value;
        }
    }
};

const unmarshallParams = (str) => {
    try {
        return JSON.parse(atob(decodeURI(str)));
    } catch(err) {
        console.log(err);
        return null;
    }
}

const displayDevices = async () => {
    if(IS_SAFARI) {
        // needed for safari (getUserMedia before enumerateDevices) may be a problem if constraints change for Chrome
        await navigator.mediaDevices.getUserMedia(state.constraints);
    }
    const devices = await navigator.mediaDevices.enumerateDevices();
    const audioSourceEl = document.getElementById('audio-source');
    const videoSourceEl = document.getElementById('video-source');
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

const sendToParent = (message) => {
    if (window.parent) {
        window.parent.postMessage(message, state.origin)
    }
}

const areParamsValid = ({origin, room, name, proc, duration, uid}) => {
    return  typeof origin !== 'undefined' &&
            typeof room !== 'undefined' &&
            typeof uid !== 'undefined' &&
            typeof name !== 'undefined' &&
            typeof proc !== 'undefined' &&
            !isNaN(duration);
}

const clean = (obj) => {
    for (let prop in obj) {
      if (obj[prop] === null || obj[prop] === undefined) delete obj[prop];
    }
    return obj;
  }

const filterJoinPayload = (params) => {
    // explicit list, without origin
    let { room, name, proc, duration, uid, size, videoCodec, width, height, audioFx, videoFx } = params;
    if(!["vp8", "h264", "vp9"].includes(videoCodec)) videoCodec = null;
    if(isNaN(size)) size = null;
    if(isNaN(width)) width = null;
    if(isNaN(height)) height = null;

    return clean({ room, name, proc, duration, uid, size, videoCodec, width, height, audioFx, videoFx });
}

const init = async () => {
    // required join params
    let params = unmarshallParams(getQueryVariable("params"));

    if (!areParamsValid(params)) {
        document.getElementById("placeholder").innerHTML = "Invalid parameters"
    } else {
        const joinPayload = filterJoinPayload(params);
        // prefer specified codec
        if (SUPPORT_SET_CODEC && params.videoCodec) {
            const { codecs } = RTCRtpSender.getCapabilities('video');
            state.preferredCodecs = [...codecs].sort(({ mimeType: mt1 }, { mimeType: mt2 }) => {
                if (mt1.includes(params.videoCodec)) return -1;
                if (mt2.includes(params.videoCodec)) return 1;
                return 0;
            })
        }
        // save state
        state.joinPayload = joinPayload;
        state.origin = params.origin;
        state.constraints = {
            audio: { ...DEFAULT_CONSTRAINTS.audio, ...params.audio },
            video: { ...DEFAULT_CONSTRAINTS.video, ...params.video },
        };

        try {
            // Init UX
            await displayDevices();
            await startRTC();
        } catch (err) {
            console.error(err);
            stop("error");
        }
    }
};

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

const stop = (reason) => {
    const message = typeof reason === "string" ? { kind: reason } : reason;
    state.stream.getTracks().forEach((track) => track.stop());
    sendToParent(message);
}

const startRTC = async () => {
    // RTCPeerConnection
    const pc = new RTCPeerConnection(DEFAULT_PEER_CONFIGURATION);

    // Add local tracks before signaling
    const stream = await navigator.mediaDevices.getUserMedia(state.constraints);
    stream.getTracks().forEach((track) => {
        console.log(track.getSettings());
        pc.addTrack(track, stream);
    });
    state.stream = stream;

    if (SUPPORT_SET_CODEC && state.params && state.params.videoCodec) {
        const transceiver = pc.getTransceivers().find(t => t.sender && t.sender.track === stream.getVideoTracks()[0]);
        transceiver.setCodecPreferences(state.preferredCodecs);
    }

    // Signaling
    const wsProtocol = window.location.protocol === "https:" ? "wss" : "ws";
    const ws = new WebSocket(`${wsProtocol}://${window.location.host}/ws`);

    ws.onopen = function () {
        ws.send(
            JSON.stringify({
                kind: "join",
                payload: JSON.stringify(state.joinPayload),
            })
        );
    };

    ws.onclose = function () {
        console.log("[ws] closed");
        stop("disconnected");
    };

    ws.onerror = function (event) {
        console.error("[ws] error: " + event.data);
        stop("error");
    };

    ws.onmessage = async function (event) {
        let message = JSON.parse(event.data);
        if (!message) return console.error("[ws] can't parse message");

        if (message.kind === "offer") {
            const offer = JSON.parse(message.payload);
            if (!offer) {
                return console.error("[ws] can't parse offer");
            }
            console.log("[ws] received offer");
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
                return console.error("[ws] can't parse candidate");
            }
            console.log("[ws] candidate");
            pc.addIceCandidate(candidate);
        } else if (message.kind === "start") {
            console.log("[ws] start");
        } else if (message.kind === "finishing") {
            console.log("[ws] finishing");
            document.getElementById("finishing").classList.remove("d-none");
        } else if (message.kind.startsWith("error") || message.kind === "finish") {
            stop(message);
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

    pc.ontrack = function (event) {
        let el = document.createElement(event.track.kind);
        el.id = event.track.id;
        el.srcObject = event.streams[0];
        el.autoplay = true;
        document.getElementById("placeholder").appendChild(el);

        event.streams[0].onremovetrack = ({ track }) => {
            const el = document.getElementById(track.id);
            if (el) el.parentNode.removeChild(el);
        };
    };
};


document.addEventListener("DOMContentLoaded", init);