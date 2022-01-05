import React, { useState, useEffect, useContext } from 'react';
import Context from "../../context";

const filterOptions = (groupedFilters) => {
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

export default () => {
    const { dispatch, state: { started, flatFilters, groupedFilters } } = useContext(Context);
    const [activeDisplay, setActiveDisplay] = useState("");

    useEffect(() => {
        if (flatFilters) {
            setActiveDisplay(flatFilters[0].display);
        }
    }, [flatFilters]);

    const handleAdd = () => {
        dispatch({ type: "addFilter", payload: activeDisplay });
    }

    return !started && (
        <>
            <div className="col-auto">
                <select className="form-select" value={activeDisplay} onChange={e => setActiveDisplay(e.currentTarget.value)}>
                    {filterOptions(groupedFilters)}
                </select>
            </div>
            <div className="col-auto">
                <button type="button" className="btn" onClick={handleAdd}>add</button>
            </div>
        </>
    );
};