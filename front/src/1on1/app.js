// State
let state = {
    audioIn: null,
};

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
        autoGainControl: false,
        channelCount: 1,
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

const getQueryVariable = (key) => {
    const query = window.location.search.substring(1);
    const vars = query.split("&");
    for (let i = 0; i < vars.length; i++) {
        const pair = vars[i].split("=");
        if (decodeURIComponent(pair[0]) == key) {
            return decodeURIComponent(pair[1]);
        }
    }
};

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

const init = async () => {
    // required state
    const room = getQueryVariable("room");
    const name = getQueryVariable("name");
    const proc = getQueryVariable("proc");
    const duration = parseInt(getQueryVariable("duration"), 10);
    const uid = getQueryVariable("uid");
    // optional
    const audioDeviceId = getQueryVariable("aid");
    const videoDeviceId = getQueryVariable("vid");

    if (typeof room === 'undefined' || typeof name === 'undefined' || !["0", "1"].includes(proc) || isNaN(duration) || typeof uid === 'undefined') {
        document.getElementById("placeholder").innerHTML = "Invalid parameters"
    } else {
        state = {
            ...state,
            room,
            name,
            proc,
            duration,
            uid,
            audioDeviceId,
            videoDeviceId
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
    state.stream.getTracks().forEach((track) => track.stop());
    sendToParent(reason);
}

const startRTC = async () => {
    // RTCPeerConnection
    const pc = new RTCPeerConnection(DEFAULT_PEER_CONFIGURATION);
    // Add local tracks before signaling
    const constraints = { ...DEFAULT_CONSTRAINTS };
    if (state.audioDeviceId) {
        constraints.audio = {
            ...constraints.audio,
            deviceId: { ideal: state.audioDeviceId },
        };
    }
    if (state.videoDeviceId) {
        constraints.video = {
            ...constraints.video,
            deviceId: { ideal: state.videoDeviceId },
        };
    }
    const stream = await navigator.mediaDevices.getUserMedia(constraints);
    stream.getTracks().forEach((track) => pc.addTrack(track, stream));
    state.stream = stream;

    // Signaling
    const wsProtocol = window.location.protocol === "https:" ? "wss" : "ws";
    const ws = new WebSocket(`${wsProtocol}://${window.location.host}/ws`);

    ws.onopen = function () {
        const { room, name, proc: rawProc, duration, uid } = state;
        // "0" -> false
        const proc = Boolean(parseInt(rawProc));
        ws.send(
            JSON.stringify({
                type: "join",
                payload: JSON.stringify({ room, name, duration, uid, proc }),
            })
        );
    };

    ws.onclose = function (evt) {
        console.log("[ws] closed");
        stop("disconnected");
    };

    ws.onerror = function (evt) {
        console.error("[ws] error: " + evt.data);
        stop("error");
    };

    ws.onmessage = async function (evt) {
        let msg = JSON.parse(evt.data);
        if (!msg) return console.error("[ws] can't parse message");

        if (msg.type === "offer") {
            const offer = JSON.parse(msg.payload);
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
                    type: "answer",
                    payload: JSON.stringify(answer),
                })
            );
        } else if (msg.type === "candidate") {
            const candidate = JSON.parse(msg.payload);
            if (!candidate) {
                return console.error("[ws] can't parse candidate");
            }
            console.log("[ws] candidate");
            pc.addIceCandidate(candidate);
        } else if (msg.type === "start") {
            console.log("[ws] start");
        } else if (msg.type === "finishing") {
            console.log("[ws] finishing");
            document.getElementById("finishing").classList.remove("d-none");
        } else if (msg.type.startsWith("error") || msg.type === "finish") {
            stop(msg.type);
        }
    };

    pc.onicecandidate = (e) => {
        if (!e.candidate) return;
        ws.send(
            JSON.stringify({
                type: "candidate",
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