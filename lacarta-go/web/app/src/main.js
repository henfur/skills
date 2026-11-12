import { mount } from "svelte";
import "./app.css";
import App from "./App.svelte";
import { start } from "./lib/state.svelte.js";

// The theme chosen with the Theme button wins over the system's.
try {
  const t = localStorage.getItem("lacarta-theme");
  if (t) document.documentElement.dataset.theme = t;
} catch (e) {}

mount(App, { target: document.getElementById("app") });
start();
