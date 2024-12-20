let state;

const randomId = () =>
  Math.random()
    .toString(36)
    .replace(/[^a-z]+/g, "")
    .substring(0, 8);

const processMozza = (videoFx) => {
  if (!videoFx.startsWith("mozza")) return videoFx;
  // already using file paths
  if (videoFx.includes("/")) return videoFx;
  let output = videoFx.replace(/deform=([^\s]+)/, "deform=plugins/$1.dfm");
  output = output.replace(/shape-model=([^\s]+)/, "shape-model=plugins/$1.dat");
  return output;
};

const processMozzaDeform = (property, value) => {
  if (property !== "deform") return value;
  if (value.includes("/")) return value;
  return `plugins/${value}.dfm`;
}

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

const parseControlFxSequence = (properties, values, durations) => {
  let sequence = properties.map((p, i) => ({ property: p, value: values[i], duration: durations[i] }));
  sequence = sequence.filter(({duration}) => !!duration);
  sequence = sequence.map((fx) => {
    if (fx.property.length == 0 || isNaN(fx.value)) return {...fx, onlyWait: true};
    return fx;
  });
  return sequence;
};

const playControlFxSequence = (type, sequence) => {
  const next = sequence.shift();
  if (typeof next === "undefined") return;

  setTimeout(() => {
    if(!next.onlyWait) {
      state.ducksoup.controlFx(type, next.property, next.value, next.duration);
    }
    playControlFxSequence(type, sequence); // sequence has been shifted
  }, next.duration);
};

const start = async ({
  // not processed
  signalingUrl,
  videoFormat,
  recordingMode,
  audioOnly,
  // processed
  isMirror: im,
  userId: uId,
  interactionName: iName,
  size: s,
  width: w,
  height: h,
  framerate: fr,
  duration: d,
  audioFx: afx,
  videoFx: vfx,
  audioDevice: ad,
  videoDevice: vd,
  gpu: g,
  overlay: o,
}) => {
  const isMirror = !!im;
  // required
  const interactionName = isMirror ? randomId() : iName;
  const userId = isMirror ? randomId() : uId;
  const size = isMirror ? 1 : parseInt(s, 10);
  const namespace = isMirror ? "test_mirror" : "test_interaction";
  // parse
  const width = parseIntWithFallback(w, 800);
  const height = parseIntWithFallback(h, 600);
  const framerate = parseIntWithFallback(fr, 25);
  const duration = parseIntWithFallback(d, 30);
  const gpu = !!g;
  const overlay = !!o;
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
    ...(framerate && { framerate: { ideal: framerate } }),
    ...(vd && { deviceId: { ideal: vd } }),
  };
  const audio = {
    ...(ad && { deviceId: { ideal: ad } }),
  };

  // full peerOptions
  const peerOptions = {
    signalingUrl,
    interactionName,
    userId,
    duration,
    audioOnly,
    // optional
    videoFormat,
    recordingMode,
    namespace,
    size,
    video,
    audio,
    width,
    height,
    framerate,
    audioFx,
    videoFx,
    gpu,
    overlay,
    logLevel: 2,
  };

  // UX
  hide(".show-when-not-running");
  hide(".show-when-ended");
  show(".show-when-running");
  if (isMirror) {
    // save space for remote video before local video
    const wrapperEl = document.getElementById("ducksoup-wrapper");
    const mountEl = document.getElementById("ducksoup-mount");
    wrapperEl.style.width = state.width + "px";
    wrapperEl.style.height = state.height + "px";
    mountEl.style.width = state.width + "px";
    mountEl.style.height = state.height + "px";
  }
  // stop if previous instance exists
  if (state.ducksoup) state.ducksoup.stop();
  // start new DuckSoup
  const options = { isMirror };
  state.ducksoup = await DuckSoup.render(
    {
      callback: ducksoupListener(options),
      stats: true,
    },
    peerOptions
  );
  window.state = state;
};

