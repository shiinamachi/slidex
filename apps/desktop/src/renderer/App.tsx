import type { ReactElement } from "react";
import { createRootRoute, createRouter } from "@tanstack/react-router";

function EmptyDesktopRoot(): ReactElement {
  return <main className="desktop-root" aria-label="slidex Desktop" />;
}

const rootRoute = createRootRoute({
  component: EmptyDesktopRoot
});

export const router = createRouter({
  routeTree: rootRoute
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
