import React, { useRef } from 'react';
import { randomId } from '../helpers';

export default () => {
    const localVideo = useRef(null);
    const handleDuckSoup = (message) => {
        const { kind, payload } = message;
        if (kind === "local-stream") {
            localVideo.current.srcObject = payload;
        }
    }
    const handleStart = async () => {
        // Init signalingURL with default value
        const wsProtocol = window.location.protocol === "https:" ? "wss" : "ws";
        const pathPrefixhMatch = /(.*)test/.exec(window.location.pathname);
        // depending on DS_WEB_PREFIX, signaling endpoint may be located at /ws or /prefix/ws
        const pathPrefix = pathPrefixhMatch[1];
        const signalingUrl = `${wsProtocol}://${window.location.host}${pathPrefix}ws`;

        await DuckSoup.render({
            callback: handleDuckSoup
        }, {
            signalingUrl,
            namespace: "playground",
            recordingMode: "none",
            size: 1,
            roomId: randomId(),
            userId: randomId(),
            gpu: true,
            videoFormat: "H264",
            duration: 30
        });
    }
    return (
        <div className="media-container">
            <div className="media local">
                <video ref={localVideo} autoPlay muted></video>
            </div>
            <div className="media remote">
                <video autoPlay muted></video>
                <audio muted></audio>
                <div className="control">
                     <div className="play" onClick={handleStart}><span>►</span></div>
                </div>
            </div>
        </div>
    );
};