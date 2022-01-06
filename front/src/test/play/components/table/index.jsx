import React, { useContext } from 'react';
import Context from "../../context";
import AddFilter from "./add_filter";
import Filter from './filter'

export default ({ type }) => {
    const { state: { enabledFilters } } = useContext(Context);
    const tableClassName = `table-container table-${type}`
    return (
        <div className={tableClassName}>
            <div className="row add-filter-row">
                <AddFilter type={type}/>
            </div>
            <div className="row filters-row">
                <div className="col">
                    {enabledFilters.filter(({ type: t }) => t === type).map(f => (
                        <Filter key={f.id} filter={f} />
                    ))}
                </div>
            </div>
        </div>
    );
};