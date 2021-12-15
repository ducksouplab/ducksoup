import React, { useEffect, useReducer } from 'react';
import * as ReactDOM from "react-dom";
import Context, { reducer, initialState } from "./context";
import Media from "./components/media";
import Table from "./components/table";

const App = () => {
  const [state, dispatch] = useReducer(reducer, initialState);

  useEffect(async () => {
      const allFilters = await (await fetch("/assets/config/play.json")).json();
      dispatch({ type: "setAllFilters", payload: allFilters.audio });
  }, []);

  return (
    <Context.Provider value={{ state, dispatch }}>
      <Media />
      <Table />
    </Context.Provider>
  );
}

document.addEventListener("DOMContentLoaded", async () => {
  ReactDOM.render(
    <App />,
    document.getElementById("root")
  );
});
