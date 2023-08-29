const randomId = () =>
  Math.random()
    .toString(36)
    .replace(/[^a-z]+/g, "")
    .substring(0, 8);

const looseJSONParse = (str) => {
  try {
    return JSON.parse(str);
  } catch (error) {
    console.error(error);
  }
};

const state = {};

const start = () => {
  // Signaling
  const { href } = window.location;
  const wsProtocol = window.location.protocol === "https:" ? "wss" : "ws";
  const pathPrefixhMatch = /(.*)test/.exec(window.location.pathname);
  // depending on DUCKSOUP_WEB_PREFIX, signaling endpoint may be located at /ws or /prefix/ws
  const pathPrefix = pathPrefixhMatch[1];
  const signalingUrl = `${wsProtocol}://${window.location.host}${pathPrefix}ws`
  const ws = new WebSocket(signalingUrl + '?href=' + encodeURI(href));

  const send = (kind, payload) => {
    const message = { kind };
    // conditionnally add and possiblty format payload
    if (!!payload) {
      const payloadStr =
        typeof payload === "string" ? payload : JSON.stringify(payload);
      message.payload = payloadStr;
    }
    ws.send(JSON.stringify(message));  
  }

  ws.onmessage = async (event) => {
    //console.debug("[DuckSoup] ws.onmessage ", event);
    const message = looseJSONParse(event.data);
    const { kind, payload } = message;

    if (kind === "joined") {
      const { iceServers } = payload;
      state.iceServers = iceServers;
      console.log(iceServers);
    } else if (kind === "offer") {
      const { iceServers } = state;
      const pc = new RTCPeerConnection({ iceServers });
      const stream = await navigator.mediaDevices.getUserMedia({audio: true});
      
      pc.onicecandidate = (e) => {
        if(!!e.candidate) {
          console.log(`ice_candidate: ${e.candidate.type}: ${e.candidate.candidate}`);
        }
      };

      pc.onconnectionstatechange = () => {
        console.debug("connection_state_change:", pc.connectionState);
      };

      pc.onsignalingstatechange = () => {
        console.debug("signaling_state_changed: ", pc.signalingState.toString());
      };

      pc.onnegotiationneeded = (e) => {
        console.debug("negotiation_needed: ", pc.signalingState, e);
      };

      pc.oniceconnectionstatechange = () => {
        console.debug("ice_connection_state: " + pc.iceConnectionState);
      };

      pc.onicegatheringstatechange = () => {
        console.debug("ice_gathering_state_changed:", pc.iceGatheringState.toString());
      };

      pc.onicecandidateerror = (e) => {
        console.debug("[DS] ice_candidate_failed:", `${e.url}#${e.errorCode}: ${e.errorText}`);
      };

      // Add local tracks before signaling
      stream.getTracks().forEach((track) => {
        pc.addTrack(track, stream);
      });

      // desc = await pc.createOffer({offerToReceiveAudio: 1});
      // pc.setLocalDescription(desc);

      // save to state
      state.pc = pc;
      state.stream = stream;

      const offer = looseJSONParse(payload);
      console.debug(
        `[DuckSoup] server offer sdp (length ${payload.length}):\n${offer.sdp}`
      );

      await pc.setRemoteDescription(offer);
      const answer = await pc.createAnswer();
      await pc.setLocalDescription(answer);
    }
  };

  ws.onopen = () => {
    let joinPayload = {
      "interactionName": randomId(),
      "userId": randomId(),
      "namespace": "test_ice",
      "size": 1,
      "videoFormat": "H264",
      "recordingMode": "muxed",
      "gpu": true
    }
    send("join", joinPayload);
  };

  ws.onclose = () => {
    state.stream.getTracks().forEach((track) => track.stop());
    state.pc.close();
  };

  setTimeout(() => {
    ws.close();
  }, 3000);
};

document.addEventListener("DOMContentLoaded", start);