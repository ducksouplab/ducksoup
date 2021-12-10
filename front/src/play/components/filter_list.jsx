import React, { useState, useEffect } from 'react';

export default () => {
    const [filters, setFilters] = useState();
    const [active, setActive] = useState("");
    useEffect(async () => {
        const data = await (await fetch("/assets/config/play.json")).json();
        setFilters(data.audio);
        setActive(data.audio[0].display);
    }, []);

    return (
        <div className="row">
            <div className="col-auto">
                <select className="form-select" value={active} onChange={e => setActive(e.currentTarget.value)}>
                    {filters && filters.map(f => (
                        <option key={f.display}>{f.display}</option>
                        ))} 
                </select>
            </div>
            <div className="col-auto">
                <button type="button" className="btn" onClick={() => console.log(active)}>add</button>
            </div>
        </div>
    );
};