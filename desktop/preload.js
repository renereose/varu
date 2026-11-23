// What the installer page (installer/wizard.html) may ask of the app.
const { contextBridge, ipcRenderer } = require("electron");

contextBridge.exposeInMainWorld("VaruDesktop", {
  openVaru: (target) => ipcRenderer.send("open-varu", String(target)),
  close: () => ipcRenderer.send("close-installer"),
});
