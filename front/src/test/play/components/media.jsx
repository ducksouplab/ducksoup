import React, { useContext, useRef } from 'react';
import Context from "../context";
import { randomId } from '../helpers';

const getSignalingUrl = () => {
    const wsProtocol = window.location.protocol === "https:" ? "wss" : "ws";
    const pathPrefixhMatch = /(.*)test/.exec(window.location.pathname);
    // depending on DS_WEB_PREFIX, signaling endpoint may be located at /ws or /prefix/ws
    const pathPrefix = pathPrefixhMatch[1];
    return `${wsProtocol}://${window.location.host}${pathPrefix}ws`;
}

const genFxString = (filters) => {
    return filters.reduce((acc, f) => {
        let intro = acc.length === 0 ? "" : "! ";
        intro += `${f.gst} name=${f.id} `;
        const props = f.controls.reduce((acc, c) => {
            return acc + `${c.gst}=${c.current} `;
        }, "");
        return acc + intro + props;
    }, "");
}

const bindStream = (el, stream) => {
    el.srcObject = stream;
    el.muted = false;
    stream.onremovetrack = () => {
        el.pause();
    };
}

export default () => {
    const { dispatch, state: { filters } } = useContext(Context);
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
        }
    }

    const handleStart = async () => {
        dispatch({ type: "isRunning" });

        const ducksoup = await DuckSoup.render({
            callback: handleDuckSoupEvents
        }, {
            signalingUrl: getSignalingUrl(),
            debug: true,
            namespace: "playground",
            recordingMode: "none",
            size: 1,
            roomId: randomId(),
            userId: randomId(),
            gpu: true,
            videoFormat: "H264",
            duration: 30,
            audioFx: genFxString(filters)
        });

        dispatch({ type: "attachPlayer", payload: ducksoup });
    }
    return (
        <div className="media-container">
            <div className="media local">
                <video ref={localVideo} autoPlay muted />
            </div>
            <div className="media remote">
                <video ref={remoteVideo} autoPlay />
                <audio ref={remoteAudio} muted />
                <div className="control">
                     <div className="play" onClick={handleStart}><span>►</span></div>
                </div>
            </div>
        </div>
    );
};