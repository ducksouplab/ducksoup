import React, { useState, useEffect, useContext } from 'react';
import Context from "../../context";

export default () => {
    const { dispatch, state: { running, allFilters } } = useContext(Context);
    const [activeDisplay, setActiveDisplay] = useState("");

    useEffect(() => {
        if (allFilters) {
            setActiveDisplay(allFilters[0].display);
        }
    }, [allFilters]);

    const handleAdd = () => {
        dispatch({ type: "addFilter", payload: activeDisplay });
    }

    return !running && (
        <>
            <div className="col-auto">
                <select className="form-select" value={activeDisplay} onChange={e => setActiveDisplay(e.currentTarget.value)}>
                    {allFilters && allFilters.map(f => (
                        <option key={f.display}>{f.display}</option>
                    ))}
                </select>
            </div>
            <div className="col-auto">
                <button type="button" className="btn" onClick={handleAdd}>add</button>
            </div>
        </>
    );
};