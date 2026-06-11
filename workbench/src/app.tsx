import { Router } from "@solidjs/router";
import { FileRoutes } from "@solidjs/start/router";
import { Suspense } from "solid-js";

function workbenchBase(): string {
  return window.__SLIDEX_WORKBENCH__?.workbenchBase || "/";
}

export default function App() {
  return (
    <Router base={workbenchBase()} root={(props) => <Suspense>{props.children}</Suspense>}>
      <FileRoutes />
    </Router>
  );
}
