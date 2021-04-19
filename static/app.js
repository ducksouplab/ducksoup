// State
const state = {
  audioIn: null,
};

// Config

const DEFAULT_CONSTRAINTS = {
  video: {
    width: { ideal: 1280 },
    height: { ideal: 720 },
    frameRate: { ideal: 30 },
    facingMode: { ideal: "user" },
  },
  audio: {
    sampleSize: 16,
    autoGainControl: false,
    channelCount: 1,
    latency: { ideal: 0.003 },
    echoCancellation: false,
    noiseSuppression: false,
  },
};

const DEFAULT_PEER_CONFIGURATION = {
  iceServers: [
    {
      urls: "stun:stun.l.google.com:19302",
    },
  ],
};

// Init
const init = async () => {
  try {
    const devices = await navigator.mediaDevices.enumerateDevices();
    const audioSelect = document.getElementById("audio-select");
    for (let i = 0; i !== devices.length; ++i) {
      const device = devices[i];
      if (device.kind === "audioinput") {
        const li = document.createElement("li");
        const a = document.createElement("a");
        a.href = "#";
        a.text = device.label || `microphone ${audioInputSelect.length + 1}`;
        a.addEventListener("click", () => {
          state.audioIn = device.deviceId;
          document.getElementById("audio-in-label").textContent = device.label;
        });
        li.appendChild(a);
        audioSelect.appendChild(li);
      }
    }
  } catch (err) {
    console.error(err);
  }
};

const processSDP = (sdp) => {
  if (!window.navigator.userAgent.includes("Mozilla")) return sdp;
  return sdp
    .split("\r\n")
    .map((line) => {
      if (line.startsWith("a=fmtp:111")) {
        return line.replace("stereo=1", "stereo=0");
      } else {
        return line;
      }
    })
    .join("\r\n");
};

const startRTC = async () => {
  // UX
  document.getElementById("start-container").classList.add("hide");
  document.getElementById("stop-container").classList.remove("hide");

  // RTCPeerConnection
  const pc = new RTCPeerConnection(DEFAULT_PEER_CONFIGURATION);
  // Add local tracks before signaling
  const constraints = { ...DEFAULT_CONSTRAINTS };
  if (state.audioIn) {
    constraints.audio = {
      ...constraints.audio,
      deviceId: { ideal: state.audioIn },
    };
  }
  const stream = await navigator.mediaDevices.getUserMedia(constraints);
  const localVideoEl = document.getElementById("local-video");
  localVideoEl.srcObject = stream;
  stream.getTracks().forEach((track) => pc.addTrack(track, stream));

  // Signaling
  const wsProtocol = window.location.protocol === "https:" ? "wss" : "ws";
  const signaling = new WebSocket(
    `${wsProtocol}://${window.location.host}/signaling`
  );
  signaling.onclose = function (evt) {
    console.log("Websocket has closed");
  };

  signaling.onmessage = async function (evt) {
    let msg = JSON.parse(evt.data);
    if (!msg) return console.error("failed to parse msg");

    switch (msg.event) {
      case "offer":
        const offer = JSON.parse(msg.data);
        if (!offer) {
          return console.error("failed to parse answer");
        }
        pc.setRemoteDescription(offer);
        const answer = await pc.createAnswer();
        answer.sdp = processSDP(answer.sdp);
        pc.setLocalDescription(answer);
        signaling.send(
          JSON.stringify({
            event: "answer",
            data: JSON.stringify(answer),
          })
        );
        return;

      case "candidate":
        const candidate = JSON.parse(msg.data);
        if (!candidate) {
          return console.error("failed to parse candidate");
        }

        pc.addIceCandidate(candidate);
    }
  };

  signaling.onerror = function (evt) {
    console.error("signaling: " + evt.data);
  };

  pc.onicecandidate = (e) => {
    if (!e.candidate) return;
    signaling.send(
      JSON.stringify({
        event: "candidate",
        data: JSON.stringify(e.candidate),
      })
    );
  };

  pc.ontrack = function (event) {
    let el = document.createElement(event.track.kind);
    el.id = event.track.id;
    console.log(event.streams);
    el.srcObject = event.streams[0];
    el.autoplay = true;
    if (event.track.kind === "video") {
      el.classList.add("responsive-video");
      el.classList.add("col");
      el.classList.add("s6");
    }
    document.getElementById("remote").appendChild(el);

    event.streams[0].onremovetrack = ({ track }) => {
      const el = document.getElementById(track.id);
      if (el) el.parentNode.removeChild(el);
    };
  };

  // Stats
  setInterval(() => logStats(pc), 1000);
};

let now = Date.now();
let audioBytesSent = 0;
let audioBytesReceived = 0;
let videoBytesSent = 0;
let videoBytesReceived = 0;

// Stats
const kbps = (bytes, duration, intro) => {
  const result = (8 * bytes) / duration / 1024;
  return result.toFixed(1);
};
const logStats = async (pc) => {
  const pcStats = await pc.getStats();
  const newNow = Date.now();
  let newAudioBytesSent = 0;
  let newAudioBytesReceived = 0;
  let newVideoBytesSent = 0;
  let newVideoBytesReceived = 0;

  pcStats.forEach((report) => {
    if (report.type === "outbound-rtp" && report.kind === "audio") {
      newAudioBytesSent += report.bytesSent;
    } else if (report.type === "inbound-rtp" && report.kind === "audio") {
      newAudioBytesReceived += report.bytesReceived;
    } else if (report.type === "outbound-rtp" && report.kind === "video") {
      newVideoBytesSent += report.bytesSent;
    } else if (report.type === "inbound-rtp" && report.kind === "video") {
      newVideoBytesReceived += report.bytesReceived;
    }
  });

  const elapsed = (newNow - now) / 1000;
  document.getElementById("audio-up").textContent = kbps(
    newAudioBytesSent - audioBytesSent,
    elapsed
  );
  document.getElementById("audio-down").textContent = kbps(
    newAudioBytesReceived - audioBytesReceived,
    elapsed
  );
  document.getElementById("video-up").textContent = kbps(
    newVideoBytesSent - videoBytesSent,
    elapsed
  );
  document.getElementById("video-down").textContent = kbps(
    newVideoBytesReceived - videoBytesReceived,
    elapsed
  );
  now = newNow;
  audioBytesSent = newAudioBytesSent;
  audioBytesReceived = newAudioBytesReceived;
  videoBytesSent = newVideoBytesSent;
  videoBytesReceived = newVideoBytesReceived;

  // for (const sender of pc.getSenders()) {
  //   console.log("---------- RTCRtpSender stat", sender.track.kind);
  //   const senderStats = await sender.getStats();
  //   senderStats.forEach((report) => {
  //     console.log(report.type, report);
  //   });
  // }
};

document.addEventListener("DOMContentLoaded", init);
// UX
document.addEventListener("DOMContentLoaded", () => {
  const elems = document.querySelectorAll(".dropdown-trigger");
  const instances = M.Dropdown.init(elems, { constrainWidth: false });
  document.getElementById("start").addEventListener("click", startRTC);
  document
    .getElementById("stop")
    .addEventListener("click", () => location.reload());
});
