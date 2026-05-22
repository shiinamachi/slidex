export const IPC_CHANNELS = {
  appInfo: "app:info",
  slidexStatus: "slidex:status"
} as const;

export type DesktopAppInfo = {
  name: string;
  version: string;
  mode: "development" | "production";
  cliBridge: "not-implemented";
};

export type SlidexStatus = {
  ready: false;
  reason: string;
};

export type SlidexDesktopAPI = {
  getAppInfo: () => Promise<DesktopAppInfo>;
  getSlidexStatus: () => Promise<SlidexStatus>;
};
