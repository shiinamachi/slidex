import { mount, StartClient } from "@solidjs/start/client";

export default function start() {
  const root = document.getElementById("app");
  if (!root) throw new Error("Workbench root element is unavailable");
  root.replaceChildren();
  mount(() => <StartClient />, root);
}

start();
