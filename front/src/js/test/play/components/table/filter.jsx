import React, { useContext } from "react";
import Context from "../../context";
import Knob from "./knob";

export default ({ filter }) => {
  const { dispatch } = useContext(Context);

  const handleRemove = () => {
    if (window.confirm(`Delete ${filter.display} filter?`)) {
      dispatch({ type: "removeFilter", payload: filter.id });
    }
  };

  return (
    <div className="filter">
      <div className="filter-label">{filter.display}</div>
      <div className="filter-remove" onClick={handleRemove}>
        ✕
      </div>
      {filter.url && (
        <a href={filter.url} target="_blank">
          <div className="filter-help">?</div>
        </a>
      )}
      {filter.controls ? (
        <div className="filter-knobs">
          {filter.controls.map((c) => (
            <Knob key={c.gst} control={c} filter={filter} />
          ))}
        </div>
      ) : (
        <div className="no-knobs">no control</div>
      )}
    </div>
  );
};
