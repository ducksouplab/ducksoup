document.addEventListener("DOMContentLoaded", () => {
    console.log("[DuckSoup test] v1.5.4")
});

let state;

const randomId = () => Math.random().toString(36).replace(/[^a-z]+/g, '').substring(0, 8);

const processMozza = (videoFx) => {
    if (!videoFx.startsWith("mozza")) return videoFx;
    // already using file paths
    if (videoFx.includes("/")) return videoFx;
    let output = videoFx.replace(/deform=([^\s]+)/, "deform=plugins/$1.dfm")
    output = output.replace(/shape-model=([^\s]+)/, "shape-model=plugins/$1.dat")
    return output;
};

const hide = (selector) => {
    const targets = document.querySelectorAll(selector);

    for (let i = 0; i < targets.length; i++) {
        targets[i].classList.add("d-none");
    }
};

const show = (selector) => {
    const targets = document.querySelectorAll(selector);

    for (let i = 0; i < targets.length; i++) {
        targets[i].classList.remove("d-none");
    }
};

const parseIntWithFallback = (raw, fallback) => {
    const parsed = parseInt(raw, 10);
    return isNaN(parsed) ? fallback : parsed;
};

const start = async ({
    signalingUrl,
    isMirror: im,
    userId: uid,
    roomId: rid,
    size: s,
    videoFormat,
    recordingMode,
    width: w,
    height: h,
    frameRate: fr,
    duration: d,
    audioFx: afx,
    videoFx: vfx,
    audioDevice: ad,
    videoDevice: vd,
    gpu: g
}) => {
    const isMirror = !!im;
    // required
    const roomId = isMirror ? randomId() : rid;
    const userId = isMirror ? randomId() : uid;
    const size = isMirror ? 1 : parseInt(s, 10);
    const namespace = isMirror ? "mirror" : "room";
    // parse
    const width = parseIntWithFallback(w, 800);
    const height = parseIntWithFallback(h, 600);
    const frameRate = parseIntWithFallback(fr, 30);
    const duration = parseIntWithFallback(d, 30);
    const gpu = !!g;
    // initialize state
    state = { userId, width, height, isMirror, peerCount: 0 };
    // add name if fx is not empty
    let audioFx = afx;
    let videoFx = vfx;
    // add name if not empty
    if (!!afx && afx.length > 0) audioFx += " name=audio_fx";
    if (!!vfx && vfx.length > 0) videoFx += " name=video_fx";
    videoFx = processMozza(videoFx);

    // optional
    const video = {
        ...(width && { width: { ideal: width } }),
        ...(height && { height: { ideal: height } }),
        ...(frameRate && { frameRate: { ideal: frameRate } }),
        ...(vd && { deviceId: { ideal: vd } }),
    }
    const audio = {
        ...(ad && { deviceId: { ideal: ad } }),
    }

    // full peerOptions
    const peerOptions = {
        signalingUrl,
        roomId,
        userId,
        duration,
        // optional
        videoFormat,
        recordingMode,
        namespace,
        size,
        video,
        audio,
        width,
        height,
        frameRate,
        audioFx,
        videoFx,
        gpu
    };

    // UX
    hide(".show-when-not-running");
    hide(".show-when-ended");
    show(".show-when-running");
    if (isMirror) {
        // save space for remote video before local video
        const mountEl = document.getElementById("ducksoup-root");
        mountEl.style.width = state.width + "px";
        mountEl.style.height = state.height + "px";
    }
    // stop if previous instance exists
    if (state.ducksoup) state.ducksoup.stop()
    // start new DuckSoup
    const options = { isMirror };
    state.ducksoup = await DuckSoup.render({
        callback: ducksoupListener(options),
        debug: true,
        stats: true,
    }, peerOptions);
};

