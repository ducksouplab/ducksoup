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


//set test duration
test_duration = 26

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
  //const namespace = isMirror ? "test_mirror" : "test_interaction";
  const namespace = "audio_visual_test"
  // parse
  const width = parseIntWithFallback(w, 800);
  const height = parseIntWithFallback(h, 600);
  const framerate = parseIntWithFallback(fr, 25);
  const duration = parseIntWithFallback(test_duration, 30);
  const gpu = !!g;
  const overlay = !!o;
  // initialize state
  state = { namespace, interactionName, userId, width, height, isMirror, peerCount: 0 };
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
    clearMount();
    show(".show-when-not-running");
    hide(".show-when-running");

    // Reset everything related to the test period
    clearTimeout(timeoutId);
    const noise_test = document.getElementById("noise_test");
    const signal_test = document.getElementById("signal_test");
    const stop_message_div = document.getElementById("stopped_div");
    stop_message_div.classList.add("d-none");
    signal_test.classList.add("d-none");
    noise_test.classList.add("d-none")
    currentPhase = "noise"
    volumeLoggingActive = false;
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
  const videoInput = document.getElementById("input-video");
  for (let i = 0; i !== devices.length; ++i) {
    const device = devices[i];
    if (device.kind === "audioinput") {
      const option = document.createElement("option");
      option.text = device.label || `microphone ${audioInput.length + 1}`;
      (option.value = device.deviceId), audioInput.appendChild(option);
    } else if (device.kind === "videoinput") {
      const option = document.createElement("option");
      option.text = device.label || `camera ${videoInput.length + 1}`;
      (option.value = device.deviceId), videoInput.appendChild(option);
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
  // replace mountEl contents
  clearMount();
  // update UX
  show(".show-when-not-running");
  show(".show-when-ended");
  hide(".show-when-running");
  hide(".show-when-ending");
};

const clearMessage = () => {
  document.getElementById("stopped-message").innerHTML = "";
};

const replaceMessage = (message, id = document.getElementById("stopped-message")) => {
  id.innerHTML = message 
};

const appendMessage = (message) => {
  document.getElementById("stopped-message").innerHTML += message + "<br/>";
};


//######################################################//
//## SETUP TO MONITOR VOLUME LEVELS DURING AUDIO TEST //##
//######################################################//

// Activate logging
let timeoutId;
let volumeLevels = []; // Array to store volume levels
let noiseLevels = []; // Array to store noise levels
let volumeLoggingActive = false; // Flag to track logging state
let currentPhase = "noise"
let analyser, audioContext, dataArray;

 // Log volume level
 function logVolumeLevel() {
  if (!volumeLoggingActive) return; // Stop logging if the flag is false
  analyser.getByteFrequencyData(dataArray);
  var volume = dataArray.reduce((a, b) => a + b, 0) / dataArray.length;
  
  // Store the noise level
  if (currentPhase == "noise"){noiseLevels.push(volume);}
  // Store the volume level
  if (currentPhase == "signal"){volumeLevels.push(volume);}
  // Continue logging
  requestAnimationFrame(logVolumeLevel);
}

// Helper function to calculate median
function calculateMedian(arr) {
  if (arr.length === 0) return 0; // Handle empty array case

  const sortedArr = [...arr].sort((a, b) => a - b);
  const mid = Math.floor(sortedArr.length / 2);

  return sortedArr.length % 2 !== 0 
      ? sortedArr[mid] 
      : (sortedArr[mid - 1] + sortedArr[mid]) / 2;
}

// Sends audio test data to backend for storage
const sendAudioData = async (namespace, interaction, data) => {
  // Add timestamp to data
  const enrichedData = {
    ...data,
    timestamp: new Date().toISOString()
  };

  try {
    const response = await fetch('/POST_audio_test', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        namespace,
        interaction,
        data: enrichedData
      })
    });
    
    if (!response.ok) throw new Error('Failed to save data');
    return true;
  } catch (error) {
    console.error('Save error:', error);
    return false;
  }
}

