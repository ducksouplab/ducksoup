import React, { useState, useEffect } from 'react';
import Filter from './filter'
import { randomId } from '../helpers';

export default () => {
    const [allFilters, setAllFilters] = useState([]);
    const [enabledFilters, setEnabledFilters] = useState([]);
    const [active, setActive] = useState("");

    useEffect(async () => {
        const data = await (await fetch("/assets/config/play.json")).json();
        setAllFilters(data.audio);
        setActive(data.audio[0].display);
    }, []);

    const handleAdd = () => {
        const toAdd = allFilters.find((f) => f.display === active);
        if(toAdd) {      
            // important: clone and assign an id      
            setEnabledFilters(f => [...f, { ...toAdd, id: randomId() }]);
        }
    }

    return (
        <>
            <div className="row">
                <div className="col-auto">
                    <select className="form-select" value={active} onChange={e => setActive(e.currentTarget.value)}>
                        {allFilters.map(f => (
                            <option key={f.display}>{f.display}</option>
                        ))}
                    </select>
                </div>
                <div className="col-auto">
                    <button type="button" className="btn" onClick={handleAdd}>add</button>
                </div>
            </div>
            <div className="row">
                {enabledFilters.map(f => (
                    <Filter key={f.id} filter={f} />
                ))}
            </div>
        </>
    );
};