const state = {};

const randomId = () => Math.random().toString(36).replace(/[^a-z]+/g, '').substr(0, 8);

const processMozza = (videoFx) => {
    if(!videoFx.startsWith("mozza")) return videoFx;
    console.log("blabla");
    // already using file paths
    if(videoFx.includes("/")) return videoFx;
    let output = videoFx.replace(/deform=([^\s]+)/, "deform=plugins/$1.dfm")
    output = output.replace(/shape-model=([^\s]+)/, "shape-model=plugins/$1.dat")
    return output;
}

// mozza deform=plugins/gui1.dfm alpha=1 overlay=true
// mozza deform=plugins/gui1.dfm alpha=1 overlay=true

const hide = (selector) => {
    const targets = document.querySelectorAll(selector);

    for (let i = 0; i < targets.length; i++) {
        targets[i].classList.add("d-none");
    }
}

const show = (selector) => {
    const targets = document.querySelectorAll(selector);

    for (let i = 0; i < targets.length; i++) {
        targets[i].classList.remove("d-none");
    }
}

const start = async ({
    videoCodec,
    width: w,
    height: h,
    frameRate: fr,
    duration: d,
    audioFx: afx,
    videoFx: vfx
}) => {
    // required
    const roomId = randomId();
    const userId = randomId();
    state.userId = userId;
    // parse
    const width = parseInt(w, 10);
    const height = parseInt(h, 10);
    const frameRate = parseInt(fr, 10);
    const duration = parseInt(d, 10);
    // add name if fx is not empty
    let audioFx = afx;
    let videoFx = vfx;
    // add name if not empty and not "passthrough" nor "forward" which are special cases
    if(!!afx && afx.length > 0 && !["passthrough", "forward"].includes(afx)) audioFx += " name=fx";
    if(!!vfx && vfx.length > 0 && !["passthrough", "forward"].includes(vfx)) videoFx += " name=fx";
    videoFx = processMozza(videoFx);
    console.log(videoFx);

    // optional

    const video = {
        ...(width && { width: { ideal: width } }),
        ...(height && { height: { ideal: height } }),
        ...(frameRate && { frameRate: { ideal: frameRate } }),
    }
    
    const wsProtocol = window.location.protocol === "https:" ? "wss" : "ws";

    const peerOptions = {
        signalingUrl: `${wsProtocol}://${window.location.host}/ws`,
        roomId,
        userId,
        duration,
        // optional
        videoCodec,
        namespace: "mirror",
        size: 1, // size 1 for mirroring
        video,
        width,
        height,
        frameRate,
        audioFx,
        videoFx
    };
    console.log("peerOptions: ", peerOptions);

    const mountEl = document.getElementById("ducksoup-container");
    mountEl.style.width = width + "px";
    mountEl.style.height = height + "px";
    // UX
    mountEl.classList.remove("d-none");
    hide(".show-when-not-running");
    hide(".show-when-ended");
    show(".show-when-running");
    // stop if previous instance exists
    if(state.ducksoup) state.ducksoup.stop()
    // start new DuckSoup
    state.ducksoup = await DuckSoup.render(mountEl, peerOptions, {
        callback: receiveMessage,
        debug: true,
    });
    document.getElementById("local-video").srcObject = state.ducksoup.stream;
};

document.addEventListener("DOMContentLoaded", () => {
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
  });

const clearMessage = () => {
    document.getElementById("stopped-message").innerHTML = "";
}

const replaceMessage = (message) => {
    document.getElementById("stopped-message").innerHTML = message + '<br/>' ;
}

const appendMessage = (message) => {
    document.getElementById("stopped-message").innerHTML += message + '<br/>' ;
}

// communication with iframe
const receiveMessage = (message) => {
    const { kind, payload } = message;
    if(kind !== "stats") {
        console.log("[DuckSoup]", kind);
    }
    if(kind.startsWith("error") || kind === "end" || kind === "disconnection") {
        show(".show-when-not-running");
        show(".show-when-ended");
        hide(".show-when-running");
    }
    if (kind === "end") {
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
        appendMessage("Connection lost");
    } else if (kind === "error") {
        replaceMessage("Error");
    } else if (kind === "start") {
        clearMessage();
    } else if (kind === "stats") {
        document.getElementById("audio-up").textContent = payload.audioUp;
        document.getElementById("audio-down").textContent = payload.audioDown;
        document.getElementById("video-up").textContent = payload.videoUp;
        document.getElementById("video-down").textContent = payload.videoDown;
    }
};