document.addEventListener("DOMContentLoaded", async () => {
  resetUX();

  // Init signalingURL with default value
  const wsProtocol = window.location.protocol === "https:" ? "wss" : "ws";
  const pathPrefixhMatch = /(.*)test/.exec(window.location.pathname);
  // depending on DUCKSOUP_WEB_PREFIX, signaling endpoint may be located at /ws or /prefix/ws
  const pathPrefix = pathPrefixhMatch[1];
  document.getElementById(
    "input-signaling-url"
  ).value = `${wsProtocol}://${window.location.host}${pathPrefix}ws`;

  const formSettings = document.getElementById("settings");
  formSettings.addEventListener("submit", (e) => {
    e.preventDefault();
    const settings = {};
    const formData = new FormData(formSettings);
    for (var key of formData.keys()) {
      settings[key] = formData.get(key);
    }
    start(settings);
    // additional form filling for /test/interaction/ page
    targetAudioFx = document.getElementById("input-audio-user-id");
    targetVideoFx = document.getElementById("input-video-user-id");
    if(targetAudioFx) targetAudioFx.value = settings.userId;
    if(targetVideoFx) targetVideoFx.value = settings.userId;
  });

  document.getElementById("stop").addEventListener("click", () => {
    if (state.ducksoup) state.ducksoup.stop();
    //clearMount();
    show(".show-when-not-running");
    hide(".show-when-running");
    const started_message = document.getElementById("started_message");
    started_message.classList.add("d-none");

// Remove the 'd-none' class
    ducksoupMount.classList.remove("d-none");
  });


  // /test/mirror/ control sequence fx
  const sequenceFxForms = document.querySelectorAll("form.fx-sequence");

  for (const form of sequenceFxForms) {
    form.addEventListener("submit", (e) => {
      e.preventDefault();
      if (state.ducksoup) {
        const type = e.target.querySelector("[name='type']").value;
        const properties = [...e.target.querySelectorAll("[name='property[]']")].map(el => el.value);
        const values = [...e.target.querySelectorAll("[name='value[]']")].map(el => parseFloat(el.value));
        const durations = [...e.target.querySelectorAll("[name='duration[]']")].map(el => parseInt(el.value, 10));
        const sequence = parseControlFxSequence(properties, values, durations);
        playControlFxSequence(type, sequence);
      }
    });
  }

  // /test/mirror/ control int/float/string fx
  document.querySelector("form.fx-infer-kind")?.addEventListener("submit", (e) => {
    e.preventDefault();
    if (state.ducksoup) {
      const name = e.target.querySelector("[name='name']").value;
      const property = e.target.querySelector("[name='property']").value;
      let value = e.target.querySelector("[name='value']").value;
      value = processMozzaDeform(property, value);

      let kind = "string";
      if (!isNaN(value)) {
        kind = value.toString().indexOf('.') != -1 ? "float" : "int";
      }

      state.ducksoup.polyControlFx(name, property, kind, value);
    }
  });

  // /test/interaction/ control fx
  const fxForms = document.querySelectorAll("form.fx");

  for (let i = 0; i < fxForms.length; i++) {
    fxForms[i].addEventListener("submit", (e) => {
      e.preventDefault();
      if (state.ducksoup) {
        const type = e.target.querySelector("[name='type']").value;
        const property = e.target.querySelector("[name='property']").value;
        const value = parseFloat(
          e.target.querySelector("[name='value']").value
        );
        const duration = parseInt(
          e.target.querySelector("[name='duration']").value,
          10
        );
        const userIdEl = e.target.querySelector("[name='userId']");
        if (userIdEl) {
          state.ducksoup.controlFx(type + "_fx", property, value, duration, userIdEl.value);
        } else {
          state.ducksoup.controlFx(type + "_fx", property, value, duration);
        }
      }
    });
  }

  // audio input selection
  const devices = await navigator.mediaDevices.enumerateDevices();
  const audioInput = document.getElementById("input-audio");
  for (let i = 0; i !== devices.length; ++i) {
    const device = devices[i];
    if (device.kind === "audioinput") {
      const option = document.createElement("option");
      option.text = device.label || `microphone ${audioInput.length + 1}`;
      (option.value = device.deviceId), audioInput.appendChild(option);
    } else if (device.kind === "videoinput") {
      const option = document.createElement("option");
    }
  }
});


const clearMount = () => {
  const mountEl = document.getElementById("ducksoup-mount");
  while (mountEl.firstChild) {
    mountEl.removeChild(mountEl.firstChild);
  }
};

const resetUX = () => {
  // replace mountEl contents -- WHy?
  //clearMount();
  // update UX
  show(".show-when-not-running");
  show(".show-when-ended");
  hide(".show-when-running");
  hide(".show-when-ending");
};

const clearMessage = () => {
  document.getElementById("stopped-message").innerHTML = "";
};

const replaceMessage = (message) => {
  document.getElementById("stopped-message").innerHTML = message + "<br/>";
};

const appendMessage = (message) => {
  document.getElementById("stopped-message").innerHTML += message + "<br/>";
};

// communication with player
const ducksoupListener = (options) => (message) => {
  const { kind, payload } = message;
  const mountEl = document.getElementById("ducksoup-mount");

  // grouped cases
  if (kind !== "stats") {
    console.debug("[Received by test client]", kind);
  }
  if (kind.startsWith("error") || kind === "closed") {
    resetUX();
  }

  // specific cases
  if (kind === "start") {
    clearMessage();
  } else if (kind === "local-stream") {
    if (options.isMirror) {
      // display direct stream in mirror mode
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
      container.classList.add("video-container");
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
      if (state.isMirror) {
        container.insertAdjacentHTML(
          "beforeend",
          "<div class='overlay overlay-top show-when-running'><div>Through server</div></div>"
        );
      } else {
        ++state.peerCount;
        container.insertAdjacentHTML(
          "beforeend",
          `<div class='overlay overlay-top show-when-running'><div>Peer #${state.peerCount}</div></div>`
        );
      }
      mountEl.appendChild(container);
      hide(".show-when-ending");
    } else { // audio
      let el = document.createElement(track.kind);
      el.id = track.id;
      el.srcObject = streams[0];
      el.autoplay = true;
      mountEl.appendChild(el);
      
      const started_message = document.getElementById("started_message");
      started_message.classList.remove("d-none");

    }
    // on remove
    streams[0].onremovetrack = ({ track }) => {
      const el = document.getElementById(track.id);
      if (el) el.parentNode.removeChild(el);
    };
  } else if (kind === "ending") {
    show(".show-when-ending");
    if (state.ducksoup) state.ducksoup.serverLog("interaction_ending_received");
  } else if (kind === "files") {
    if (payload && payload[state.userId]) {
      let html = "The test just finished. Were you able to hear yourself correctly, with good volume and without background noise? If not then you are not allowed to participate in the experiment. Please return your prolific submission using <a href=\"https://app.prolific.com/submissions/complete?cc=C1A9QE6C\">this link</a>. If you were able to hear yourself clearly and with a good volume, you can participate in the experiment. Please use the code <strong>2025</strong> to proceed. ";
      // html += payload[state.userId].join("<br/>") + "<br/>";
      replaceMessage(html);
    } else {
      console.log(kind, payload);
      replaceMessage("Connection terminated");
    }
  } else if (kind === "error-duplicate") {
    replaceMessage("Connection denied (already connected)");
  } else if (kind === "error") {
    replaceMessage("Error");
  }
};

