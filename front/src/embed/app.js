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

const init = async () => {
    const room = getQueryVariable("room");
    const uid = getQueryVariable("uid");
    const name = getQueryVariable("name");
    const proc = getQueryVariable("proc", toBool);
    const h264 = getQueryVariable("h264", toBool);
    const duration = getQueryVariable("duration", (v) => parseInt(v, 10));
    if (typeof room === 'undefined' || typeof uid === 'undefined' || typeof name === 'undefined' || typeof proc === 'undefined' || isNaN(duration)) {
        document.getElementById("error").classList.remove("d-none");
        document.getElementById("embed").classList.add("d-none");
    } else {
        const params = { room, uid, name, proc, duration, h264 };
        document.getElementById("embed").src = `/1on1/?params=${marshallParams(params)}`;
    }
};

document.addEventListener("DOMContentLoaded", init);

const displayStop = (message) => {
    document.getElementById("stopped-message").innerHTML = message;
    document.getElementById("stopped").classList.remove("d-none");
    document.getElementById("embed").classList.add("d-none");
}

// communication with iframe
window.addEventListener("message", (event) => {
    if (event.origin !== window.location.origin) {
        return;
    } else if (event.data.type === "finish") {
        let html = "Conversation terminée, les fichiers suivant ont été enregistrés:<br/><br/>";
        html += event.data.payload.replaceAll(";", "<br/>")
        displayStop(html);
    } else if (event.data.type === "error-full") {
        displayStop("Connexion refusée (salle complète)");
    } else if (event.data.type === "error-duplicate") {
        displayStop("Connexion refusée (déjà connecté-e)");
    } else if (event.data.type === "disconnected") {
        displayStop("Connexion perdue");
    } else if (event.data.type === "error") {
        displayStop("Erreur");
    }
});