document.addEventListener("DOMContentLoaded", async () => {
    reinitUX();

    // Init signalingURL with default value
    const wsProtocol = window.location.protocol === "https:" ? "wss" : "ws";
    const pathPrefixhMatch = /(.*)test/.exec(window.location.pathname);
    // depending on DS_WEB_PREFIX, signaling endpoint may be located at /ws or /prefix/ws
    const pathPrefix = pathPrefixhMatch[1];
    document.getElementById("input-signaling-url").value = `${wsProtocol}://${window.location.host}${pathPrefix}ws`;

    const formSettings = document.getElementById("settings");
    formSettings.addEventListener("submit", (e) => {
        e.preventDefault();
        const settings = {};
        const formData = new FormData(formSettings);
        for (var key of formData.keys()) {
            settings[key] = formData.get(key);
        }
        start(settings);
    });

    document
        .getElementById("stop")
        .addEventListener("click", () => {
            if (state.ducksoup) state.ducksoup.stop();
            clearMount();
            show(".show-when-not-running");
            hide(".show-when-running");
        });

    const fxForms = document.querySelectorAll("form.fx");

    for (let i = 0; i < fxForms.length; i++) {
        fxForms[i].addEventListener("submit", (e) => {
            e.preventDefault();
            if (state.ducksoup) {
                const type = e.target.querySelector("[name='type']").value;
                const property = e.target.querySelector("[name='property']").value;
                const value = parseFloat(e.target.querySelector("[name='value']").value);
                const duration = parseInt(e.target.querySelector("[name='duration']").value, 10);
                state.ducksoup.controlFx(type + "_fx", property, value, duration);
            }
        });
    }

    // audio input selection
    const devices = await navigator.mediaDevices.enumerateDevices();
    const audioInput = document.getElementById("input-audio");
    const videoInput = document.getElementById("input-video");
    for (let i = 0; i !== devices.length; ++i) {
        const device = devices[i];
        if (device.kind === "audioinput") {
            const option = document.createElement("option");
            option.text = device.label || `microphone ${audioInput.length + 1}`;
            option.value = device.deviceId,
                audioInput.appendChild(option);
        } else if (device.kind === "videoinput") {
            const option = document.createElement("option");
            option.text = device.label || `camera ${videoInput.length + 1}`;
            option.value = device.deviceId,
                videoInput.appendChild(option);
        }
    }
});

const clearMount = () => {
    const mountEl = document.getElementById("ducksoup-root");
    while (mountEl.firstChild) {
        mountEl.removeChild(mountEl.firstChild);
    }
};

const reinitUX = () => {
    // replace mountEl contents
    clearMount();
    // update UX
    show(".show-when-not-running");
    show(".show-when-ended");
    hide(".show-when-running");
    hide(".show-when-ending");
}

const clearMessage = () => {
    document.getElementById("stopped-message").innerHTML = "";
};

const replaceMessage = (message) => {
    document.getElementById("stopped-message").innerHTML = message + '<br/>';
};

const appendMessage = (message) => {
    document.getElementById("stopped-message").innerHTML += message + '<br/>';
};

