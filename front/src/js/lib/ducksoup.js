// Config

const DEFAULT_CONSTRAINTS = {
  video: {
    width: { ideal: 800 },
    height: { ideal: 600 },
    frameRate: { ideal: 25 },
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

const MAX_VIDEO_BITRATE = 1500000;
const MAX_AUDIO_BITRATE = 64000;
const BITRATE_RAMP_DURATION = 3000;

// Chrome 122 fix

let chrome122Fix = false;

// Init

document.addEventListener("DOMContentLoaded", async () => {
  const ua = navigator.userAgent;
  const containsChrome = ua.indexOf("Chrome") > -1;
  const containsSafari = ua.indexOf("Safari") > -1;
  // needed for safari (getUserMedia before enumerateDevices), but could be a problem if constraints change for Chrome
  if (containsSafari && !containsChrome) {
    await navigator.mediaDevices.getUserMedia(DEFAULT_CONSTRAINTS);
  }
  if (containsChrome) {
    const versionExec = /Chrome\/([0-9]+)/.exec(ua);
    if (versionExec.length > 1) {
      const version = parseInt(versionExec[1], 10);
      if (version >= 122) {
        chrome122Fix = true;
      }
    }
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
    audioOnly,
    size,
    width,
    height,
    audioFx,
    videoFx,
    framerate,
    namespace,
    videoFormat,
    recordingMode,
    gpu,
    overlay,
  } = peerOptions;
  // null fields will be deleted by clean()
  if (!["VP8", "H264"].includes(videoFormat)) videoFormat = null;
  audioOnly = !!audioOnly ? true : null;
  if (isNaN(size)) size = null;
  if (isNaN(width)) width = null;
  if (isNaN(height)) height = null;
  if (isNaN(framerate)) framerate = null;
  if (!gpu) gpu = null;
  if (!overlay) overlay = null;

  return clean({
    interactionName,
    userId,
    duration,
    audioOnly,
    size,
    width,
    height,
    audioFx,
    videoFx,
    framerate,
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


const fixChrome122 = (sdp) => {
  console.log(chrome122Fix);
  if (!chrome122Fix) return sdp;
  return sdp
    .split("\r\n")
    .map((line) => {
      if (chrome122Fix && line.startsWith("a=group:BUNDLE")) {
        return `${line}\r\na=msid-semantic: WMS`;      
      } else {
        return line;
      }
    })
    .join("\r\n");
};

const processOffer = (sdp) => {
  let output = fixChrome122(sdp);
  return output;
};

const processAnswer = (sdp) => {
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
  }, BITRATE_RAMP_DURATION / STEPS);
};

// DuckSoup

class DuckSoup {
  // private instance fields
  #pc;
  #ws;
  #started;
  #startedRTC;
  #stopped;
  #rtcConfig;
  #stream;
  #info;
  #stats;
  #logLevel;
  #mountEl;
  #joinPayload;
  #constraints;
  #statsIntervalId;
  #signalingUrl;
  #callback;
  #pendingCandidates;

  // API

  constructor(embedOptions, peerOptions) {
    // log
    console.debug("[DS] embedOptions: ", embedOptions);
    console.debug("[DS] peerOptions: ", peerOptions);

    // check errors
    const err = optionsFirstError(embedOptions, peerOptions);
    if (err) throw new Error(err);

    // init
    this.#started = false // locally started
    this.#startedRTC = false // signaling with server has come to start peer connection
    this.#stopped = false;
    this.#pendingCandidates = [];

    const { mountEl } = embedOptions;
    if (mountEl) {
      this.#mountEl = mountEl;
      // replace mountEl contents
      while (mountEl.firstChild) {
        mountEl.removeChild(mountEl.firstChild);
      }
    }
    this.#signalingUrl = peerOptions.signalingUrl;
    this.#joinPayload = parseJoinPayload(peerOptions);
    // by default we cancel echo except in mirror mode (interaction size=1) (mirror mode is for test purposes)
    const echoCancellation = this.#joinPayload.size !== 1;
    this.#constraints = {
      audio: {
        ...DEFAULT_CONSTRAINTS.audio,
        echoCancellation,
        ...peerOptions.audio,
      }
    };
    if (!this.#joinPayload.audioOnly) {
      this.#constraints.video = { ...DEFAULT_CONSTRAINTS.video, ...peerOptions.video };
    }
    this.#logLevel = 1;
    if (peerOptions && typeof peerOptions.logLevel !== undefined) {
      this.#logLevel = peerOptions.logLevel;
    }
    this.#stats = embedOptions && embedOptions.stats;
    this.#callback = embedOptions && embedOptions.callback;
    // needed for debug and stats
    this.#info = {
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

  async start() {
    try {
      // async calls
      await this.#startWS();
      this.#started = true;
    } catch (err) {
      console.log(err);
      this.#forward({ kind: "error", payload: err });
    }
  }

  stop(code = 1000) {
    if (this.#stopped) return;
    if (this.#ws) {
      this.#serverSend("stop"); // will stop server ressources faster that #stopRTC
      this.#ws.close(code); // https://datatracker.ietf.org/doc/html/rfc6455#section-7.4.1
    }
    this.#stopRTC();
    this.#stopped = true;
  }

  controlFx(name, property, value, duration, userId) {
    if (!this.#checkControl(name, property, value, duration, userId)) return;
    this.#serverSend("client_control", {
      name,
      property,
      value,
      ...(duration && { duration }),
      ...(userId && { userId }),
    });
  }

  polyControlFx(name, property, kind, value) {
    if (!this.#checkControl(name, property, value)) return;
    const strValue = value.toString();
    this.#serverSend("client_polycontrol", { name, property, kind, value: strValue });
  }

  // add prefix to differentiate from ducksoup.js logs
  serverLog(kind, payload) {
    this.#serverSend(`ext_${kind}`, payload);
  }

  // called by debug/client app to do tests
  async limit(maxKbps) {
    for (const sender of this.#pc.getSenders()) {
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

  #serverSend(kind, payload) {
    if (this.#ws.readyState === 1) { // the connection is open and ready to communicate
      const message = { kind };
      // conditionnally add and possiblty format payload
      if (!!payload) {
        const payloadStr =
          typeof payload === "string" ? payload : JSON.stringify(payload);
        message.payload = payloadStr;
      }
      this.#ws.send(JSON.stringify(message));
    }
  }

  #checkControl(name, property, value, duration, userId) {
    const durationValid =
      typeof duration === "undefined" || typeof duration === "number";
    const userIdValid =
      typeof userId === "undefined" || typeof userId === "string";
    return (
      typeof name === "string" &&
      typeof property === "string" &&
      (typeof value === "number" || typeof value === "string") &&
      durationValid &&
      userIdValid
    );
  }

  // send local event to js that has embedded the player
  #forward(event, force) {
    if (this.#callback && (this.#started || force)) {
      const message = typeof event === "string" ? { kind: event } : event;
      this.#callback(message);
    }
  }

  #stopRTC() {
    if (state.rampInterval) {
      clearInterval(state.rampInterval);
      delete state.rampInterval;
    }
    if (this.#statsIntervalId) {
      clearInterval(this.#statsIntervalId);
    }
    if (this.#stream) {
      this.#stream.getTracks().forEach((track) => track.stop());
    }
    if (this.#pc) {
      this.#pc.close();
    }
  }

  #debugCandidatePair(pair) {
    this.#serverSend(
      "client_selected_candidate_pair",
      `client=${pair.local.candidate} server=${pair.remote.candidate}`
    );
  }

  async #startWS() {
    // Signaling
    const { href } = window.location;  
    const ws = new WebSocket(this.#signalingUrl + '?href=' + encodeURI(href));
    this.#ws = ws;

    ws.onmessage = async (event) => {
      //console.debug("[DS] ws.onmessage ", event);
      const message = looseJSONParse(event.data);
      const { kind, payload } = message;

      if (kind === "joined") {
        const { iceServers } = payload;
        this.#rtcConfig = { iceServers };
        // and forward
        this.#forward(message);
      } else if (kind === "offer") {
        // start PC and add tracks
        await this.#startRTCOnce();
        // set offer
        const offer = looseJSONParse(payload);
        offer.sdp = processOffer(offer.sdp);
        await this.#pc.setRemoteDescription(offer);
        // add pending candidates
        for (const candidate of this.#pendingCandidates) {
          this.#addIceCandidate(candidate);
        }
        // create and share answer
        const answer = await this.#pc.createAnswer();
        answer.sdp = processAnswer(answer.sdp);
        await this.#pc.setLocalDescription(answer);
        this.#serverSend("client_answer", answer);
        console.debug(`[DS] server offer sdp (length ${payload.length})\n`, offer.sdp);
      } else if (kind === "candidate") {
        const candidate = looseJSONParse(payload);
        if (!this.#pc.remoteDescription) {
          this.#pendingCandidates.push(candidate);
        } else {
          this.#addIceCandidate(candidate);
        }
      } else if (kind === "start") {
        // set encoding parameters
        rampBitrate(this.#pc);
        // unmute
        // stream.getTracks().forEach((track) => {
        //     track.enabled = true;
        // });
        this.#forward(message, true); // force with true since player is not already running
        // Getting peerconnection stats is needed either for stats or debug option
        if (this.#stats || this.#logLevel >= 1) {
          this.#statsIntervalId = setInterval(() => this.#updateStats(), 1000);
        }
      } else if (kind.startsWith("error")) {
        this.#forward(message);
        this.stop(4000);
      } else if (["other_joined", "other_left", "ending", "files", "end"].includes(kind)) {
        // just forward
        this.#forward(message);
      }
    };

    ws.onopen = () => {
      this.#serverSend("join", this.#joinPayload);
    };

    ws.onclose = (event) => {
      console.debug("[DS] ws.onclose ", event);
      this.#forward({ kind: "closed" });
      this.#stopRTC();
    };

    ws.onerror = (event) => {
      console.debug("[DS] ws.onerror ", event);
      this.#forward({ kind: "error", payload: event.data });
      this.stop(4000); // used as error
    };

    setTimeout(() => {
      if (ws.readyState === 0) {
        console.error("[DS] ws can't connect (after 10 seconds)");
      }
    }, 10000);
  }

  #addIceCandidate(candidate) {
    try {
      this.#pc.addIceCandidate(candidate);
      console.debug("[DS] server candidate:", candidate);
    } catch (error) {
      console.error(error);
    }
  }

  async #startRTCOnce() {
    if(this.#startedRTC) return;
    // RTCPeerConnection
    const pc = new RTCPeerConnection(this.#rtcConfig);
    this.#pc = pc;
    console.log("[DS] RTC config: ", this.#rtcConfig);

    // Add local tracks before signaling
    const stream = await navigator.mediaDevices.getUserMedia(this.#constraints);
    this.#stream = stream;
    stream.getTracks().forEach((track) => {
      // implement a mute-like behavior (with `enabled`) until the interaction does start
      // see https://developer.mozilla.org/en-US/docs/Web/API/MediaStreamTrack/enabled
      //track.enabled = false;//disabled for now
      pc.addTrack(track, stream);
    });
    this.#forward({
      kind: "local-stream",
      payload: stream,
    }, true);

    this.#bindPCCallbacks();
    this.#startedRTC = true;
  }

  #bindPCCallbacks = () => {
    const pc = this.#pc;

    pc.onicecandidate = (e) => {
      if (!e.candidate) return;
      console.debug("[DS] ice_candidate " + e.candidate.type + " " + e.candidate.candidate);
      this.#serverSend("client_ice_candidate", e.candidate);
    };

    pc.ontrack = (event) => {
      if (this.#mountEl) {
        let el = document.createElement(event.track.kind);
        el.id = event.track.id;
        el.srcObject = event.streams[0];
        el.autoplay = true;
        if (event.track.kind === "video") {
          if (this.#joinPayload.width) {
            el.style.width = this.#joinPayload.width + "px";
          } else {
            el.style.width = "100%";
          }
          if (this.#joinPayload.height) {
            el.style.height = this.#joinPayload.height + "px";
          }
        }
        this.#mountEl.appendChild(el);
        // on remove
        event.streams[0].onremovetrack = ({ track }) => {
          const el = document.getElementById(track.id);
          if (el) el.parentNode.removeChild(el);
        };
      } else {
        this.#forward({
          kind: "track",
          payload: event,
        });
      }
      console.debug(`[DS] on track (while connection state is ${pc.connectionState})`);
    };

    // for server logging
    if (this.#logLevel >= 2) {
      pc.onconnectionstatechange = () => {
        this.#serverSend("client_connection_state_changed", pc.connectionState);
        console.debug("[DS] onconnectionstatechange:", pc.connectionState);
      };

      // when "stable" -> ICE gathering has complete
      pc.onsignalingstatechange = () => {
        this.#serverSend(
          "client_signaling_state_changed",
          pc.signalingState.toString()
        );
        console.debug("[DS] signaling_state_changed: ", pc.signalingState.toString());
      };

      pc.onnegotiationneeded = (e) => {
        this.#serverSend("client_negotiation_needed", pc.signalingState);
        console.debug("[DS] negotiation_needed: ", pc.signalingState, e);
      };

      pc.oniceconnectionstatechange = () => {
        const state = pc.iceConnectionState;
        console.debug("[DS] ice_connection_state: " + state);
        this.#serverSend("client_ice_connection_state_" + state);

        if (state === "failed") {
          pc.restartIce();
        } else if (state === "connected") {
          // add listeners on first sender (likely the same info to be shared for audio and video)
          const firstSender = pc.getSenders()[0];
          if (firstSender) {
            const { iceTransport } = firstSender.transport;
            if (iceTransport && this.#logLevel >= 2) {
              const pair = iceTransport.getSelectedCandidatePair();
              this.#debugCandidatePair(pair);
              console.debug(
                `[DS] selected candidate pair: client=${pair.local.candidate} server=${pair.remote.candidate}`
              );
            }
          }
        }
      };

      pc.onicegatheringstatechange = () => {
        this.#serverSend(
          "client_ice_gathering_state_changed",
          pc.iceGatheringState.toString()
        );
        console.debug("[DS] ice_gathering_state_changed:", pc.iceGatheringState.toString());
      };

      pc.onicecandidateerror = (e) => {
        this.#serverSend(
          "client_ice_candidate_failed",
          `${e.url}#${e.errorCode}: ${e.errorText}`
        );
        console.debug("[DS] ice_candidate_failed:", `${e.url}#${e.errorCode}: ${e.errorText}`);
      };
    }
  }

  async #updateStats() {
    const pc = this.#pc;
    const pcStats = await pc.getStats();

    if (this.#logLevel >= 1) {
      pcStats.forEach((report) => {
        if (report.type === "outbound-rtp" && report.kind === "video") {
          // encoded size
          let newEncodedWidth = report.frameWidth;
          let newEncodedHeight = report.frameHeight;
          if (
            newEncodedWidth &&
            newEncodedHeight &&
            (newEncodedWidth !== this.#info.encodedWith ||
              newEncodedHeight !== this.#info.encodedHeight)
          ) {
            this.#serverSend(
              "client_video_resolution_updated",
              `${newEncodedWidth}x${newEncodedHeight}`
            );
            this.#info.encodedWith = newEncodedWidth;
            this.#info.encodedHeight = newEncodedHeight;
          }
          // FPS
          let newFramesPerSecond = report.framesPerSecond;
          if (
            typeof newFramesPerSecond !== "undefined" &&
            newFramesPerSecond !== this.#info.framesPerSecond
          ) {
            this.#serverSend("client_video_fps_updated", `${newFramesPerSecond}`);
            this.#info.framesPerSecond = newFramesPerSecond;
          }
          // PLI
          let newPliCount = report.pliCount;
          if (
            typeof newPliCount !== "undefined" &&
            newPliCount !== this.#info.pliCount
          ) {
            this.#serverSend("client_pli_received_count_updated", `${newPliCount}`);
            this.#info.pliCount = newPliCount;
          }
          // FIR
          let newFirCount = report.firCount;
          if (
            typeof newFirCount !== "undefined" &&
            newFirCount !== this.#info.firCount
          ) {
            this.#serverSend("client_fir_received_count_updated", `${newFirCount}`);
            this.#info.firCount = newFirCount;
          }
          // KF
          let newKeyFramesEncoded = report.keyFramesEncoded;
          if (
            typeof newKeyFramesEncoded !== "undefined" &&
            newKeyFramesEncoded !== this.#info.keyFramesEncoded
          ) {
            this.#serverSend(
              "client_keyframe_encoded_count_updated",
              `${newKeyFramesEncoded}`
            );
            this.#info.keyFramesEncoded = newKeyFramesEncoded;
            //console.debug("[DS] encoded KFs", newKeyFramesEncoded);
          }
        }
        if (report.type === "inbound-rtp" && report.kind === "video") {
          // KF
          let newKeyFramesDecoded = report.keyFramesDecoded;
          if (
            typeof newKeyFramesDecoded !== "undefined" &&
            newKeyFramesDecoded !== this.#info.keyFramesDecoded
          ) {
            this.#serverSend(
              "client_keyframe_decoded_count_updated",
              `${newKeyFramesDecoded}`
            );
            this.#info.keyFramesDecoded = newKeyFramesDecoded;
            //console.debug("[DS] decoded KFs", newKeyFramesDecoded);
          }
        }
      });
    }

    if (this.#stats) {
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
      const elapsed = (newNow - this.#info.now) / 1000;
      const audioUp = kbps(
        newAudioBytesSent - this.#info.audioBytesSent,
        elapsed
      );
      const audioDown = kbps(
        newAudioBytesReceived - this.#info.audioBytesReceived,
        elapsed
      );
      const videoUp = kbps(
        newVideoBytesSent - this.#info.videoBytesSent,
        elapsed
      );
      const videoDown = kbps(
        newVideoBytesReceived - this.#info.videoBytesReceived,
        elapsed
      );
      this.#forward({
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

      this.#info = {
        ...this.#info,
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
    await player.start();
    return player;
  },
};
