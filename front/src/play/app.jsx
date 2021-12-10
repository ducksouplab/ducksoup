import * as React from "react";
import * as ReactDOM from "react-dom";
import FilterList from "./components/filter_list";

const App = () => {
    return (<FilterList />);
}

document.addEventListener("DOMContentLoaded", async () => {
    ReactDOM.render(
        <App />,
        document.getElementById("root")
    );
});
