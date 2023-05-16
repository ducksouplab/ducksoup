// Config

const DEFAULT_CONSTRAINTS = {
  video: {
    width: { ideal: 800 },
    height: { ideal: 600 },
    frameRate: { ideal: 30 },
    facingMode: { ideal: "user" },
  },
  audio: {
    sampleSize: 16,
    channelCount: 1,
    autoGainControl: false,
    latency: { ideal: 0.003 },
    noiseSuppression: false,
  },
};

const DEFAULT_RTC_CONFIG = {
  iceServers: [
    {
      urls: "stun:stun.l.google.com:19302",
    },
    {
      urls: "stun:stun3.l.google.com:19302",
    },
  ],
};

const MAX_VIDEO_BITRATE = 1000000;
const MAX_AUDIO_BITRATE = 64000;

// Init

document.addEventListener("DOMContentLoaded", async () => {
  console.log("[DuckSoup] v1.5.31");

  const ua = navigator.userAgent;
  const containsChrome = ua.indexOf("Chrome") > -1;
  const containsSafari = ua.indexOf("Safari") > -1;
  // needed for safari (getUserMedia before enumerateDevices), but could be a problem if constraints change for Chrome
  if (containsSafari && !containsChrome) {
    await navigator.mediaDevices.getUserMedia(DEFAULT_CONSTRAINTS);
  }
});

// Pure functions

const optionsFirstError = (
  { mountEl, callback },
  { interactionName, userId, duration }
) => {
  if (!mountEl && !callback) return "invalid embedOptions";
  if (
    typeof interactionName === "undefined" ||
    typeof userId === "undefined" ||
    isNaN(duration)
  )
    return "invalid peerOptions";
  return null;
};

const clean = (obj) => {
  for (let prop in obj) {
    if (obj[prop] === null || obj[prop] === undefined) delete obj[prop];
  }
  return obj;
};

const parseJoinPayload = (peerOptions) => {
  // explicit list, without origin
  let {
    interactionName,
    userId,
    duration,
    size,
    width,
    height,
    audioFx,
    videoFx,
    frameRate,
    namespace,
    videoFormat,
    recordingMode,
    gpu,
    overlay,
  } = peerOptions;
  if (!["VP8", "H264"].includes(videoFormat)) videoFormat = null;
  if (isNaN(size)) size = null;
  if (isNaN(width)) width = null;
  if (isNaN(height)) height = null;
  if (isNaN(frameRate)) frameRate = null;
  if (!gpu) gpu = null;
  if (!overlay) overlay = null;

  return clean({
    interactionName,
    userId,
    duration,
    size,
    width,
    height,
    audioFx,
    videoFx,
    frameRate,
    namespace,
    videoFormat,
    recordingMode,
    gpu,
    overlay,
  });
};

const preferMono = (sdp) => {
  // https://datatracker.ietf.org/doc/html/rfc7587#section-6.1
  return sdp
    .split("\r\n")
    .map((line) => {
      if (line.startsWith("a=fmtp:111")) {
        if (line.includes("stereo=")) {
          return line.replace("stereo=1", "stereo=0");
        } else {
          return `${line};stereo=0`;
        }
      } else {
        return line;
      }
    })
    .join("\r\n");
};

const addTWCC = (sdp) => {
  // TODO improved parsing/placing/indexing of additional extmap
  return sdp
    .split("\r\n")
    .map((line) => {
      if (line.startsWith("a=extmap:2 ")) {
        return (
          line +
          "\r\na=extmap:3 http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01"
        );
      } else {
        return line;
      }
    })
    .join("\r\n");
};

const processSDP = (sdp) => {
  let output = preferMono(sdp);
  // output = addTWCC(output);
  return output;
};

const kbps = (bytes, duration) => {
  const result = (8 * bytes) / duration / 1024;
  return result.toFixed(1);
};

const looseJSONParse = (str) => {
  try {
    return JSON.parse(str);
  } catch (error) {
    console.error(error);
  }
};

// Unpure functions

const state = {};

