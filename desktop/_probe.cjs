// 在 Electron 渲染进程环境里诊断 WS：加载实际页面，抓 console 日志
const { app, BrowserWindow } = require('electron')
app.commandLine.appendSwitch('ignore-certificate-errors')
app.whenReady().then(async () => {
  const win = new BrowserWindow({ show: false, webPreferences: { offscreen: true } })
  win.webContents.on('console-message', (_e, _lvl, msg) => {
    if (/ws|daemon|lock|error|closed|connect/i.test(msg)) console.log('[page]', msg.slice(0, 200))
  })
  const url = process.argv[2]
  await win.loadURL(url)
  setTimeout(() => { console.log('--- done'); app.exit(0) }, 12000)
})
