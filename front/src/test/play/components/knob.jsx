import React, { useState } from 'react';
import { Donut } from 'react-dial-knob';

const theme = {
    donutThickness: 10,
    donutColor: '#777',
    bgrColor: '#ccc',
    maxedBgrColor: 'rgb(255, 0, 0)',
    centerColor: '#fff',
    centerFocusedColor: '#eee',
}

export default ({ control }) => {
    const [value, setValue] = useState(control.default);
    return (
        <div className="knob">
            <Donut
            diameter={80}
            min={control.min}
            max={control.max}
            theme={theme}
            step={1}
            value={value}
            onValueChange={setValue}
        >
                <div className="knob-label">{control.display}</div>
            </Donut>
        </div>
    );
};