// communication with iframe
const ducksoupListener = (options) => (message) => {
    const { kind, payload } = message;
    const mountEl = document.getElementById("ducksoup-root");

    // grouped cases
    if (kind !== "stats") {
        console.log("[App]", kind);
    }
    if (kind.startsWith("error") || kind === "closed") {
        reinitUX();
    }

    // specific cases
    if (kind === "start") {
        clearMessage();
    } else if (kind === "local-stream") {
        if (options.isMirror) { // display direct stream in mirror mode
            // insert local video
            document.getElementById("local-video").srcObject = payload;
        }
    } else if (kind === "track") {
        mountEl.classList.remove("d-none");
        const { track, streams } = payload;
        if (track.kind === "video") {
            // append stream
            let container = document.createElement("div");
            container.id = track.id;
            container.classList.add("video-container")
            // create <video>
            let el = document.createElement(track.kind);
            el.srcObject = streams[0];
            el.autoplay = true;
            // size
            container.style.width = state.width + "px";
            container.style.height = state.height + "px";
            el.style.width = state.width + "px";
            el.style.height = state.height + "px";
            // append
            container.appendChild(el);
            container.insertAdjacentHTML("beforeend", "<div class='overlay overlay-bottom show-when-ending'><div>Conversation soon ending</div></div>");
            if (state.isMirror) {
                container.insertAdjacentHTML("beforeend", "<div class='overlay overlay-top show-when-running'><div>Through server</div></div>");
            } else {
                ++state.peerCount;
                container.insertAdjacentHTML("beforeend", `<div class='overlay overlay-top show-when-running'><div>Peer #${state.peerCount}</div></div>`);
            }
            mountEl.appendChild(container);;
            hide(".show-when-ending");
        } else {
            let el = document.createElement(track.kind);
            el.id = track.id;
            el.srcObject = streams[0];
            el.autoplay = true;
            mountEl.appendChild(el);
        }
        // on remove
        streams[0].onremovetrack = ({ track }) => {
            const el = document.getElementById(track.id);
            if (el) el.parentNode.removeChild(el);
        };
    } else if (kind === "ending") {
        show(".show-when-ending");
    } else if (kind === "files") {
        if (payload && payload[state.userId]) {
            let html = "The following files have been recorded:<br/><br/>";
            html += payload[state.userId].join("<br/>") + "<br/>";
            replaceMessage(html);
        } else {
            replaceMessage("Connection terminated");
        }
    } else if (kind === "error-duplicate") {
        replaceMessage("Connection denied (already connected)");
    } else if (kind === "error-disconnection") {
        appendMessage("Error: disconnected");
    } else if (kind === "error") {
        replaceMessage("Error");
    } else if (kind === "stats") {
        document.getElementById("audio-out-bitrate").textContent = payload.audioUp;
        document.getElementById("audio-in-bitrate").textContent = payload.audioDown;
        document.getElementById("video-out-bitrate").textContent = payload.videoUp;
        document.getElementById("video-in-bitrate").textContent = payload.videoDown;
        const { outboundRTPVideo, inboundRTPVideo, outboundRTPAudio, inboundRTPAudio, remoteOutboundRTPVideo, remoteInboundRTPVideo, remoteOutboundRTPAudio, remoteInboundRTPAudio } = payload;
        if (outboundRTPVideo) {
            // add processed props
            outboundRTPVideo.averageEncodeTime = Number(outboundRTPVideo.totalEncodeTime / outboundRTPVideo.framesEncoded).toFixed(3);
            // select displayed props
            const props = ["frameWidth", "frameHeight", "framesPerSecond", "qualityLimitationReason", "keyFramesEncoded", "firCount", "pliCount", "sliCount", "nackCount", "framesDiscardedOnSend", "averageEncodeTime", "packetsSent"];
            // render
            for (let p of props) {
                document.getElementById(`video-out-${p}`).textContent = outboundRTPVideo[p];
            }
        }
        if (inboundRTPVideo) {
            // add processed props
            inboundRTPVideo.processedJitter = Number(inboundRTPVideo.jitterBufferDelay / inboundRTPVideo.jitterBufferEmittedCount).toFixed(3);
            // select displayed props
            const props = ["frameWidth", "frameHeight", "framesPerSecond", "keyFramesDecoded", "pliCount", "firCount", "sliCount", "nackCount", "processedJitter", "jitter", "packetsReceived", "packetsLost", "packetsDiscarded", "packetsRepaired", "framesDropped"];
            // render
            for (let p of props) {
                document.getElementById(`video-in-${p}`).textContent = inboundRTPVideo[p];
            }
        }
        if (outboundRTPAudio) {
            // select displayed props
            const props = ["nackCount", "targetBitrate", "packetsSent"];
            // render
            for (let p of props) {
                document.getElementById(`audio-out-${p}`).textContent = outboundRTPAudio[p];
            }
        }
        if (inboundRTPAudio) {
            // add processed props
            inboundRTPAudio.processedJitter = Number(inboundRTPAudio.jitterBufferDelay / inboundRTPAudio.jitterBufferEmittedCount).toFixed(3);
            if (inboundRTPAudio.totalSamplesDuration) {
                inboundRTPAudio.totalSamplesDuration = inboundRTPAudio.totalSamplesDuration.toFixed(2);
            }
            // select displayed props
            const props = ["processedJitter", "nackCount", "concealedSamples", "totalSamplesDuration", "jitter", "packetsReceived", "packetsLost", "packetsDiscarded", "packetsRepaired"];
            // render
            for (let p of props) {
                document.getElementById(`audio-in-${p}`).textContent = inboundRTPAudio[p];
            }
        }
        if (remoteOutboundRTPAudio) {
            // select displayed props
            const props = ["packetsSent", "fractionLost", "packetsLost", "roundTripTime", "roundTripTimeMeasurements", "totalRoundTripTime"];
            // render
            for (let p of props) {
                document.getElementById(`remote-audio-out-${p}`).textContent = remoteOutboundRTPAudio[p];
            }
        }
        if (remoteOutboundRTPVideo) {
            // select displayed props
            const props = ["packetsSent", "fractionLost", "packetsLost", "roundTripTime", "roundTripTimeMeasurements", "totalRoundTripTime"];
            // render
            for (let p of props) {
                document.getElementById(`remote-video-out-${p}`).textContent = remoteOutboundRTPVideo[p];
            }
        }
        if (remoteInboundRTPAudio) {
            if (remoteInboundRTPAudio.jitter) {
                remoteInboundRTPAudio.jitter = remoteInboundRTPAudio.jitter.toFixed(3);
            }
            // select displayed props
            const props = ["jitter", "fractionLost", "packetsLost", "roundTripTime", "roundTripTimeMeasurements", "totalRoundTripTime"];
            // render
            for (let p of props) {
                document.getElementById(`remote-audio-in-${p}`).textContent = remoteInboundRTPAudio[p];
            }
        }
        if (remoteInboundRTPVideo) {
            if (remoteInboundRTPVideo.jitter) {
                remoteInboundRTPVideo.jitter = remoteInboundRTPVideo.jitter.toFixed(3);
            }
            // select displayed props
            const props = ["jitter", "fractionLost", "packetsLost", "roundTripTime", "roundTripTimeMeasurements", "totalRoundTripTime"];
            // render
            for (let p of props) {
                document.getElementById(`remote-video-in-${p}`).textContent = remoteInboundRTPVideo[p];
            }
        }
    }
};