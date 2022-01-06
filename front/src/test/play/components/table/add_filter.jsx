import React, { useState, useEffect, useContext } from 'react';
import Context from "../../context";

const filterOptions = (type, groupedFilters) => {
    if (!groupedFilters) return null;
    const categories = Object.keys(groupedFilters);
    if (!categories) return null;

    return categories.map(c => (
        <optgroup key={c} label={c}>
            {groupedFilters[c].map(f => (
                <option key={f.display}>{f.display}</option>
            ))}
        </optgroup>
    ));
}

export default ({ type }) => {
    const { dispatch, state: { started, flatFilters, groupedAudioFilters, groupedVideoFilters } } = useContext(Context);
    const groupedFilters = type === "audio" ? groupedAudioFilters : groupedVideoFilters;
    const [activeDisplay, setActiveDisplay] = useState("");

    useEffect(() => {
        if (flatFilters) {
            setActiveDisplay(flatFilters.filter(({ type: t }) => t === type)[0].display);
        }
    }, [flatFilters]);

    const handleAdd = () => {
        dispatch({ type: "addFilter", payload: activeDisplay });
    }

    return !started && (
        <>
            <label className="col-auto col-form-label"><span className="type">{ type }</span> filters</label>
            <div className="col-auto">
                <select className="form-select" value={activeDisplay} onChange={e => setActiveDisplay(e.currentTarget.value)}>
                    {filterOptions(type, groupedFilters)}
                </select>
            </div>
            <div className="col-auto">
                <button type="button" className="btn btn-secondary" onClick={handleAdd}>add</button>
            </div>
        </>
    );
};