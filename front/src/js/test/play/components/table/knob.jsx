import React, { useContext, useEffect, useRef, useState } from "react";
import Context from "../../context";
import SvgKnob from "./lib/svg-knob";

const getPrecision = (num) => {
  const split = num.toString().split(".");
  if (split.length < 2) return 0;
  return split[1].length;
};

const round = (value, precision) => {
  if (precision === 0) return value;
  return parseFloat(Number.parseFloat(value).toFixed(precision));
};

const displayValue = (value, control) => {
  if (typeof control.displayDivider === "undefined") return value;
  let output = value / control.displayDivider;
  if (typeof control.displayDigits === "undefined") return output;
  return output.toFixed(control.displayDigits);
};

export default ({ filter: { id }, control }) => {
  const { dispatch } = useContext(Context);
  const [value, setValue] = useState(control.current);
  const svg = useRef(null);
  const precision = getPrecision(control.step);

  const handleValueChange = ({ detail: v }) => {
    dispatch({
      type: "newControlValue",
      payload: { id, gst: control.gst, kind: control.kind, value: v },
    });
    setValue(round(v, precision));
  };

  useEffect(() => {
    new SvgKnob(svg.current, {
      display_raw: true,
      value_text: false,
      center_zero: false,
      initial_value: control.current,
      value_min: control.min,
      value_max: control.max,
      value_resolution: control.step,
    });
    svg.current.addEventListener("change", handleValueChange);
  }, []);

  return (
    <div className="knob">
      <svg ref={svg} />
      <div className="knob-value">{displayValue(value, control)}</div>
      <div className="knob-label">
        <div>{control.display || control.gst}</div>
      </div>
    </div>
  );
};