const rampBitrate = (pc) => {
  const RAMP_DURATION = 3000;
  const STEPS = 8;
  let step = 0;
  state.rampInterval = setInterval(async () => {
    step++;
    for (const sender of pc.getSenders()) {
      // set bitrate
      const params = sender.getParameters();
      if (!params.encodings) params.encodings = [{}]; // needed for FF
      for (const encoding of params.encodings) {
        if (sender.track.kind === "video") {
          encoding.maxBitrate = (MAX_VIDEO_BITRATE * step) / STEPS;
        } else if (step === 1) {
          // do once for audio
          encoding.maxBitrate = MAX_AUDIO_BITRATE;
        }
      }
      await sender.setParameters(params);
    }
    if (step === STEPS) {
      clearInterval(state.rampInterval);
    }
  }, RAMP_DURATION / STEPS);
};

// DuckSoup

class DuckSoup {
  // API

  constructor(embedOptions, peerOptions) {
    console.debug("[DuckSoup] embedOptions: ", embedOptions);
    console.debug("[DuckSoup] peerOptions: ", peerOptions);

    const err = optionsFirstError(embedOptions, peerOptions);
    if (err) throw new Error(err);

    const { mountEl } = embedOptions;
    if (mountEl) {
      this._mountEl = mountEl;
      // replace mountEl contents
      while (mountEl.firstChild) {
        mountEl.removeChild(mountEl.firstChild);
      }
    }
    this._signalingUrl = peerOptions.signalingUrl;
    this._rtcConfig = peerOptions.rtcConfig || DEFAULT_RTC_CONFIG;
    this._joinPayload = parseJoinPayload(peerOptions);
    // by default we cancel echo except in mirror mode (interaction size=1) (mirror mode is for test purposes)
    const echoCancellation = this._joinPayload.size !== 1;
    this._constraints = {
      audio: {
        ...DEFAULT_CONSTRAINTS.audio,
        echoCancellation,
        ...peerOptions.audio,
      },
      video: { ...DEFAULT_CONSTRAINTS.video, ...peerOptions.video },
    };
    this._logLevel = 1;
    if (peerOptions && typeof peerOptions.logLevel !== undefined) {
      this._logLevel = peerOptions.logLevel;
    }
    this._stats = embedOptions && embedOptions.stats;
    this._doCallback = embedOptions && embedOptions.callback;
    // needed for debug and stats
    this._info = {
      now: Date.now(),
      audioBytesSent: 0,
      audioBytesReceived: 0,
      videoBytesSent: 0,
      videoBytesReceived: 0,
      encodedWith: undefined,
      encodedHeight: undefined,
      pliCount: 0,
      firCount: 0,
      keyFramesEncoded: 0,
      keyFramesDecoded: 0,
      framesPerSecond: undefined,
    };
  }

  controlFx(name, property, value, duration, userId) {
    if (!this._checkControl(name, property, value, duration, userId)) return;
    this._send("client_control", {
      name,
      property,
      value,
      ...(duration && { duration }),
      ...(userId && { userId }),
    });
  }

  polyControlFx(name, property, kind, value) {
    if (!this._checkControl(name, property, value)) return;
    const strValue = value.toString();
    this._send("client_polycontrol", { name, property, kind, value: strValue });
  }

  stop(code = 1000) {
    if (this._ws) {
      this._send("stop"); // will stop server ressources faster that _stopRTC
      this._ws.close(code); // https://datatracker.ietf.org/doc/html/rfc6455#section-7.4.1
    }
    this._stopRTC();
  }

  log(kind, payload) {
    this._send(`ext_${kind}`, payload);
  }

  // called by debug/client app to do tests
  async limit(maxKbps) {
    for (const sender of this._pc.getSenders()) {
      // set bitrate
      const params = sender.getParameters();
      if (!params.encodings) params.encodings = [{}]; // needed for FF
      for (const encoding of params.encodings) {
        if (sender.track.kind === "video") {
          encoding.maxBitrate = maxKbps * 1000;
        }
      }
      await sender.setParameters(params);
    }
  }

  // Inner methods

  _send(kind, payload) {
    const message = { kind };
    // conditionnally add and possiblty format payload
    if (!!payload) {
      const payloadStr =
        typeof payload === "string" ? payload : JSON.stringify(payload);
      message.payload = payloadStr;
    }
    this._ws.send(JSON.stringify(message));
  }

  async _initialize() {
    try {
      // async calls
      await this._startRTC();
      this._running = true;
    } catch (err) {
      console.log(err);
      this._callback({ kind: "error", payload: err });
    }
  }

  _checkControl(name, property, value, duration, userId) {
    const durationValid =
      typeof duration === "undefined" || typeof duration === "number";
    const userIdValid =
      typeof userId === "undefined" || typeof userId === "string";
    return (
      typeof name === "string" &&
      typeof property === "string" &&
      typeof value === "number" &&
      durationValid &&
      userIdValid
    );
  }

