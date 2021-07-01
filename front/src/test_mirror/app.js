const state = {};

const DEFAULT_VIDEO_CODECS = ["vp8", "h264"];

// "1" -> true
const toBool = (v) => Boolean(parseInt(v));

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

const randomId = () => Math.random().toString(36).replace(/[^a-z]+/g, '').substr(0, 8);

const init = () => {
    let videoCodecs = DEFAULT_VIDEO_CODECS;
    // dropwdown
    const codecSelect = document.getElementById("codec-select");
    videoCodecs.forEach(vc => {
      const li = document.createElement("li");
      const a = document.createElement("a");
      a.classList.add("dropdown-item");
      a.href = "#";
      a.text = vc;
      a.addEventListener("click", () => {
        state.videoCodec = vc;
        document.getElementById("codec-label").textContent = state.videoCodec;
      });
      li.appendChild(a);
      codecSelect.appendChild(li);
    });
    // default
    state.videoCodec = DEFAULT_VIDEO_CODECS[0];
    document.getElementById("codec-label").textContent = state.videoCodec;
}

const getIntegerValue = (id, defaultValue) => {
    const parsed = parseInt(document.getElementById(id).value, 10);
    return isNaN(parsed) ? defaultValue : parsed;
}

const start = async () => {
    // required
    const room = randomId();
    const uid = randomId();
    const name = uid;
    const duration = getIntegerValue("duration", 20);

    // optional
    const audioFx = document.getElementById("audioFx").value;
    const videoFx = document.getElementById("videoFx").value;
    const width = getIntegerValue("width", 800);
    const height = getIntegerValue("height", 600);
    const frameRate = getIntegerValue("frameRate", 30);
    const video = {
        ...(width && { width: { ideal: width } }),
        ...(height && { height: { ideal: height } }),
        ...(frameRate && { frameRate: { ideal: frameRate } }),
    }
    
    const wsProtocol = window.location.protocol === "https:" ? "wss" : "ws";

    const peerOptions = {
        signalingUrl: `${wsProtocol}://${window.location.host}/ws`,
        room,
        uid,
        name,
        duration,
        // optional
        namespace: "mirror",
        size: 1, // size 1 for mirroring
        video,
        width,
        height,
        ...(audioFx && { audioFx }),
        ...(videoFx && { videoFx }),
        ...(frameRate && { frameRate }),
        ...(state.videoCodec && { videoCodec: state.videoCodec }),
    };
    state.uid = uid;

    const mountEl = document.getElementById("ducksoup-container");
    mountEl.style.width = width + "px";
    mountEl.style.height = height + "px";
    mountEl.classList.remove("d-none");
    // hide
    document.getElementById("start").classList.add("d-none");
    document.getElementById("stopped").classList.add("d-none");
    document.getElementById("stop").classList.remove("d-none");
    // stop if previous instance exists
    if(state.ducksoup) state.ducksoup.stop()
    // start new DuckSoup
    state.ducksoup = await DuckSoup.render(mountEl, peerOptions, {
        callback: receiveMessage,
        debug: true,
    });
};

document.addEventListener("DOMContentLoaded", () => {
    init();
    document.getElementById("start").addEventListener("click", start);
    document
      .getElementById("stop")
      .addEventListener("click", () => location.reload());
  });
  

const hideDuckSoup = () => {
    document.getElementById("stopped").classList.remove("d-none");
    document.getElementById("ducksoup-container").classList.add("d-none");
}

const replaceMessage = (message) => {
    document.getElementById("stopped-message").innerHTML = message;
    hideDuckSoup();
}

const appendMessage = (message) => {
    document.getElementById("stopped-message").innerHTML += '<br/>' + message;
    hideDuckSoup();
}

// communication with iframe
const receiveMessage = (message) => {
    const { kind, payload } = message;
    if(kind !== "stats") {
        console.log("[DuckSoup]", kind);
    }
    if (kind === "end") {
        if(payload && payload[state.uid]) {
            let html = "Conversation terminée, les fichiers suivant ont été enregistrés :<br/><br/>";
            html += payload[state.uid].join("<br/>");
            replaceMessage(html);
            document.getElementById("start").classList.remove("d-none");
            document.getElementById("stop").classList.add("d-none");
        } else {
            replaceMessage("Conversation terminée");
        }
    } else if (kind === "error-full") {
        replaceMessage("Connexion refusée (salle complète)");
    } else if (kind === "error-duplicate") {
        replaceMessage("Connexion refusée (déjà connecté-e)");
    } else if (kind === "disconnection") {
        appendMessage("Connexion perdue");
    } else if (kind === "error") {
        replaceMessage("Erreur");
    } else if (kind === "stats") {
        document.getElementById("audio-up").textContent = payload.audioUp;
        document.getElementById("audio-down").textContent = payload.audioDown;
        document.getElementById("video-up").textContent = payload.videoUp;
        document.getElementById("video-down").textContent = payload.videoDown;
    }
};