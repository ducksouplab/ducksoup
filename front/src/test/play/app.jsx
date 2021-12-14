import * as React from "react";
import * as ReactDOM from "react-dom";
import Context from './context';
import Media from "./components/media";
import Table from "./components/table";

const appState = {};

const App = () => {
    return (
        <Context.Provider value={appState}>
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
