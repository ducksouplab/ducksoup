// Search for "DS mod" in comments to see why
// https://github.com/francoisgeorgy/svg-knob/ has not been used as is

const palettes = {
  light: {
    bg_color: "#E0E0E0",
    bg_border_color: "#BDBDBD",
    track_bg_color: "#CFD8DC",
    track_color_init: "#64B5F6",
    track_color: "#42A5F5",
    cursor_color_init: "#64B5F6",
    cursor_color: "#42A5F5",
    markers_color: "#9E9E9E",
    font_color: "#424242",
  },
  light2: {
    bg_color: "#B1DAEE",
    bg_border_color: "#569DC0",
    track_bg_color: "#B1DAEE",
    track_color_init: "#569DC0",
    track_color: "#1D6D93",
    cursor_color_init: "#569DC0",
    cursor_color: "#1D6D93",
    markers_color: "#3680A4",
    font_color: "#1D6D93",
  },
  dark: {
    bg_color: "#000000",
    bg_border_color: "#569DC0",
    track_bg_color: "#424242",
    track_color_init: "#FDD835",
    track_color: "#FFEC00",
    cursor_color_init: "#569DC0",
    cursor_color: "#FDD835",
    markers_color: "#3680A4",
    font_color: "#FFEA00",
  }
};

// FIXME: remove console usage for IE compatibility

/**
 *
 * @param elem DIV or SVN element
 * @param conf optional config
 * @returns {{value, config}}
 */
