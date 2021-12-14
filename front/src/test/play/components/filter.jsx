import React from 'react';
import Knob from './knob';

export default ({ filter }) => {
    return (
        <div className="filter">
            <div className="filter-label">{filter.display}</div>
            <div className="filter-knobs">
                { filter.controls.map((c) => (
                    <Knob key={c.display} control={c} />
                ))}       
            </div>
        </div>
    );
};