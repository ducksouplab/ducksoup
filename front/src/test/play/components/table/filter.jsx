import React from 'react';
import Knob from './knob';

export default ({ filter }) => {
    return (
        <div className="filter">
            <div className="filter-label">{filter.display}</div>
            { filter.url && (
                <a href={filter.url} target="_blank">
                    <div className="filter-help">?</div>
                </a>
            )}
            <div className="filter-knobs">
                {filter.controls.map((c) => (
                    <Knob key={c.gst} control={c} filter={filter} />
                ))}
            </div>
        </div>
    );
};