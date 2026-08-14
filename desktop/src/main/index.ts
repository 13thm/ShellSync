import { app, BrowserWindow, ipcMain, shell, Tray, Menu, nativeImage } from 'electron'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { ensureDaemon, type DaemonConnection } from './daemon'

let mainWindow: BrowserWindow | null = null
let tray: Tray | null = null
let connection: DaemonConnection | null = null

async function bootstrap() {
  try {
    connection = await ensureDaemon()
  } catch (err) {
    // Surface the error in the window so the user knows what went wrong.
    console.error('[daemon]', err)
  }
}

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1180,
    height: 760,
    minWidth: 880,
    minHeight: 600,
    show: false,
    autoHideMenuBar: true,
    title: 'ShellSync',
    backgroundColor: '#F7F8FA',
    webPreferences: {
      preload: join(__dirname, '../preload/index.mjs'),
      sandbox: false,
      contextIsolation: true,
      nodeIntegration: false,
    },
  })

  mainWindow.on('ready-to-show', () => mainWindow?.show())

  mainWindow.webContents.setWindowOpenHandler((details) => {
    shell.openExternal(details.url)
    return { action: 'deny' }
  })

  if (process.env['ELECTRON_RENDERER_URL']) {
    mainWindow.loadURL(process.env['ELECTRON_RENDERER_URL'])
  } else {
    mainWindow.loadFile(join(__dirname, '../renderer/index.html'))
  }
}

function createTray() {
  // a tiny 16x16 transparent-ish icon (no asset bundled yet; use empty image)
  const icon = nativeImage.createEmpty()
  tray = new Tray(icon)
  tray.setToolTip('ShellSync')
  const menu = Menu.buildFromTemplate([
    { label: 'ShellSync', enabled: false },
    { type: 'separator' },
    {
      label: '显示主窗口',
      click: () => {
        if (mainWindow) {
          if (mainWindow.isMinimized()) mainWindow.restore()
          mainWindow.show()
        } else {
          createWindow()
        }
      },
    },
    {
      label: '退出',
      click: () => app.quit(),
    },
  ])
  tray.setContextMenu(menu)
  tray.on('click', () => mainWindow?.show())
}

app.whenReady().then(async () => {
  await bootstrap()

  ipcMain.handle('daemon:connect', async () => {
    // Always re-resolve: the daemon may have restarted with a new port/token
    // since the last call, and ensureDaemon re-reads the lock / respawns it.
    try {
      connection = await ensureDaemon()
    } catch (err) {
      // keep the previous connection (if any) rather than nulling it out
      console.error('[daemon] reconnect failed', err)
    }
    return connection
  })

  createWindow()
  createTray()

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) createWindow()
  })
})

// Closing the window does NOT stop the daemon (it is detached). On macOS the
// app stays in the dock/tray; elsewhere we just hide on close.
app.on('window-all-closed', () => {
  if (process.platform === 'darwin') return
  // keep the app alive in tray too — user quits via tray menu
})

// fileURLToPath keeps ESM __dirname-style resolution happy in some toolchains.
void fileURLToPath
