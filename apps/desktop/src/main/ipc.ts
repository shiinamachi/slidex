import { app, ipcMain } from "electron";
import {
  IPC_CHANNELS,
  type DesktopAppInfo,
  type SlidexStatus
} from "../shared/api";

export function registerIpcHandlers(): void {
  ipcMain.handle(IPC_CHANNELS.appInfo, (): DesktopAppInfo => ({
    name: app.getName(),
    version: app.getVersion(),
    mode: app.isPackaged ? "production" : "development",
    cliBridge: "not-implemented"
  }));

  ipcMain.handle(IPC_CHANNELS.slidexStatus, (): SlidexStatus => ({
    ready: false,
    reason: "CLI 연결은 다음 구현 단계에서 연결합니다."
  }));
}
