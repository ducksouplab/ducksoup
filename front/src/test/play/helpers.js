const randomLengthString = () =>
  Math.random()
    .toString(36)
    .replace(/[^a-z]+/g, "");

const randomId = () => randomLengthString() + randomLengthString();

const getSignalingUrl = () => {
  const wsProtocol = window.location.protocol === "https:" ? "wss" : "ws";
  const pathPrefixhMatch = /(.*)test/.exec(window.location.pathname);
  // depending on DUCKSOUP_WEB_PREFIX, signaling endpoint may be located at /ws or /prefix/ws
  const pathPrefix = pathPrefixhMatch[1];
  return `${wsProtocol}://${window.location.host}${pathPrefix}ws`;
};

const genFxString = (filters, type) => {
  const convert = type === "audio" ? "audioconvert" : "videoconvert";

  return filters
    .filter(({ type: t }) => t === type)
    .reduce((acc, f) => {
      let intro = acc.length === 0 ? "" : `! ${convert} ! `;
      intro += `${f.gst} name=${f.id} `;
      const fixedProps = f.fixed
        ? f.fixed.reduce((acc, c) => {
            return acc + `${c.gst}=${c.value} `;
          }, "")
        : "";
      const props = f.controls
        ? f.controls.reduce((acc, c) => {
            return acc + `${c.gst}=${c.current} `;
          }, "")
        : "";
      return acc + intro + fixedProps + props;
    }, "");
};

export { randomId, getSignalingUrl, genFxString };
