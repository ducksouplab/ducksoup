// Init
document.addEventListener("DOMContentLoaded", async () => {
  start();
});

const looseJSONParse = (str) => {
  try {
    return JSON.parse(str);
  } catch (error) {
    console.error(error);
  }
};

const start = () => {
  const wsProtocol = window.location.protocol === "https:" ? "wss" : "ws";
  const pathPrefixhMatch = /(.*)stats/.exec(window.location.pathname);
  // depending on DUCKSOUP_WEB_PREFIX, signaling endpoint may be located at /ws or /prefix/ws
  const pathPrefix = pathPrefixhMatch[1];
  const signalingUrl = `${wsProtocol}://${window.location.host}${pathPrefix}ws?type=stats`;
  const ws = new WebSocket(signalingUrl);

  ws.onclose = (event) => {
    console.error("connection closed");
  };

  ws.onerror = (event) => {
    console.error("connection closed");
  };

  ws.onmessage = async (event) => {
    let interactions = looseJSONParse(event.data).payload;
    document.getElementById("root").innerHTML = JSON.stringify(
      { interactions },
      null,
      2
    );
  };
};
