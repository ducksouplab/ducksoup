import React, { useContext } from 'react';
import Context from "../../context";
import AddFilter from "./add_filter";
import Filter from './filter'

export default () => {
    const { dispatch, state: { filters } } = useContext(Context);
    return (
        <div className="container">
            <div className="row">
                <AddFilter />
            </div>
            <div className="row">
                <div className="col">
                    {filters.map(f => (
                        <Filter key={f.id} filter={f} />
                    ))}
                </div>
            </div>
        </div>
    );
};