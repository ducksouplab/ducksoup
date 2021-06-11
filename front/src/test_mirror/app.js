const state = {};

const DEFAULT_VIDEO_CODECS = ["h264", "vp8"];

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

const marshallParams = (obj) => encodeURI(btoa(JSON.stringify(obj)));

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
    const proc = false;
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
    
    const params = {
        origin: window.location.origin,
        room,
        uid,
        name,
        proc,
        duration,
        // optional
        size: 1, // size 1 for mirroring
        width,
        height,
        video,
        ...(audioFx && { audioFx }),
        ...(videoFx && { videoFx }),
        ...(state.videoCodec && { videoCodec: state.videoCodec }),
    };
    state.uid = uid;

    document.getElementById("embed").width = width;
    document.getElementById("embed").height = height;
    document.getElementById("embed").src = `/embed/?params=${marshallParams(params)}`;
    // hide
    document.getElementById("start").classList.add("d-none");
    document.getElementById("stopped").classList.add("d-none");
    document.getElementById("stop").classList.remove("d-none");
    // show
    document.getElementById("embed").classList.remove("d-none");

};

document.addEventListener("DOMContentLoaded", () => {
    init();
    document.getElementById("start").addEventListener("click", start);
    document
      .getElementById("stop")
      .addEventListener("click", () => location.reload());
  });
  

const hideEmbed = () => {
    document.getElementById("stopped").classList.remove("d-none");
    document.getElementById("embed").classList.add("d-none");
}

const replaceMessage = (message) => {
    document.getElementById("stopped-message").innerHTML = message;
    hideEmbed();
}

const appendMessage = (message) => {
    document.getElementById("stopped-message").innerHTML += '<br/>' + message;
    hideEmbed();
}

// communication with iframe
window.addEventListener("message", (event) => {
    if (event.origin !== window.location.origin) return;

    const { kind, payload } = event.data;
    if (kind === "finish") {
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
    } else if (kind === "disconnected") {
        appendMessage("Connexion perdue");
    } else if (kind === "error") {
        replaceMessage("Erreur");
    }

});