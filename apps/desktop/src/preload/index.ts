import { contextBridge, ipcRenderer } from "electron";
import { IPC_CHANNELS, type SlidexDesktopAPI } from "../shared/api";

const api: SlidexDesktopAPI = {
  getAppInfo: () => ipcRenderer.invoke(IPC_CHANNELS.appInfo),
  getSlidexStatus: () => ipcRenderer.invoke(IPC_CHANNELS.slidexStatus)
};

contextBridge.exposeInMainWorld("slidex", api);
