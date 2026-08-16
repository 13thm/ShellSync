import { app, BrowserWindow, ipcMain, shell, Tray, Menu, nativeImage } from 'electron'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { ensureDaemon, type DaemonConnection } from './daemon'

let mainWindow: BrowserWindow | null = null
let tray: Tray | null = null
let connection: DaemonConnection | null = null

// 应用图标（窗口 + 托盘共用），开发态位于项目根 resources/ 目录
const iconPath = join(app.getAppPath(), 'resources', 'icon.png')

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
    icon: iconPath,
    webPreferences: {
      preload: join(__dirname, '../preload/index.mjs'),
      sandbox: false,
      contextIsolation: true,
      nodeIntegration: false,
    },
  })

  mainWindow.on('ready-to-show', () => mainWindow?.show())

  // Destroyed windows must be dereferenced, else tray clicks would call
  // methods on a destroyed object ("Object has been destroyed").
  mainWindow.on('closed', () => {
    mainWindow = null
  })

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
  // 从源图缩出托盘小图标（16x16），加载失败时退回空图标
  let trayIcon = nativeImage.createFromPath(iconPath)
  trayIcon = trayIcon.isEmpty() ? nativeImage.createEmpty() : trayIcon.resize({ width: 16, height: 16 })
  tray = new Tray(trayIcon)
  tray.setToolTip('ShellSync')
  const menu = Menu.buildFromTemplate([
    { label: 'ShellSync', enabled: false },
    { type: 'separator' },
    {
      label: '显示主窗口',
      click: () => showMainWindow(),
    },
    {
      label: '退出',
      click: () => app.quit(),
    },
  ])
  tray.setContextMenu(menu)
  tray.on('click', () => showMainWindow())
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

  app.on('activate', () => showMainWindow())
})

/** Show the main window, recreating it if it was closed/destroyed. */
function showMainWindow() {
  if (mainWindow && !mainWindow.isDestroyed()) {
    if (mainWindow.isMinimized()) mainWindow.restore()
    mainWindow.show()
    mainWindow.focus()
  } else {
    createWindow()
  }
}

// Closing the window does NOT stop the daemon (it is detached). On macOS the
// app stays in the dock/tray; elsewhere we just hide on close.
app.on('window-all-closed', () => {
  if (process.platform === 'darwin') return
  // keep the app alive in tray too — user quits via tray menu
})

// fileURLToPath keeps ESM __dirname-style resolution happy in some toolchains.
void fileURLToPath
