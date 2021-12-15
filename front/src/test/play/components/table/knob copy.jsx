import React, { useContext, useState } from 'react';
import Context from "../../context";
import { Donut } from 'react-dial-knob';

const theme = {
  donutThickness: 10,
  donutColor: '#777',
  bgrColor: '#ccc',
  maxedBgrColor: 'rgb(255, 127, 127)',
  centerColor: '#fff',
  centerFocusedColor: '#f8f8f8',
}

export default ({ filter: { id }, control }) => {
  const { dispatch } = useContext(Context);
  const [value, setValue] = useState(control.default);
  const handleValueChange = (v) => {
    if(v !== value) {
      dispatch({ type: "newControlValue", payload: { id, gst: control.gst, value: v } })
      setValue(v);
    }
  }
  return (
    <div className="knob">
      <Donut
        diameter={80}
        min={control.min}
        max={control.max}
        theme={theme}
        step={control.step}
        value={value}
        onValueChange={handleValueChange}
      >
        <div className="knob-label">{control.display || control.gst}</div>
      </Donut>
    </div>
  );
};