export default function (elem, conf = {}) {

  // Like a real knob, it's the knob's position that determines the knob's value.
  // Therefore, the value is computed from the knob's position (angle).
  // However, the user has the possibility to directly set the value and in that case
  // the knob's position (angle) will be computed from the value and the knob redrawn
  // accordingly.

  //
  // All angles in method parameters are in [degrees] (except for polarToKnobAngle() and getViewboxCoord()).
  //
  // By default:
  // - knob direction is CLOCKWISE
  // - start position is 6 o'clock (bottom)
  // - knob angle is:
  //       0 [deg] angle is a   6 o'clock (bottom)
  //      90 [deg] angle is at  9 o'clock (left)
  //     180 [deg] angle is at 12 o'clock (top)
  //     270 [deg] angle is at  3 o'clock (right)
  //
  // Trigonometric functions (sin, cos, ...) operate in polar coordinates,
  // with 0 angle at 3 o'clock and a counter-clockwise direction.
  // To convert from "knob angle" to "polar angle":
  //
  //     knob    polar
  // --------------------
  //        0      270
  //       30      240
  //       90      180
  //      180       90
  //      270        0
  //      330      -60 (add 360 to get a positive equivalent value: -60 + 360 = 300)
  //
  // Formula: polar-angle = 270 - knob-angle
  //

  if (!elem) {
    throw "You must pass a DOM node reference to the Knob constructor";
  }

  let trace = false;    // when true, will log more details in the console; use enableDebug(), disableDebug() to change

  // It is faster to access a property than to access a variable...
  // See https://jsperf.com/vars-vs-props-speed-comparison/1

  const NS = "http://www.w3.org/2000/svg";
  const CW = true;    // clock-wise
  const CCW = !CW;    // counter clock-wise

  //---------------------------------------------------------------------
  // To simplify the internal coordinates transformations, we set the view box as a 100 by 100 square.
  // But, if a label is present, then we add 20 to the height (at the bottom) as a placeholder for the label.
  // In summary:
  // - 0,0..99,99: the knob itself
  // - 0,100..99,119: the label, if any
  const VIEWBOX_WIDTH = 100;
  // const VIEWBOX_HEIGHT = config.with_label ? 120 : 100;
  const HALF_WIDTH = 50;      // viewBox/2
  const HALF_HEIGHT = 50;     // viewBox/2

  let svg_element;

  if (typeof elem === "string" || elem instanceof String) {
    elem = document.querySelector(elem);
  }

  if (elem.nodeName.toLowerCase() === "svg") {
    svg_element = elem;
  } else {
    svg_element = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    elem.appendChild(svg_element);
  }

  // For the user convenience, the label can be set with the "data-label" attribute.
  // If another label is set in data-config then this later definition will override data-label.
  // let default_label = svg_element.dataset.label !== undefined ? svg_element.dataset.label : "";
  let label = elem.dataset.label !== undefined ? elem.dataset.label : false;

  let defaults = {

    // User configurable properties. The colors are defined in the 'palettes', later on.

    // No camelCase because we want to be able to have the same name in data- attributes.

    label: false,

    rotation: CW,

    default_value: 0,
    initial_value: 0,
    value_min: 0.0,
    value_max: 100.0,
    value_resolution: 1,        // null means ignore

    // split knob:
    center_zero: false,
    center_value: null,         // if null, the value will be computed from the min and max in the init() method
    center_gap: 4,              // only used when center_zero=true; is the width of the gap between the left and right track around the zero value.

    // position:
    zero_at: 270.0,             // [deg] (polar) the 0 degree will be at 270 polar degrees (6 o'clock).
    angle_min: 30.0,            // [deg] Angle in knob coordinates (0 at 6 0'clock)
    angle_max: 330.0,           // [deg] Angle in knob coordinates (0 at 6 0'clock)

    // background disk:
    bg_radius: 32,
    bg_border_width: 1,

    // track background:
    track_bg_radius: 40,
    track_bg_width: 8,

    // track:
    track_radius: 40,
    track_width: 8,

    // cursor
    cursor_radius: 18,          // same unit as radius
    cursor_length: 10,
    cursor_width: 4,

    // appearance:
    palette: "light",
    bg: false,
    track_bg: true,
    track: true,
    cursor: false,
    // CSS class names
    linecap: "butt",                   // https://developer.mozilla.org/en-US/docs/Web/SVG/Attribute/stroke-linecap

    // text displayed in the middle of the knob:
    value_text: true,
    value_position: HALF_HEIGHT + 8,    // empirical value: HALF_HEIGHT + config.font_size / 3
    font_family: "sans-serif",
    font_size: 25,

    // callback to get the text to display from the current value
    display_raw: false,                 // if true, format callback is ignored
    format_raw: v => Math.round(v),
    format: v => v,                     // formatting of the displayed value
    // off_text: null,                     // text to display when raw_value = min
    // out_of_range_text: null,            // text to display when raw_value is out of range

    font_weight: "bold",
    markers: 0,                         // number of markers; 0 or false to disable
    markers_radius: 40,
    markers_length: 8,
    markers_width: 2,

    class_bg: "knob-bg",
    class_track_bg: "knob-track-bg",
    class_track: "knob-track",
    class_value: "knob-value",
    class_cursor: "knob-cursor",
    class_markers: "knob-markers",

    snap_to_steps: false,       // TODO

    // mouse wheel support:
    mouse_wheel_acceleration: 1,

    onchange: null              // callback function
  };

  //---------------------------------------------------------------------
  // Consolidate all configs:

  let data_config = JSON.parse(elem.dataset.config || "{}");
  let c = Object.assign({}, defaults, palettes[defaults.palette], conf, data_config);
  // we re-assign conf and data_config for the case they override some of the palette colors.
  let config = Object.assign(c, palettes[c.palette], conf, data_config);

  //---------------------------------------------------------------------
  // Terminates the SVG element setup:

  let viewbox_height;
  if (config.label || (config.value_position >= (100 - (config.font_size / 2)))) {
    // make some room for the label or the value that we want to display below the knob
    viewbox_height = 120;
  } else {
    viewbox_height = 100;
  }

  // For the use of null argument with setAttributeNS, see https://developer.mozilla.org/en-US/docs/Web/SVG/Namespaces_Crash_Course#Scripting_in_namespaced_XML
  svg_element.setAttributeNS("http://www.w3.org/2000/xmlns/", "xmlns:xlink", "http://www.w3.org/1999/xlink");
  svg_element.setAttributeNS(null, "viewBox", `0 0 ${VIEWBOX_WIDTH} ${viewbox_height}`);

  // Center of arc in knob coordinates and in ViewPort"s pixels relative to the <svg> ClientBoundingRect.
  let arcCenterXPixels = 0;
  let arcCenterYPixels = 0; // equal to arcCenterXPixels because the knob is a circle

  //---------------------------------------------------------------------
  // Pre-computed values to speed-up operations:

  // At the top of the knob, we leave a gap between the left and right tracks.
  // These are angles that delimit this gap:
  let left_track_end_angle = 0;     // angle in [degrees]
  let right_track_start_angle = 0;  // angle in [degrees]

  //---------------------------------------------------------------------
  // internals

  let value = 0.0;                    // current knob's value [value_min..value_max]
  let angle = config.angle_min;       // current knob's angle in [deg] and in knob's coordinate (not polar)

  let distance = 0.0;                 // distance from arc center to mouse position
  let mouse_wheel_direction = 1;      // dependant of the OS

  //---------------------------------------------------------------------
  // SVG elements, from back to front:
  let svg_bg = null;           // background disk:
  let svg_track_bg = null;            // track background; for non zero-centered knobs
  let svg_track_bg_left = null;       // track background; for zero-centered knobs
  let svg_track_bg_right = null;      // track background; for zero-centered knobs
  let svg_track = null;
  let svg_cursor = null;
  let svg_divisions = null;
  let svg_value_text = null;
  let svg_value_text2 = null;

  //---------------------------------------------------------------------
  // mouse support
  let targetRect;
  let minDeltaY;

  //---------------------------------------------------------------------
  // true if the current knob value is different from the default value
  let has_changed = false;    // to spare some getValue() calls when testing if value has changed from default_value

  //---------------------------------------------------------------------
  // Create the knob:

  init();
  initValue();
  draw();
  attachEventHandlers();


  /**
   * Having a init function allow the knob to be re-configured.
   */
  function init() {

    if (trace) console.log("init()");

    if (config.center_zero) {
      if (!config.center_value) {
        config.center_value = getRoundedValue((config.value_max - config.value_min) / 2 + config.value_min);
      }
    }

    // At the top of the knob, we leave a gap between the left and right tracks.
    // 'left_track_end_angle' and 'right_track_start_angle' are the angles that delimit this gap.
    // Only used if center_zero=true.
    if (config.linecap === "butt") {
      left_track_end_angle = polarToKnobAngle(Math.acos(-config.center_gap / 100.0) * 180.0 / Math.PI);
      right_track_start_angle = polarToKnobAngle(Math.acos(config.center_gap / 100.0) * 180.0 / Math.PI);
    } else {
      left_track_end_angle = polarToKnobAngle(Math.acos(-(config.track_width * 1.3 + config.center_gap) / 100.0) * 180.0 / Math.PI);
      right_track_start_angle = polarToKnobAngle(Math.acos((config.track_width * 1.3 + config.center_gap) / 100.0) * 180.0 / Math.PI);
    }

    // mouse_wheel_direction = _isMacOS() ? -1 : 1; //TODO: really necessary?
  }

  function initValue() {
    // set initial value and angle:
    setValue(config.initial_value ? config.initial_value : config.default_value);
  }

  /**
   * Return the value "rounded" according to config.value_resolution
   * @param v value
   */
  function getRoundedValue(v) {
    return config.value_resolution === null ? v : Math.round(v / config.value_resolution) * config.value_resolution;
  }

  /**
   *
   * @param angle [deg] in knob's coordinates
   * @returns {*}
   */
  function getDisplayValue(angle) {
    if (trace) console.log("getDisplayValue", config.display_raw);
    let v = getValue(angle);
    return config.display_raw ? config.format_raw(v) : config.format(v);
  }

  /**
   * Trick to adjust the cursor position when the range is odd.
   */
  function getCursorCorrection() {
    let isOdd = n => Math.abs(n % 2) === 1;
    return (isOdd(Math.abs(config.value_max - config.value_min)) ? 0.5 : 0) * config.value_resolution; // DS mod
  }

  /**
   * Get the knob's value determined by the knob's position (angle)
   * @param a [deg] in knob's coordinates
   * @returns {number}
   */
  function getValue(a) {
    let p = a === undefined ? angle : a;
    let v = ((p - config.angle_min) / (config.angle_max - config.angle_min)) * (config.value_max - config.value_min) + config.value_min;
    return getRoundedValue(v - getCursorCorrection());
  }

  /**
   * Set knob's value
   * @param v
   */
  function setValue(v) {
    if (v < config.value_min) {
      value = config.value_min;
    } else if (v > config.value_max) {
      value = config.value_max;
    } else {
      value = v;
    }
    setAngle(((v + getCursorCorrection() - config.value_min) / (config.value_max - config.value_min)) * (config.angle_max - config.angle_min) + config.angle_min);
    if (trace) console.log(`setValue(${v}) angle=` + ((v - config.value_min) / (config.value_max - config.value_min)) * (config.angle_max - config.angle_min) + config.angle_min);
    return true;
  }

  /**
   * Set knob's angle
   * @param new_angle in [deg]
   * @param fire_event
   */
  function setAngle(new_angle, fire_event) {
    let prev = angle;
    let notify = fire_event && (new_angle !== angle);
    angle = Math.min(Math.max(new_angle, config.angle_min), config.angle_max);
    if (notify) {
      // fire the event if the change of angle affect the value:
      if (getValue(prev) !== getValue()) {
        notifyChange();
      }
    }
  }

  /**
   * Increment (or decrement if the increment is negative) the knob's angle.
   * @param increment
   */
  function incAngle(increment) {
    setAngle(Math.min(Math.max(angle + increment, config.angle_min), config.angle_max), true);
  }

  /**
   * Return polar coordinates angle from our "knob coordinates" angle
   */
  function knobToPolarAngle(angle) {
    let a = config.zero_at - angle;
    if (a < 0) a = a + 360.0;
    if (trace) console.log(`knobToPolarAngle ${angle} -> ${a}`);
    return a;
  }

  /**
   *
   * @param angle [deg] with 0 at 3 o'clock
   * @returns {number}
   */
  function polarToKnobAngle(angle) {
    // "-" for changing CCW to CW
    if (trace) console.log(`polarToKnobAngle ${angle} -> ${(config.zero_at - angle + 360.0) % 360.0}`);
    return (config.zero_at - angle + 360.0) % 360.0;    // we add 360 to handle negative values down to -360
  }

  /**
   * startDrag() must have been called before to init the targetRect variable.
   */
  function mouseUpdate(e) {

    // MouseEvent.clientX (standard property: YES)
    // The clientX read-only property of the MouseEvent interface provides
    // the horizontal coordinate within the application's client area at which
    // the event occurred (as opposed to the coordinates within the page).
    // For example, clicking in the top-left corner of the client area will always
    // result in a mouse event with a clientX value of 0, regardless of whether
    // the page is scrolled horizontally. Originally, this property was defined
    // as a long integer. The CSSOM View Module redefined it as a double float.

    let dxPixels = e.clientX - targetRect.left;
    let dyPixels = e.clientY - targetRect.top;

    // mouse delta in cartesian coordinate with path center=0,0 and scaled (-1..0..1) relative to path:
    // <svg> center:       (dx, dy) == ( 0,  0)
    // <svg> top-left:     (dx, dy) == (-1,  1)
    // <svg> bottom-right: (dx, dy) == ( 1, -1) (bottom right of the 100x100 viewBox, ignoring the bottom 100x20 for the label)
    let dx = (dxPixels - arcCenterXPixels) / (targetRect.width / 2);
    let dy = - (dyPixels - arcCenterYPixels) / (targetRect.width / 2);  // targetRect.width car on a 20px de plus en hauteur pour le label

    if (config.rotation === CCW) dx = - dx;

    // convert to polar coordinates
    let angle_rad = Math.atan2(dy, dx);
    if (angle_rad < 0) angle_rad = 2.0 * Math.PI + angle_rad;

    if (trace) console.log(`mouseUpdate: position in svg = ${dxPixels}, ${dyPixels} pixels; ${dx.toFixed(3)}, ${dy.toFixed(3)} rel.; angle ${angle_rad.toFixed(3)} rad`);

    setAngle(polarToKnobAngle(angle_rad * 180.0 / Math.PI), true);

    // distance from arc center to mouse position:
    // distance = Math.sqrt(dx*(HALF_WIDTH/config.track_radius)*dx*(HALF_WIDTH/config.track_radius) + dy*(HALF_HEIGHT/config.track_radius)*dy*(HALF_HEIGHT/config.track_radius));
  }

  /**
   *
   * @param e
   */
  function startDrag(e) {

    if (trace) console.log("startDrag");

    e.preventDefault();

    // API: Event.currentTarget
    //      Identifies the current target for the event, as the event traverses the DOM. It always REFERS TO THE ELEMENT
    //      TO WHICH THE EVENT HANDLER HAS BEEN ATTACHED, as opposed to event.target which identifies the element on
    //      which the event occurred.
    //      https://developer.mozilla.org/en-US/docs/Web/API/Event/currentTarget

    // currentTarget = e.currentTarget;

    // API: Element.getBoundingClientRect() (standard: YES)
    //      The Element.getBoundingClientRect() method returns the size of an element
    //      and its POSITION RELATIVE TO THE VIEWPORT.
    //      The amount of scrolling that has been done of the viewport area (or any other
    //      scrollable element) is taken into account when computing the bounding rectangle.
    //      This means that the rectangle's boundary edges (top, left, bottom, and right)
    //      change their values every time the scrolling position changes (because their
    //      values are relative to the viewport and not absolute).
    //      https://developer.mozilla.org/en/docs/Web/API/Element/getBoundingClientRect

    // targetRect = currentTarget.getBoundingClientRect(); // currentTarget must be the <svg...> object
    targetRect = svg_element.getBoundingClientRect();

    // Note: we must take the boundingClientRect of the <svg> and not the <path> because the <path> bounding rect
    //       is not constant because it encloses the current arc.

    // By design, the arc center is at equal distance from top and left.
    arcCenterXPixels = targetRect.width / 2;
    //noinspection JSSuspiciousNameCombination
    arcCenterYPixels = arcCenterXPixels;

    document.addEventListener("mousemove", handleDrag, false);
    document.addEventListener("mouseup", endDrag, false);

    mouseUpdate(e);
    redraw();
  }

  /**
   *
   * @param e
   */
  function handleDrag(e) {
    e.preventDefault();
    mouseUpdate(e);
    redraw();
  }

  /**
   *
   */
  function endDrag() {
    if (trace) console.log("endDrag");
    document.removeEventListener("mousemove", handleDrag, false);
    document.removeEventListener("mouseup", endDrag, false);
  }

  /**
   *
   * @param e
   * @returns {boolean}
   */
  function mouseWheelHandler(e) {

    // WheelEvent
    // This is the standard wheel event interface to use. Old versions of browsers implemented the two non-standard
    // and non-cross-browser-compatible MouseWheelEvent and MouseScrollEvent interfaces. Use this interface and avoid
    // the latter two.
    // The WheelEvent interface represents events that occur due to the user moving a mouse wheel or similar input device.

    // https://stackoverflow.com/questions/5527601/normalizing-mousewheel-speed-across-browsers
    // https://github.com/facebook/fixed-data-table/blob/master/src/vendor_upstream/dom/normalizeWheel.js

    e.preventDefault();

    let dy = e.deltaY;

    if (dy !== 0) {
      // normalize Y delta
      if (minDeltaY > Math.abs(dy) || !minDeltaY) {
        minDeltaY = Math.abs(dy);
      }
    }

    incAngle(dy / minDeltaY * mouse_wheel_direction * config.mouse_wheel_acceleration);

    // TODO: mouse speed detection (https://stackoverflow.com/questions/22593286/detect-measure-scroll-speed)

    redraw();

    return false;
  }

  /**
   *
   * @param e
   */
  function startTouch(e) {

    if (trace) console.log("startTouch");

    e.preventDefault(); // necessary to avoid moving all the page

    targetRect = svg_element.getBoundingClientRect();

    // By design, the arc center is at equal distance from top and left.
    arcCenterXPixels = targetRect.width / 2;
    //noinspection JSSuspiciousNameCombination
    arcCenterYPixels = arcCenterXPixels;

    document.addEventListener("touchmove", handleTouch, { passive: false });
    document.addEventListener("touchend", endTouch);

  }

  /**
   *
   * @param e
   */
  function handleTouch(e) {

    if (trace) console.log("handleTouch", e.touches);

    e.preventDefault();

    let touchesIndex = e.touches.length - 1;

    let dxPixels = e.touches[touchesIndex].clientX - targetRect.left;
    let dyPixels = e.touches[touchesIndex].clientY - targetRect.top;

    let dx = (dxPixels - arcCenterXPixels) / (targetRect.width / 2);
    let dy = - (dyPixels - arcCenterYPixels) / (targetRect.width / 2);  // targetRect.width car on a 20px de plus en hauteur pour le label

    if (config.rotation === CCW) dx = - dx;

    // convert to polar coordinates
    let angle_rad = Math.atan2(dy, dx);
    if (angle_rad < 0) angle_rad = 2.0 * Math.PI + angle_rad;

    if (trace) console.log(`handleTouch: position in svg = ${dxPixels}, ${dyPixels} pixels; ${dx.toFixed(3)}, ${dy.toFixed(3)} rel.; angle ${angle_rad.toFixed(3)} rad`);

    setAngle(polarToKnobAngle(angle_rad * 180.0 / Math.PI), true);

    redraw();

  }

  /**
   *
   */
  function endTouch() {
    if (trace) console.log("endTouch");
    document.removeEventListener("touchmove", handleTouch);
    document.removeEventListener("touchend", endTouch);
  }

  /**
   *
   */
  function attachEventHandlers() {
    if (trace) console.log("attach attachEventHandlers");
    svg_element.addEventListener("mousedown", function (e) {
      startDrag(e);
    });
    svg_element.addEventListener("wheel", function (e) {
      mouseWheelHandler(e);
    });
    svg_element.addEventListener("touchstart", startTouch, { passive: false });
  }

  /**
   *
   */
  function notifyChange() {
    if (trace) console.log("knob value has changed");
    let value = getValue();     // TODO: cache the value
    let event = new CustomEvent("change", { "detail": value });
    //svg_element.dispatchEvent(event);
    elem.dispatchEvent(event);
    if (config.onchange) {
      config.onchange(value);
    }
  }

  /**
   * Utility function to configure the mousewheel direction.
   * @returns {*}
   * @private
   */
  function _isMacOS() {
    return ["Macintosh", "MacIntel", "MacPPC", "Mac68K"].indexOf(window.navigator.platform) !== -1;
  }

  /**
   * Return viewBox X,Y coordinates
   * @param angle in [degree] (polar, 0 at 3 o'clock)
   * @param radius; defaults to config.radius
   * @returns {{x: number, y: number}}
   */
  function getViewboxCoord(angle, radius) {
    let a = angle * Math.PI / 180.0;
    let r = radius === undefined ? config.track_radius : radius;
    let x = Math.cos(a) * r;
    let y = Math.sin(a) * r;
    return {
      x: config.rotation === CW ? (HALF_WIDTH + x) : (HALF_WIDTH - x),
      y: HALF_HEIGHT - y
    }
  }

  /**
   *
   * @param from_angle in [degree] in knob's coordinates
   * @param to_angle in [degree] in knob's coordinates
   * @param radius
   */
  function getArc(from_angle, to_angle, radius) {

    if (trace) console.group(`getArc(${from_angle}, ${to_angle}, ${radius})`);

    // SVG d: "A rx,ry xAxisRotate LargeArcFlag,SweepFlag x,y".
    // SweepFlag is either 0 or 1, and determines if the arc should be swept in a clockwise (1), or anti-clockwise (0) direction
    // ref: https://developer.mozilla.org/en-US/docs/Web/SVG/Attribute/d

    let a0 = knobToPolarAngle(from_angle);
    let a1 = knobToPolarAngle(to_angle);

    // little trick to force a full arc (360deg) when from=0 and to=360
    if (from_angle !== to_angle) {
      // with this we make sure that x1 will be different than x0 within the path definition
      a0 -= 0.0001;
      a1 += 0.0001;
    }

    let { x: x0, y: y0 } = getViewboxCoord(a0, radius);
    let { x: x1, y: y1 } = getViewboxCoord(a1, radius);

    let delta_angle = (a0 - a1 + 360.0) % 360.0;

    let large_arc = delta_angle < 180.0 ? 0 : 1;
    let arc_direction = config.rotation === CW ? 1 : 0;

    let p = `M ${x0},${y0} A ${radius},${radius} 0 ${large_arc},${arc_direction} ${x1},${y1}`;

    if (trace) console.groupEnd();
    if (trace) console.log("arc: " + p);

    return p;
  }

  /**
   *
   * @returns {*}
   */
  function getTrackPath() {

    if (trace) console.log("getTrackPath()");

    let p = null;

    if (config.center_zero) {

      if (Array.isArray(config.center_value)) {
        // let v = getValue();
        // console.log('center value is an array; getValue=', getValue(), typeof v);
        if (config.center_value.includes(getValue())) {
          if (trace) console.log("getTrackPath: center position, track not drawn");
          // track is not drawn when the value is at center
          return p;
        }
      } else {
        if (getValue() === config.center_value) {
          if (trace) console.log("getTrackPath: center position, track not drawn");
          // track is not drawn when the value is at center
          return p;
        }
      }

      // we assume the split is at 180 [deg] (knob"s angle)
      if (angle < 180) {
        p = getArc(Math.min(angle, left_track_end_angle), left_track_end_angle, config.track_radius);
      } else if (angle > 180) {
        p = getArc(right_track_start_angle, Math.max(angle, right_track_start_angle), config.track_radius);
      }

    } else {
      p = getArc(config.angle_min, angle, config.track_radius);
    }

    return p;
  }

  /**
   *
   */
  function draw_background() {

    if (trace) console.log("draw_background()", config.bg);

    if (!config.bg) return;

    // For the use of null argument with setAttributeNS, see https://developer.mozilla.org/en-US/docs/Web/SVG/Namespaces_Crash_Course#Scripting_in_namespaced_XML

    //
    // back disk:
    //
    svg_bg = document.createElementNS(NS, "circle");
    svg_bg.setAttributeNS(null, "cx", `${HALF_WIDTH}`);
    svg_bg.setAttributeNS(null, "cy", `${HALF_HEIGHT}`);
    svg_bg.setAttributeNS(null, "r", `${config.bg_radius}`);
    svg_bg.setAttribute("fill", `${config.bg_color}`);
    svg_bg.setAttribute("stroke", `${config.bg_border_color}`);
    svg_bg.setAttribute("stroke-width", `${config.bg_border_width}`);
    svg_bg.setAttribute("class", config.class_bg);
    svg_element.appendChild(svg_bg);
  }

  /**
   *
   */
  function draw_markers() {

    if (trace) console.log("draw_markers()", config.markers);

    if (!config.markers) return;

    let p = "";
    let step = (config.angle_max - config.angle_min) / config.markers;
    for (let a = config.angle_min; a <= config.angle_max; a += step) {
      let from = getViewboxCoord(knobToPolarAngle(a), config.markers_radius);    // getViewboxCoord(angle, radius)
      let to = getViewboxCoord(knobToPolarAngle(a), config.markers_radius + config.markers_length);
      p += `M ${from.x},${from.y} L ${to.x},${to.y} `;
    }

    svg_divisions = document.createElementNS(NS, "path");
    svg_divisions.setAttributeNS(null, "d", p);
    svg_divisions.setAttribute("stroke", `${config.markers_color}`);
    svg_divisions.setAttribute("stroke-width", `${config.markers_width}`);
    svg_divisions.setAttribute("stroke-linecap", config.linecap);
    svg_divisions.setAttribute("class", config.class_markers);
    svg_element.appendChild(svg_divisions);
  }

  /*
          function draw_units() {
              let pos = getViewboxCoord(angle_min_polar, config.divisions_radius);    // getViewboxCoord(angle, radius)
              svg_value_text = document.createElementNS(NS, "text");
              svg_value_text.setAttributeNS(null, "x", `${pos.x}`);
              svg_value_text.setAttributeNS(null, "y", `${pos.y}`);
              // svg_value_text.setAttribute("text-anchor", "middle");
              svg_value_text.setAttribute("cursor", "default");
              svg_value_text.setAttribute("font-family", config.font_family);
              svg_value_text.setAttribute("font-size", `10`);
              // svg_value_text.setAttribute("font-weight", `${config.font_weight}`);
              svg_value_text.setAttribute("fill", config.font_color);
              // svg_value_text.setAttribute("class", config.class_value);
              // svg_value_text.textContent = getDisplayValue();
              svg_value_text.textContent = config.value_min.toString();
              svg_element.appendChild(svg_value_text);
          }
  */

  /**
   *
   */
  function draw_track_background() {

    if (trace) console.log("draw_track_background()", config.track_bg);

    // For the use of null argument with setAttributeNS, see https://developer.mozilla.org/en-US/docs/Web/SVG/Namespaces_Crash_Course#Scripting_in_namespaced_XML

    if (!config.track_bg) return;

    //
    // track background:
    //
    if (config.center_zero) {

      // left track background
      svg_track_bg_left = document.createElementNS(NS, "path");
      svg_track_bg_left.setAttributeNS(null, "d", getArc(config.angle_min, left_track_end_angle, config.track_bg_radius));
      svg_track_bg_left.setAttribute("stroke", `${config.track_bg_color}`);
      svg_track_bg_left.setAttribute("stroke-width", `${config.track_bg_width}`);
      svg_track_bg_left.setAttribute("stroke-linecap", config.linecap);
      svg_track_bg_left.setAttribute("fill", "transparent");
      svg_track_bg_left.setAttribute("class", config.class_track_bg);
      svg_element.appendChild(svg_track_bg_left);

      // right track background
      svg_track_bg_right = document.createElementNS(NS, "path");
      svg_track_bg_right.setAttributeNS(null, "d", getArc(right_track_start_angle, config.angle_max, config.track_bg_radius));
      svg_track_bg_right.setAttribute("stroke", `${config.track_bg_color}`);
      svg_track_bg_right.setAttribute("stroke-width", `${config.track_bg_width}`);
      svg_track_bg_right.setAttribute("stroke-linecap", config.linecap);
      svg_track_bg_right.setAttribute("fill", "transparent");
      svg_track_bg_right.setAttribute("class", config.class_track_bg);
      svg_element.appendChild(svg_track_bg_right);

    } else {

      svg_track_bg = document.createElementNS(NS, "path");
      svg_track_bg.setAttributeNS(null, "d", getArc(config.angle_min, config.angle_max, config.track_bg_radius));
      svg_track_bg.setAttribute("stroke", `${config.track_bg_color}`);
      svg_track_bg.setAttribute("stroke-width", `${config.track_bg_width}`);
      svg_track_bg.setAttribute("fill", "transparent");
      svg_track_bg.setAttribute("stroke-linecap", config.linecap);
      svg_track_bg.setAttribute("class", config.class_track_bg);
      svg_element.appendChild(svg_track_bg);

    }
  }

  /**
   *
   */
  function draw_track() {

    if (trace) console.log("draw_track()", config.track);

    if (!config.track) return;
    let p = getTrackPath();
    if (p) {
      svg_track = document.createElementNS(NS, "path");
      svg_track.setAttributeNS(null, "d", p);
      svg_track.setAttribute("stroke", `${config.track_color_init}`);
      svg_track.setAttribute("stroke-width", `${config.track_width}`);
      svg_track.setAttribute("fill", "transparent");
      svg_track.setAttribute("stroke-linecap", config.linecap);
      svg_track.setAttribute("class", config.class_track);
      svg_element.appendChild(svg_track);
    }
  }

  /**
   *
   * @returns {string}
   */
  function getTrackCursor() {
    let a = knobToPolarAngle(angle);
    let from = getViewboxCoord(a, config.cursor_radius);
    let to = getViewboxCoord(a, config.cursor_radius + config.cursor_length);
    return `M ${from.x},${from.y} L ${to.x},${to.y}`;
  }

  /**
   *
   */
  function draw_cursor() {

    if (trace) console.log("draw_cursor()", config.cursor);

    if (!config.cursor) return;

    let p = getTrackCursor();
    if (p) {
      svg_cursor = document.createElementNS(NS, "path");
      svg_cursor.setAttributeNS(null, "d", p);
      svg_cursor.setAttribute("stroke", `${config.cursor_color_init}`);
      svg_cursor.setAttribute("stroke-width", `${config.cursor_width}`);
      svg_cursor.setAttribute("fill", "transparent");
      svg_cursor.setAttribute("stroke-linecap", config.linecap);
      svg_cursor.setAttribute("class", config.class_cursor);
      svg_element.appendChild(svg_cursor);
    }
  }

  function getFontSize(text, multiline = false) {
    if (typeof text !== "string") return config.font_size;
    const f = 22;   // empiric value
    let m = Math.min(f, text.length);
    if (multiline) {
      return config.font_size * Math.min(0.75, Math.max(((f - m) / f), 0.25));
    } else {
      return config.font_size * Math.max(((f - m) / f), 0.25);
    }

  }

  function draw_value_line1(text, multiline = false) {
    if (trace) console.log("draw_value_line1", text);
    svg_value_text = document.createElementNS(NS, "text");
    svg_value_text.setAttributeNS(null, "x", `${HALF_WIDTH}`);
    svg_value_text.setAttributeNS(null, "y", `${config.value_position}`);
    svg_value_text.setAttribute("text-anchor", "middle");
    svg_value_text.setAttribute("cursor", "default");
    svg_value_text.setAttribute("font-family", config.font_family);
    svg_value_text.setAttribute("font-size", `${getFontSize(text, multiline)}`);
    svg_value_text.setAttribute("font-weight", `${config.font_weight}`);
    svg_value_text.setAttribute("fill", config.font_color);
    svg_value_text.setAttribute("class", config.class_value);
    svg_value_text.textContent = text;
    svg_element.appendChild(svg_value_text);
  }

  function draw_value_line2(text) {
    if (trace) console.log("draw_value_line2", text);
    svg_value_text2 = document.createElementNS(NS, "text");
    svg_value_text2.setAttributeNS(null, "x", `${HALF_WIDTH}`);
    svg_value_text2.setAttributeNS(null, "y", `${config.value_position + 4}`);
    svg_value_text2.setAttribute("text-anchor", "middle");
    svg_value_text2.setAttribute("cursor", "default");
    svg_value_text2.setAttribute("font-family", config.font_family);
    svg_value_text2.setAttribute("font-size", `${getFontSize(text, true)}`);
    svg_value_text2.setAttribute("font-weight", `${config.font_weight}`);
    svg_value_text2.setAttribute("fill", config.font_color);
    svg_value_text2.setAttribute("class", config.class_value);
    svg_value_text2.textContent = text;
    svg_element.appendChild(svg_value_text2);

  }

  /**
   *
   */
  function draw_value() {

    if (trace) console.log("draw_value", config.value_text);

    if (!config.value_text) return;

    const t = getDisplayValue();

    if (typeof t === "number" || t.indexOf("\n") <= 0) {

      if (trace) console.log("single line");
      draw_value_line1(t)

    } else {
      const lines = t.split("\n");
      if (trace) console.log("two lines", lines);

      draw_value_line1(lines[0], true);
      draw_value_line2(lines[1]);
    }

  }

  function update_value() {

    if (trace) console.log("update_value", config.value_text);

    if (!config.value_text) return;

    const t = getDisplayValue();

    if (typeof t === "number" || t.indexOf("\n") <= 0) {
      // single line:

      if (trace) console.log("single line");

      if (svg_value_text) {
        svg_value_text.setAttributeNS(null, "y", `${config.value_position}`);
        svg_value_text.setAttribute("font-size", `${getFontSize(t)}`);
        svg_value_text.textContent = t;
      }

      if (svg_value_text2) {
        svg_element.removeChild(svg_value_text2);
        svg_value_text2 = null;
      }

    } else {
      // two lines:

      const lines = t.split("\n");
      if (trace) console.log("two lines", t, lines);

      if (svg_value_text) {
        svg_value_text.setAttributeNS(null, "y", `${config.value_position - 12}`);
        svg_value_text.setAttribute("font-size", `${getFontSize(lines[0], true)}`);
        svg_value_text.textContent = lines[0];
      }

      if (svg_value_text2) {
        svg_value_text2.setAttributeNS(null, "y", `${config.value_position + 4}`);
        svg_value_text2.setAttribute("font-size", `${getFontSize(lines[1], true)}`);
        svg_value_text2.textContent = lines[1];
      } else {
        draw_value_line2(lines[1]);
      }
    }

  }

  /**
   *
   */
  function draw() {

    if (trace) console.log("draw()", config);

    draw_background();
    draw_track_background();
    draw_markers();
    // draw_units();
    draw_track();
    draw_cursor();
    draw_value();
  }

  /**
   *
   */
  function redraw() {

    if (trace) console.log("redraw()", config);

    let p = getTrackPath();
    if (p) {
      if (svg_track) {
        if (trace) console.log("redraw track already exist, update d");
        svg_track.setAttributeNS(null, "d", p);
      } else {
        if (trace) console.log("redraw draw_track");
        draw_track();
      }
    } else {
      if (svg_track) {
        if (trace) console.log("redraw track already exist, hide it");
        svg_track.setAttributeNS(null, "d", "");    // we hide the track
      }
    }

    if (!has_changed) {
      has_changed = getValue() !== config.default_value;
      if (has_changed) {
        if (svg_track) {
          svg_track.setAttribute("stroke", `${config.track_color}`);
        }
      }
    }

    if (svg_cursor) {
      p = getTrackCursor();
      if (p) {
        if (trace) console.log("redraw cursor");
        svg_cursor.setAttributeNS(null, "d", p);
        if (has_changed) {
          svg_cursor.setAttribute("stroke", `${config.cursor_color}`);
        }
      }
    }

    if (svg_value_text) {
      if (trace) console.log("redraw svg_value_text");
      // svg_value_text.textContent = getDisplayValue();
      update_value();
    }
  }

  /**
   *
   */
  return {
    set value(v) {
      // console.group("value setter");
      setValue(v);
      redraw();
      // console.groupEnd();
    },
    set config(new_config) {
      // console.group("config setter");
      config = Object.assign({}, defaults, config, new_config);
      init();
      redraw();
      // console.groupEnd();
    },
    setConfigValue: function (attribute, value) {
      // console.group("setConfigValue");
      config[attribute] = value;
      init();
      redraw();
      // console.groupEnd();
    },
    initValue,
    enableDebug: function () {
      trace = true;
    },
    disableDebug: function () {
      trace = false;
    }
  };

}