//######################################################//
//## SETUP TO MONITOR VOLUME LEVELS DURING AUDIO TEST //##
//######################################################//


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


      //UI Control
      const noise_test = document.getElementById("noise_test");
      const signal_test = document.getElementById("signal_test");
      //Show noiste test ui when test starts.
      noise_test.classList.remove("d-none");

      //Create new audio context 
      audioContext = new window.AudioContext(); //Create a new audio context where we have the streams[0] as the audio input source.
      analyser = audioContext.createAnalyser(); //Creates an analyser node to the audio context so that we can analyse properties of incoming signal.
      source = audioContext.createMediaStreamSource(streams[0]); // Create the source
      source.connect(analyser); // Connect the analyser method/node to the source. 

      dataArray = new Uint8Array(analyser.frequencyBinCount);

      // Activate logging
      volumeLoggingActive = true;
      logVolumeLevel();
      timeoutId = setTimeout(() => {
        currentPhase = "signal"
        //Remove noise test UI
        noise_test.classList.add("d-none");
        //Add signal test UI
        signal_test.classList.remove("d-none")
      }, 12000);


    } else { // audio
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
    if (state.ducksoup) state.ducksoup.serverLog("interaction_ending_received");
  } else if (kind === "files") {
      // Deactivate logging
      volumeLoggingActive = false;
      // Calculate the average volume level
      var medianVolume = calculateMedian(volumeLevels)
      var medianNoise = calculateMedian(noiseLevels)
      volumeLevels = []; //Reset volume sample
      noiseLevels = []; //Reset volume sample
      currentPhase = "noise"; //Reset phase

      const signal_test = document.getElementById("signal_test");
      signal_test.classList.add("d-none");

      const passed = medianNoise < 2 && medianVolume > 4.5;
      sendAudioData(state.namespace, state.interactionName, {
        noiseLevels: medianNoise,
        volumeLevels: medianVolume,
        passed: passed
      });

    if (payload && payload[state.userId]) {
      if (passed){
          let html =  `
          <p id="stopped-message">
            The test just finished. Your microphone is of sufficient quality. Could you hear yourself <b>clearly</b> in your headphones?  Could you see yourself on the screen?
          </p>
          <p>
             If the answer is <b>no</b> to any of these questions, you are not allowed to continue. Please <b>return</b> your Prolific submission using 
            <a href="https://app.prolific.com/submissions/complete?cc=C1A9QE6C">this link</a>. You will be paid for the time you took setting up the experiment.
          </p>
          <p>
            If you could hear and see yourself clearly, you are allowed to continue with the experiment. 
            Please use the code <b>2025</b> to proceed.
          </p>
            `;
            div = document.getElementById("stopped_div");
            div.classList.remove("d-none");
            div.style.padding = "20px";
            div.style.margin = "20px";
            replaceMessage(html, div);
      }else{let html =  `
        <p id="stopped-message">
          The test just finished. Unfortunately you <b>do not</b> meet the microphone quality requirements needed for this study. 
          We are very sorry about this. You will be <b>compensated</b> for the time you spent setting up. 
        </p>

        <p id="stopped-message">
          If you <b>unintentionally</b> failed to follow the instructions you are allowed to redo the test. Otherwise we kindly ask you to <b>return</b> your Prolific submission using 
          <a href="https://app.prolific.com/submissions/complete?cc=C1A9QE6C">this link</a>.
        </p>
      `;
      div = document.getElementById("stopped_div");
      div.classList.remove("d-none");
      div.style.padding = "20px";
      div.style.margin = "20px";
      replaceMessage(html, div);
      }
    } else {
      console.log(kind, payload);
      replaceMessage("Connection terminated");
    }
  } else if (kind === "error-duplicate") {
    replaceMessage("Connection denied (already connected)");
  } else if (kind === "error") {
    replaceMessage("Error");
  } else if (kind === "stats") {
    document.getElementById("audio-out-bitrate").textContent = payload.audioUp;
    document.getElementById("audio-in-bitrate").textContent = payload.audioDown;
    document.getElementById("video-out-bitrate").textContent = payload.videoUp;
    document.getElementById("video-in-bitrate").textContent = payload.videoDown;
    const {
      outboundRTPVideo,
      inboundRTPVideo,
      outboundRTPAudio,
      inboundRTPAudio,
      remoteOutboundRTPVideo,
      remoteInboundRTPVideo,
      remoteOutboundRTPAudio,
      remoteInboundRTPAudio,
    } = payload;
    if (outboundRTPVideo) {
      // add processed props
      outboundRTPVideo.averageEncodeTime = Number(
        outboundRTPVideo.totalEncodeTime / outboundRTPVideo.framesEncoded
      ).toFixed(3);
      // select displayed props
      const props = [
        "frameWidth",
        "frameHeight",
        "framesPerSecond",
        "qualityLimitationReason",
        "keyFramesEncoded",
        "firCount",
        "pliCount",
        "sliCount",
        "nackCount",
        "framesDiscardedOnSend",
        "averageEncodeTime",
        "packetsSent",
      ];
      // render
      for (let p of props) {
        document.getElementById(`video-out-${p}`).textContent =
          outboundRTPVideo[p];
      }
    }
    if (inboundRTPVideo) {
      // add processed props
      inboundRTPVideo.processedJitter = Number(
        inboundRTPVideo.jitterBufferDelay /
          inboundRTPVideo.jitterBufferEmittedCount
      ).toFixed(3);
      // select displayed props
      const props = [
        "frameWidth",
        "frameHeight",
        "framesPerSecond",
        "keyFramesDecoded",
        "pliCount",
        "firCount",
        "sliCount",
        "nackCount",
        "processedJitter",
        "jitter",
        "packetsReceived",
        "packetsLost",
        "packetsDiscarded",
        "packetsRepaired",
        "framesDropped",
      ];
      // render
      for (let p of props) {
        document.getElementById(`video-in-${p}`).textContent =
          inboundRTPVideo[p];
      }
    }
    if (outboundRTPAudio) {
      // select displayed props
      const props = ["nackCount", "targetBitrate", "packetsSent"];
      // render
      for (let p of props) {
        document.getElementById(`audio-out-${p}`).textContent =
          outboundRTPAudio[p];
      }
    }
    if (inboundRTPAudio) {
      // add processed props
      inboundRTPAudio.processedJitter = Number(
        inboundRTPAudio.jitterBufferDelay /
          inboundRTPAudio.jitterBufferEmittedCount
      ).toFixed(3);
      if (inboundRTPAudio.totalSamplesDuration) {
        inboundRTPAudio.totalSamplesDuration =
          inboundRTPAudio.totalSamplesDuration.toFixed(2);
      }
      // select displayed props
      const props = [
        "processedJitter",
        "nackCount",
        "concealedSamples",
        "totalSamplesDuration",
        "jitter",
        "packetsReceived",
        "packetsLost",
        "packetsDiscarded",
        "packetsRepaired",
      ];
      // render
      for (let p of props) {
        document.getElementById(`audio-in-${p}`).textContent =
          inboundRTPAudio[p];
      }
    }
    if (remoteOutboundRTPAudio) {
      // select displayed props
      const props = [
        "packetsSent",
        "fractionLost",
        "packetsLost",
        "roundTripTime",
        "roundTripTimeMeasurements",
        "totalRoundTripTime",
      ];
      // render
      for (let p of props) {
        document.getElementById(`remote-audio-out-${p}`).textContent =
          remoteOutboundRTPAudio[p];
      }
    }
    if (remoteOutboundRTPVideo) {
      // select displayed props
      const props = [
        "packetsSent",
        "fractionLost",
        "packetsLost",
        "roundTripTime",
        "roundTripTimeMeasurements",
        "totalRoundTripTime",
      ];
      // render
      for (let p of props) {
        document.getElementById(`remote-video-out-${p}`).textContent =
          remoteOutboundRTPVideo[p];
      }
    }
    if (remoteInboundRTPAudio) {
      if (remoteInboundRTPAudio.jitter) {
        remoteInboundRTPAudio.jitter = remoteInboundRTPAudio.jitter.toFixed(3);
      }
      // select displayed props
      const props = [
        "jitter",
        "fractionLost",
        "packetsLost",
        "roundTripTime",
        "roundTripTimeMeasurements",
        "totalRoundTripTime",
      ];
      // render
      for (let p of props) {
        document.getElementById(`remote-audio-in-${p}`).textContent =
          remoteInboundRTPAudio[p];
      }
    }
    if (remoteInboundRTPVideo) {
      if (remoteInboundRTPVideo.jitter) {
        remoteInboundRTPVideo.jitter = remoteInboundRTPVideo.jitter.toFixed(3);
      }
      // select displayed props
      const props = [
        "jitter",
        "fractionLost",
        "packetsLost",
        "roundTripTime",
        "roundTripTimeMeasurements",
        "totalRoundTripTime",
      ];
      // render
      for (let p of props) {
        document.getElementById(`remote-video-in-${p}`).textContent =
          remoteInboundRTPVideo[p];
      }
    }
  }
};
