import type { SlidexDesktopAPI } from "../shared/api";

declare global {
  interface Window {
    slidex: SlidexDesktopAPI;
  }
}

export {};
