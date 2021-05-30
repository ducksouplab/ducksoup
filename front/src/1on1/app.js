// A marshalled params javascript object needs to be sent to DuckSoup
// - attended marshalling is encodeURI(btoa(JSON.stringify(params)))
// - params must contain:
//   - uid (string) a unique user identifier
//   - name (string) the user display name
//   - room (string) the room display name
//   - proc (boolean) to ask for media processing
//   - duration (integer) the duration of the experiment in seconds
// - params may contain:
//   - h264 (boolean) if h264 encoding should be preferred (vp8 is default)
//   - audio (object) merged with DuckSoup default constraints and passed to getUserMedia 
//   - video (object) merged with DuckSoup default constraints and passed to getUserMedia 

// Example :
// const params = {
//     uid: "uniqueId",
//     name: "nickname",
//     room: "hall",
//     proc: false,
//     duration: 30,
//     audio: {
//         deviceId: { ideal: "deviceId" }
//     }
// };

// State
let state = {};

// Config
const DEFAULT_CONSTRAINTS = {
    video: {
        width: { ideal: 640 },
        height: { ideal: 480 },
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
    // needed for safari: getUserMedia before enumerateDevices
    await navigator.mediaDevices.getUserMedia({ audio: true, video: true });
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
        window.parent.postMessage(message, window.location.origin)
    }
}

const areParamsValid = ({room, name, proc, duration, uid}) => {
    return  typeof room !== 'undefined' &&
            typeof uid !== 'undefined' &&
            typeof name !== 'undefined' &&
            typeof proc !== 'undefined' &&
            !isNaN(duration);
}

const init = async () => {
    // required state
    const params = unmarshallParams(getQueryVariable("params"));

    if (!areParamsValid(params)) {
        document.getElementById("placeholder").innerHTML = "Invalid parameters"
    } else {
        // prefer H264
        if (SUPPORT_SET_CODEC && params.h264) {
            const { codecs } = RTCRtpSender.getCapabilities('video');
            state.preferredCodecs = [...codecs].sort(({ mimeType: mt1 }, { mimeType: mt2 }) => {
                if (mt1.includes("264")) return -1;
                if (mt2.includes("264")) return 1;
                return 0;
            })
        }
        state = { ...state, ...params };

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
    const constraints = {
        audio: { ...DEFAULT_CONSTRAINTS.audio, ...state.audio },
        video: { ...DEFAULT_CONSTRAINTS.video, ...state.video },
    };
    const stream = await navigator.mediaDevices.getUserMedia(constraints);
    stream.getTracks().forEach((track) => pc.addTrack(track, stream));
    state.stream = stream;

    if (SUPPORT_SET_CODEC && state.h264) {
        const transceiver = pc.getTransceivers().find(t => t.sender && t.sender.track === stream.getVideoTracks()[0]);
        transceiver.setCodecPreferences(state.preferredCodecs);
    }

    // Signaling
    const wsProtocol = window.location.protocol === "https:" ? "wss" : "ws";
    const ws = new WebSocket(`${wsProtocol}://${window.location.host}/ws`);

    ws.onopen = function () {
        const { room, name, proc, duration, uid, h264 } = state;
        ws.send(
            JSON.stringify({
                kind: "join",
                payload: JSON.stringify({ room, name, duration, uid, proc, h264 }),
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
            console.log(message)
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