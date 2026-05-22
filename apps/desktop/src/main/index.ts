import { app, BrowserWindow } from "electron";
import { registerIpcHandlers } from "./ipc";
import { createMainWindow } from "./window";

function focusExistingWindow(): void {
  const [mainWindow] = BrowserWindow.getAllWindows();
  if (!mainWindow) {
    return;
  }
  if (mainWindow.isMinimized()) {
    mainWindow.restore();
  }
  mainWindow.focus();
}

const hasSingleInstanceLock = app.requestSingleInstanceLock();

if (!hasSingleInstanceLock) {
  app.quit();
} else {
  app.on("second-instance", focusExistingWindow);

  app.whenReady().then(async () => {
    registerIpcHandlers();
    await createMainWindow();

    app.on("activate", async () => {
      if (BrowserWindow.getAllWindows().length === 0) {
        await createMainWindow();
      }
    });
  });

  app.on("window-all-closed", () => {
    if (process.platform !== "darwin") {
      app.quit();
    }
  });
}
