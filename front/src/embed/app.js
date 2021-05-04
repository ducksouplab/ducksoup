const randomId = () => Math.random().toString(36).replace(/[^a-z]+/g, '').substr(0, 8);

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
    const name = getQueryVariable("name");
    const proc = getQueryVariable("proc");
    const duration = parseInt(getQueryVariable("duration"), 10);
    if (typeof room === 'undefined' || typeof name === 'undefined' || !["0", "1"].includes(proc) || isNaN(duration)) {
        document.getElementById("error").classList.remove("d-none");
    } else {
        document.getElementById("embed").src = `/1on1/?room=${room}&name=${name}&proc=${proc}&duration=${duration}&uid=${randomId()}`;
    }
};

document.addEventListener("DOMContentLoaded", init);
// communication with iframe
window.addEventListener("message", (event) => {
    if (event.origin !== window.location.origin) {
        return;
    }
    if (event.data === "stop") {
        document.getElementById("stopped").classList.remove("d-none");
        document.getElementById("embed").classList.add("d-none");
    }
});