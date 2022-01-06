import React, { useEffect, useReducer } from 'react';
import * as ReactDOM from "react-dom";
import Context, { reducer, initialState } from "./context";
import Media from "./components/media";
import Table from "./components/table";

const App = () => {
  const [state, dispatch] = useReducer(reducer, initialState);

  useEffect(async () => {
      const flatFilters = await (await fetch("/assets/config/play.json")).json();
      dispatch({ type: "setFilters", payload: flatFilters });
  }, []);

  return (
    <Context.Provider value={{ state, dispatch }}>
      <Media />
      <div className="container">
        <div className="row">
          <div className="col flex-row-reverse">
              <Table type="audio"/>
          </div>
          <div className="col">
              <Table type="video"/>
          </div>
        </div>
      </div>
    </Context.Provider>
  );
}

document.addEventListener("DOMContentLoaded", async () => {
  ReactDOM.render(
    <App />,
    document.getElementById("root")
  );
});
