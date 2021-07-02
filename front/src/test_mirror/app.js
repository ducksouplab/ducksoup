const state = {};

const randomId = () => Math.random().toString(36).replace(/[^a-z]+/g, '').substr(0, 8);

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
    const room = randomId();
    const uid = randomId();
    const name = uid;
    state.uid = uid;
    // parse
    const width = parseInt(w, 10);
    const height = parseInt(h, 10);
    const frameRate = parseInt(fr, 10);
    const duration = parseInt(d, 10);
    // add name if fx is not empty
    const audioFx = !!afx && afx.length > 0 ? afx + " name=fx" : afx;
    const videoFx = !!vfx && vfx.length > 0 ? vfx + " name=fx" : vfx;

    // optional

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
    mountEl.classList.remove("d-none");
    // hide
    document.getElementById("start").classList.add("d-none");
    document.getElementById("stopped").classList.add("d-none");
    document.getElementById("stop").classList.remove("d-none");
    document.getElementById("live-control").classList.remove("d-none");
    // stop if previous instance exists
    if(state.ducksoup) state.ducksoup.stop()
    // start new DuckSoup
    state.ducksoup = await DuckSoup.render(mountEl, peerOptions, {
        callback: receiveMessage,
        debug: true,
    });
};

document.addEventListener("DOMContentLoaded", () => {
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
      .addEventListener("click", () => location.reload());

    document.getElementById("audio-control").addEventListener("click", () => {
        if(state.ducksoup) {
            const property = document.getElementById("audio-property").value;
            const value = parseFloat(document.getElementById("audio-value").value);
            state.ducksoup.audioControl("fx", property, value);
        }
    });

    document.getElementById("video-control").addEventListener("click", () => {
        if(state.ducksoup) {
            const property = document.getElementById("video-property");
            const value = parseFloat(document.getElementById("video-value").value);
            state.ducksoup.videoControl("fx", property, value);
        }
    });
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
            document.getElementById("live-controle").classList.add("d-none");
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