  // send local event to js that has embedded the player
  _callback(event, force) {
    if (this._doCallback && (this._running || force)) {
      const message = typeof event === "string" ? { kind: event } : event;
      this._doCallback(message);
    }
  }

  _stopRTC() {
    if (state.rampInterval) {
      clearInterval(state.rampInterval);
      delete state.rampInterval;
    }
    if (this._stream) {
      this._stream.getTracks().forEach((track) => track.stop());
    }
    if (this._pc) {
      this._pc.close();
    }
  }

  _debugCandidatePair(pair) {
    this._send(
      "client_selected_candidate_pair",
      `client=${pair.local.candidate} server=${pair.remote.candidate}`
    );
  }

  async _startRTC() {
    // RTCPeerConnection
    const pc = new RTCPeerConnection(this._rtcConfig);
    this._pc = pc;

    // Add local tracks before signaling
    const stream = await navigator.mediaDevices.getUserMedia(this._constraints);
    stream.getTracks().forEach((track) => {
      // implement a mute-like behavior (with `enabled`) until the interaction does start
      // see https://developer.mozilla.org/en-US/docs/Web/API/MediaStreamTrack/enabled
      //track.enabled = false;//disabled for now
      pc.addTrack(track, stream);
    });
    this._callback(
      {
        kind: "local-stream",
        payload: stream,
      },
      true
    );
    this._stream = stream;

    // Signaling
    const { href } = window.location;  
    const ws = new WebSocket(this._signalingUrl + '?href=' + encodeURI(href));

    this._ws = ws;

    ws.onclose = (event) => {
      console.log("[DuckSoup] ws.onclose ", event);
      this._callback("closed");
      this._stopRTC();
      if (this._statsIntervalId) clearInterval(this._statsIntervalId);
    };

    ws.onerror = (event) => {
      console.log("[DuckSoup] ws.onerror ", event);
      this._callback({ kind: "error", payload: event.data });
      this.stop(4000); // used as error
    };

    ws.onmessage = async (event) => {
      //console.log("[DuckSoup] ws.onmessage ", event);
      let message = looseJSONParse(event.data);

      if (message.kind === "offer") {
        const offer = looseJSONParse(message.payload);
        console.debug(
          `[DuckSoup] server offer sdp (length ${message.payload.length}):\n${offer.sdp}`
        );

        pc.setRemoteDescription(offer);
        // console.log("[DuckSoup] offer: ", offer);
        const answer = await pc.createAnswer();
        answer.sdp = processSDP(answer.sdp);
        pc.setLocalDescription(answer);
        this._send("client_answer", answer);
      } else if (message.kind === "candidate") {
        const candidate = looseJSONParse(message.payload);
        console.debug("[DuckSoup] server candidate:", candidate);
        try {
          pc.addIceCandidate(candidate);
        } catch (error) {
          console.error(error);
        }
      } else if (message.kind === "start") {
        // set encoding parameters
        rampBitrate(pc);
        // unmute
        // stream.getTracks().forEach((track) => {
        //     track.enabled = true;
        // });
        this._callback({ kind: "start" }, true); // force with true since player is not already running
        // Getting peerconnection stats is needed either for stats or debug option
        if (this._stats || this._logLevel >= 1) {
          console.log(`[DuckSoup] start stats`);
          this._statsIntervalId = setInterval(() => this._updateStats(), 1000);
        }
      } else if (message.kind === "ending") {
        this._callback({ kind: "ending" });
      } else if (message.kind === "files") {
        this._callback(message);
      } else if (message.kind.startsWith("error")) {
        this._callback(message);
        this.stop(4000);
      }
    };

    ws.onopen = () => {
      this._send("join", this._joinPayload);

      pc.onicecandidate = (e) => {
        if (!e.candidate) return;
        this._send("client_ice_candidate", e.candidate);
        console.debug("[DuckSoup] client candidate:", e.candidate);
      };

      pc.ontrack = (event) => {
        console.debug(
          `[DuckSoup] on track (while connection state is ${pc.connectionState})`
        );
        if (this._mountEl) {
          let el = document.createElement(event.track.kind);
          el.id = event.track.id;
          el.srcObject = event.streams[0];
          el.autoplay = true;
          if (event.track.kind === "video") {
            if (this._joinPayload.width) {
              el.style.width = this._joinPayload.width + "px";
            } else {
              el.style.width = "100%";
            }
            if (this._joinPayload.height) {
              el.style.height = this._joinPayload.height + "px";
            }
          }
          this._mountEl.appendChild(el);
          // on remove
          event.streams[0].onremovetrack = ({ track }) => {
            const el = document.getElementById(track.id);
            if (el) el.parentNode.removeChild(el);
          };
        } else {
          this._callback({
            kind: "track",
            payload: event,
          });
        }
      };

      // for server logging
      if (this._logLevel >= 2) {
        pc.onconnectionstatechange = () => {
          this._send("client_connection_state_changed", pc.connectionState);
          // console.debug("[DuckSoup] onconnectionstatechange:", pc.connectionState);
        };

        pc.onsignalingstatechange = () => {
          this._send(
            "client_signaling_state_changed",
            pc.signalingState.toString()
          );
          // console.debug("[DuckSoup] onsignalingstatechange:", pc.signalingState.toString());
        };

        pc.onnegotiationneeded = () => {
          this._send("client_negotiation_needed", pc.signalingState);
          console.debug("[DuckSoup] onnegotiationneeded: ", pc.signalingState);
        };

        pc.oniceconnectionstatechange = () => {
          const state = pc.iceConnectionState;
          this._send("client_ice_connection_state_" + state);
          if (state === "connected") {
            // add listeners on first sender (likely the same info to be shared for audio and video)
            const firstSender = pc.getSenders()[0];
            if (firstSender) {
              const { iceTransport } = firstSender.transport;
              if (iceTransport && this._logLevel >= 2) {
                const pair = iceTransport.getSelectedCandidatePair();
                this._debugCandidatePair(pair);
                console.debug(
                  `[DuckSoup] selected candidate pair: client=${pair.local.candidate} server=${pair.remote.candidate}`
                );
              }
            }
          }
        };

        pc.onicegatheringstatechange = () => {
          this._send(
            "client_ice_gathering_state_changed",
            pc.iceGatheringState.toString()
          );
          // console.debug("[DuckSoup] onicegatheringstatechange:", pc.iceGatheringState.toString());
        };

        pc.onicecandidateerror = (e) => {
          this._send(
            "client_ice_candidate_failed",
            `${e.url}#${e.errorCode}: ${e.errorText}`
          );
          // console.debug("[DuckSoup] onicecandidateerror:", `${e.url}#${e.errorCode}: ${e.errorText}`);
        };
      }
    };

    setTimeout(() => {
      if (ws.readyState === 0) {
        console.error("[DuckSoup] ws can't connect (after 10 seconds)");
      }
    }, 10000);
  }

