let state;

const randomId = () => Math.random().toString(36).replace(/[^a-z]+/g, '').substr(0, 8);

const processMozza = (videoFx) => {
    if(!videoFx.startsWith("mozza")) return videoFx;
    // already using file paths
    if(videoFx.includes("/")) return videoFx;
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
    isMirror: im,
    userId: uid,
    roomId: rid,
    size: s,
    videoCodec,
    width: w,
    height: h,
    frameRate: fr,
    duration: d,
    audioFx: afx,
    videoFx: vfx,
    audioDevice: ad
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
    // initialize state
    state = { userId, width, height, isMirror, peerCount: 0};
    // add name if fx is not empty
    let audioFx = afx;
    let videoFx = vfx;
    // add name if not empty and not "passthrough" nor "forward" which are special cases
    if(!!afx && afx.length > 0 && !["passthrough", "forward"].includes(afx)) audioFx += " name=fx";
    if(!!vfx && vfx.length > 0 && !["passthrough", "forward"].includes(vfx)) videoFx += " name=fx";
    videoFx = processMozza(videoFx);
    // signaling
    const wsProtocol = window.location.protocol === "https:" ? "wss" : "ws";

    // optional
    const video = {
        ...(width && { width: { ideal: width } }),
        ...(height && { height: { ideal: height } }),
        ...(frameRate && { frameRate: { ideal: frameRate } }),
    }
    const audio = {
        ...(ad && { deviceId: { ideal: ad }}),
    }
    
    // full peerOptions
    const peerOptions = {
        signalingUrl: `${wsProtocol}://${window.location.host}/ws`,
        roomId,
        userId,
        duration,
        // optional
        videoCodec,
        namespace,
        size,
        video,
        audio,
        width,
        height,
        frameRate,
        audioFx,
        videoFx
    };
    console.log("peerOptions: ", peerOptions);

    // UX
    hide(".show-when-not-running");
    hide(".show-when-ended");
    show(".show-when-running");
    if(isMirror) {
        // save space for remote video before local video
        const mountEl = document.getElementById("ducksoup-root");
        mountEl.style.width = state.width + "px";
        mountEl.style.height = state.height + "px";
    }
    // stop if previous instance exists
    if(state.ducksoup) state.ducksoup.stop()
    // start new DuckSoup
    state.ducksoup = await DuckSoup.render({
        callback: ducksoupListener,
        debug: true,
    }, peerOptions);
    // display direct stream in mirror mode
    if(isMirror) {
        // insert local video
        document.getElementById("local-video").srcObject = state.ducksoup.stream;
    }
};

document.addEventListener("DOMContentLoaded", async() => {
    const formSettings = document.getElementById("settings");

    // UX
    show(".show-when-not-running");
    hide(".show-when-ended");
    hide(".show-when-running");

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
        if(state.ducksoup) state.ducksoup.stop();
      });

    document.getElementById("audio-control").addEventListener("click", () => {
        if(state.ducksoup) {
            const property = document.getElementById("audio-property").value;
            const value = parseFloat(document.getElementById("audio-value").value);
            const duration = parseInt(document.getElementById("audio-duration").value, 10);
            state.ducksoup.audioControl("fx", property, value, duration);
        }
    });

    document.getElementById("video-control").addEventListener("click", () => {
        if(state.ducksoup) {
            const property = document.getElementById("video-property").value;
            const value = parseFloat(document.getElementById("video-value").value);
            const duration = parseInt(document.getElementById("video-duration").value, 10);
            state.ducksoup.videoControl("fx", property, value,duration);
        }
    });

    // audio input selection
    const devices = await navigator.mediaDevices.enumerateDevices();
    const audioInput = document.getElementById("input-audio");
    for (let i = 0; i !== devices.length; ++i) {
        const device = devices[i];
        if (device.kind === "audioinput") {
            const option = document.createElement("option");
            option.text =  device.label || `microphone ${audioInputSelect.length + 1}`;
            option.value = device.deviceId,
            audioInput.appendChild(option);
        }
    }
});

const clearMessage = () => {
    document.getElementById("stopped-message").innerHTML = "";
};

const replaceMessage = (message) => {
    document.getElementById("stopped-message").innerHTML = message + '<br/>' ;
};

const appendMessage = (message) => {
    document.getElementById("stopped-message").innerHTML += message + '<br/>' ;
};

// communication with iframe
const ducksoupListener = (message) => {
    const { kind, payload } = message;
    const mountEl = document.getElementById("ducksoup-root");

    // grouped cases
    if(kind !== "stats") {
        console.log("[DuckSoup]", kind);
    }
    if(kind.startsWith("error") || kind === "end" || kind === "disconnection") {
        show(".show-when-not-running");
        show(".show-when-ended");
        hide(".show-when-running");
        hide(".show-when-ending");
    }
    
    // specific cases
    if (kind === "start") {
        clearMessage();
    } else if (kind === "track") {
        mountEl.classList.remove("d-none");
        const { track, streams } = payload;
        if(track.kind === "video") {
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
            if(state.isMirror) {
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
    } else if (kind === "end") {
        // replace mountEl contents
        while (mountEl.firstChild) {
            mountEl.removeChild(mountEl.firstChild);
        }
        if(payload && payload[state.userId]) {
            let html = "The following files have been recorded:<br/><br/>";
            html += payload[state.userId].join("<br/>") + "<br/>";
            replaceMessage(html);
        } else {
            replaceMessage("Connection terminated");
        }
    } else if (kind === "error-duplicate") {
        replaceMessage("Connection denied (already connected)");
    } else if (kind === "disconnection") {
        appendMessage("Disconnected");
    } else if (kind === "error") {
        replaceMessage("Error");
    } else if (kind === "stats") {
        document.getElementById("audio-up").textContent = payload.audioUp;
        document.getElementById("audio-down").textContent = payload.audioDown;
        document.getElementById("video-up").textContent = payload.videoUp;
        document.getElementById("video-down").textContent = payload.videoDown;
    }
};