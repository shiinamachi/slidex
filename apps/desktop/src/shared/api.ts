export const enum IPC_CHANNELS {
  appInfo = "app:info",
  slidexStatus = "slidex:status"
}

export type DesktopAppInfo = {
  name: string;
  version: string;
  mode: "development" | "production";
  cliBridge: "not-implemented";
};

export type DesktopPlatform = NodeJS.Platform;

export type SlidexStatus = {
  ready: false;
  reason: string;
};

export type SlidexDesktopAPI = {
  platform: DesktopPlatform;
  getAppInfo: () => Promise<DesktopAppInfo>;
  getSlidexStatus: () => Promise<SlidexStatus>;
};