  async _updateStats() {
    const pc = this._pc;
    const pcStats = await pc.getStats();

    if (this._logLevel >= 1) {
      pcStats.forEach((report) => {
        if (report.type === "outbound-rtp" && report.kind === "video") {
          // encoded size
          let newEncodedWidth = report.frameWidth;
          let newEncodedHeight = report.frameHeight;
          if (
            newEncodedWidth &&
            newEncodedHeight &&
            (newEncodedWidth !== this._info.encodedWith ||
              newEncodedHeight !== this._info.encodedHeight)
          ) {
            this._send(
              "client_video_resolution_updated",
              `${newEncodedWidth}x${newEncodedHeight}`
            );
            this._info.encodedWith = newEncodedWidth;
            this._info.encodedHeight = newEncodedHeight;
          }
          // FPS
          let newFramesPerSecond = report.framesPerSecond;
          if (
            typeof newFramesPerSecond !== "undefined" &&
            newFramesPerSecond !== this._info.framesPerSecond
          ) {
            this._send("client_video_fps_updated", `${newFramesPerSecond}`);
            this._info.framesPerSecond = newFramesPerSecond;
          }
          // PLI
          let newPliCount = report.pliCount;
          if (
            typeof newPliCount !== "undefined" &&
            newPliCount !== this._info.pliCount
          ) {
            this._send("client_pli_received_count_updated", `${newPliCount}`);
            this._info.pliCount = newPliCount;
          }
          // FIR
          let newFirCount = report.firCount;
          if (
            typeof newFirCount !== "undefined" &&
            newFirCount !== this._info.firCount
          ) {
            this._send("client_fir_received_count_updated", `${newFirCount}`);
            this._info.firCount = newFirCount;
          }
          // KF
          let newKeyFramesEncoded = report.keyFramesEncoded;
          if (
            typeof newKeyFramesEncoded !== "undefined" &&
            newKeyFramesEncoded !== this._info.keyFramesEncoded
          ) {
            this._send(
              "client_keyframe_encoded_count_updated",
              `${newKeyFramesEncoded}`
            );
            this._info.keyFramesEncoded = newKeyFramesEncoded;
            //console.debug("[DuckSoup] encoded KFs", newKeyFramesEncoded);
          }
        }
        if (report.type === "inbound-rtp" && report.kind === "video") {
          // KF
          let newKeyFramesDecoded = report.keyFramesDecoded;
          if (
            typeof newKeyFramesDecoded !== "undefined" &&
            newKeyFramesDecoded !== this._info.keyFramesDecoded
          ) {
            this._send(
              "client_keyframe_decoded_count_updated",
              `${newKeyFramesDecoded}`
            );
            this._info.keyFramesDecoded = newKeyFramesDecoded;
            //console.debug("[DuckSoup] decoded KFs", newKeyFramesDecoded);
          }
        }
      });
    }

    if (this._stats) {
      const newNow = Date.now();
      let newAudioBytesSent = 0;
      let newAudioBytesReceived = 0;
      let newVideoBytesSent = 0;
      let newVideoBytesReceived = 0;
      let outboundRTPVideo, inboundRTPVideo, outboundRTPAudio, inboundRTPAudio;
      let remoteOutboundRTPVideo,
        remoteInboundRTPVideo,
        remoteOutboundRTPAudio,
        remoteInboundRTPAudio;

      pcStats.forEach((report) => {
        if (report.type === "outbound-rtp" && report.kind === "audio") {
          newAudioBytesSent += report.bytesSent;
          outboundRTPAudio = report;
        } else if (report.type === "inbound-rtp" && report.kind === "audio") {
          newAudioBytesReceived += report.bytesReceived;
          inboundRTPAudio = report;
        } else if (report.type === "outbound-rtp" && report.kind === "video") {
          newVideoBytesSent += report.bytesSent;
          outboundRTPVideo = report;
        } else if (report.type === "inbound-rtp" && report.kind === "video") {
          newVideoBytesReceived += report.bytesReceived;
          inboundRTPVideo = report;
        } else if (
          report.type === "remote-outbound-rtp" &&
          report.kind === "audio"
        ) {
          remoteOutboundRTPAudio = report;
        } else if (
          report.type === "remote-inbound-rtp" &&
          report.kind === "audio"
        ) {
          remoteInboundRTPAudio = report;
        } else if (
          report.type === "remote-outbound-rtp" &&
          report.kind === "video"
        ) {
          remoteOutboundRTPVideo = report;
        } else if (
          report.type === "remote-inbound-rtp" &&
          report.kind === "video"
        ) {
          remoteInboundRTPVideo = report;
        }
      });
      const elapsed = (newNow - this._info.now) / 1000;
      const audioUp = kbps(
        newAudioBytesSent - this._info.audioBytesSent,
        elapsed
      );
      const audioDown = kbps(
        newAudioBytesReceived - this._info.audioBytesReceived,
        elapsed
      );
      const videoUp = kbps(
        newVideoBytesSent - this._info.videoBytesSent,
        elapsed
      );
      const videoDown = kbps(
        newVideoBytesReceived - this._info.videoBytesReceived,
        elapsed
      );
      this._callback({
        kind: "stats",
        payload: {
          audioUp,
          audioDown,
          videoUp,
          videoDown,
          outboundRTPVideo,
          inboundRTPVideo,
          outboundRTPAudio,
          inboundRTPAudio,
          remoteOutboundRTPVideo,
          remoteInboundRTPVideo,
          remoteOutboundRTPAudio,
          remoteInboundRTPAudio,
        },
      });

      this._info = {
        ...this._info,
        now: newNow,
        audioBytesSent: newAudioBytesSent,
        audioBytesReceived: newAudioBytesReceived,
        videoBytesSent: newVideoBytesSent,
        videoBytesReceived: newVideoBytesReceived,
      };
    }
  }
}

// API

window.DuckSoup = {
  render: async (embedOptions, peerOptions) => {
    const player = new DuckSoup(embedOptions, peerOptions);
    await player._initialize();
    return player;
  },
};
