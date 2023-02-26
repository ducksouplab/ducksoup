import React, { useContext, useRef } from "react";
import Context from "../context";
import { randomId, getSignalingUrl, genFxString } from "../helpers";

const bindStream = (el, stream) => {
  el.srcObject = stream;
  el.muted = false;
  stream.onremovetrack = () => {
    el.pause();
  };
};

export default () => {
  const {
    dispatch,
    state: { enabledFilters, started, record, duration },
  } = useContext(Context);
  const localVideo = useRef(null);
  const remoteVideo = useRef(null);
  const remoteAudio = useRef(null);

  const handleDuckSoupEvents = (message) => {
    const { kind, payload } = message;
    if (kind === "local-stream") {
      localVideo.current.srcObject = payload;
    } else if (kind === "track") {
      const { track, streams } = payload;
      if (track.kind === "video") {
        bindStream(remoteVideo.current, streams[0]);
      } else {
        bindStream(remoteAudio.current, streams[0]);
      }
      // on remove
      streams[0].onremovetrack = ({ track }) => {
        const el = document.getElementById(track.id);
        if (el) el.parentNode.removeChild(el);
      };
    } else if (kind === "closed" || kind === "error") {
      dispatch({ type: "stop" });
      localVideo.current.srcObject = null;
      remoteVideo.current.srcObject = null;
      remoteAudio.current.srcObject = null;
    }
  };

  const handleStart = async () => {
    dispatch({ type: "start" });

    const audioFx = genFxString(enabledFilters, "audio");
    const videoFx = genFxString(enabledFilters, "video");

    console.log(audioFx);
    console.log(videoFx);

    const ducksoup = await DuckSoup.render(
      {
        callback: handleDuckSoupEvents,
      },
      {
        signalingUrl: getSignalingUrl(),
        debug: true,
        namespace: "play",
        duration: duration,
        recordingMode: record ? "muxed" : "none",
        size: 1,
        interactionName: randomId(),
        userId: randomId(),
        gpu: true,
        videoFormat: "H264",
        audioFx,
        videoFx,
      }
    );

    dispatch({ type: "attachPlayer", payload: ducksoup });
  };

  const handleStop = async () => {
    dispatch({ type: "stop" });
  };

  const handleToggleRecord = async () => {
    dispatch({ type: "toggleRecord" });
  };

  const handleDuration = async (e) => {
    dispatch({ type: "setDuration", payload: parseInt(e.target.value, 10) });
  };

  return (
    <div className="media-container">
      <div className="media local">
        <video ref={localVideo} autoPlay muted />
      </div>
      <div className="arrow">➜</div>
      <div className="link">︷</div>
      <div className="media remote">
        <video ref={remoteVideo} autoPlay />
        <audio ref={remoteAudio} muted />
        <div className="controls">
          {started ? (
            <div className="stop" onClick={handleStop}>
              <span></span>
            </div>
          ) : (
            <>
              <div className="record" onClick={handleToggleRecord}>
                {record ? <span>●</span> : <span>◌</span>}
              </div>
              <div className="input-group input-group-sm">
                <input
                  type="text"
                  className="form-control"
                  value={duration}
                  onChange={handleDuration}
                />
                <span className="input-group-text">sec</span>
              </div>
              <div className="play" onClick={handleStart}>
                <span>►</span>
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  );
};
