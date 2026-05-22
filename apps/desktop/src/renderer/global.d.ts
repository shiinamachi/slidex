import type { SlidexDesktopAPI } from "../shared/api";

declare global {
  interface Window {
    slidex: SlidexDesktopAPI;
  }
}

declare module "*.png" {
  const value: string;
  export default value;
}

export {};
