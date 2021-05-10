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

const init = async () => {
    const room = getQueryVariable("room");
    const uid = getQueryVariable("uid");
    const name = getQueryVariable("name");
    const proc = getQueryVariable("proc");
    const duration = parseInt(getQueryVariable("duration"), 10);
    if (typeof room === 'undefined' || typeof uid === 'undefined' || typeof name === 'undefined' || !["0", "1"].includes(proc) || isNaN(duration)) {
        document.getElementById("error").classList.remove("d-none");
        document.getElementById("embed").classList.add("d-none");
    } else {
        document.getElementById("embed").src = `/1on1/?room=${room}&uid=${uid}&name=${name}&proc=${proc}&duration=${duration}`;
    }
};

document.addEventListener("DOMContentLoaded", init);

const displayStop = (message) => {
    document.getElementById("stopped-message").innerText = message;
    document.getElementById("stopped").classList.remove("d-none");
    document.getElementById("embed").classList.add("d-none");
}

// communication with iframe
window.addEventListener("message", (event) => {
    if (event.origin !== window.location.origin) {
        return;
    } else if (event.data === "finish") {
        displayStop("Conversation terminée");
    } else if (event.data === "error-full") {
        displayStop("Connexion refusée (salle complète)");
    } else if (event.data === "error-duplicate") {
        displayStop("Connexion refusée (déjà connecté-e)");
    } else if (event.data === "error") {
        displayStop("Erreur");
    }
});