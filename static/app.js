// Config

const constraints = {
  video: {
    width: { ideal: 640 },
    height: { ideal: 480 },
    frameRate: { max: 30 },
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

const configuration = {
  iceServers: [
    {
      urls: "stun:stun.l.google.com:19302",
    },
  ],
};

// Signaling websocket channel
const signaling = new WebSocket(`wss://${window.location.host}/signaling`);
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
      //   answer.sdp = answer.sdp.replace(
      //     "useinbandfec=1",
      //     "useinbandfec=1; maxaveragebitrate=510000"
      //   );
      console.log("-- anwser.sdp");
      console.log(answer.sdp);
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

// RTCPeerConnection
const pc = new RTCPeerConnection(configuration);

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
  console.log(event.track);
  el.srcObject = event.streams[0];
  el.autoplay = true;
  document.getElementById("remoteVideos").appendChild(el);

  event.streams[0].onremovetrack = ({ track }) => {
    if (el.parentNode) {
      el.parentNode.removeChild(el);
    }
  };
};

// Start
const start = async () => {
  try {
    const stream = await navigator.mediaDevices.getUserMedia(constraints);
    const localVideoEl = document.getElementById("localVideo");
    localVideoEl.srcObject = stream;
    stream.getTracks().forEach((track) => pc.addTrack(track, stream));
  } catch (err) {
    console.error(err);
  }
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
const logStats = async () => {
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

document.addEventListener("DOMContentLoaded", start);
document.addEventListener("DOMContentLoaded", () => {
  setInterval(logStats, 1000);
});
