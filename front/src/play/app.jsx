import * as React from "react";
import * as ReactDOM from "react-dom";
import Media from "./components/media";
import Table from "./components/table";

const App = () => {
    return (<>
        <Media />
        <Table />
    </>
    );
}

document.addEventListener("DOMContentLoaded", async () => {
    ReactDOM.render(
        <App />,
        document.getElementById("root")
    );
});
