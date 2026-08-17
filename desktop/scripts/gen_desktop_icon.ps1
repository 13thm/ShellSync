# Generate desktop app icons (resources/icon.ico + square icon.png) from ShellSync.png
# 用法：powershell -NoProfile -ExecutionPolicy Bypass -File scripts\gen_desktop_icon.ps1
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing

$desktop  = Split-Path -Parent (Split-Path -Parent $PSCommandPath)   # desktop/
$root     = Split-Path -Parent $desktop                              # repo root
$srcPath  = Join-Path $root 'ShellSync.png'
$outDir   = Join-Path $desktop 'resources'

$sizes = @(256, 128, 64, 48, 32, 16)

$src = [System.Drawing.Image]::FromFile($srcPath)
Write-Host ("src loaded: " + $src.Width + "x" + $src.Height)

try {
    # center the non-square source on a transparent square to avoid distortion
    $side = [int][Math]::Max($src.Width, $src.Height)
    $square = New-Object System.Drawing.Bitmap($side, $side)
    $g = [System.Drawing.Graphics]::FromImage($square)
    $g.Clear([System.Drawing.Color]::Transparent)
    $g.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
    $g.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
    $g.DrawImage($src, [int](($side - $src.Width) / 2), [int](($side - $src.Height) / 2), $src.Width, $src.Height)
    $g.Dispose()

    # render every size to PNG bytes
    $frames = @()
    foreach ($s in $sizes) {
        $bmp = New-Object System.Drawing.Bitmap($s, $s)
        $g2 = [System.Drawing.Graphics]::FromImage($bmp)
        $g2.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
        $g2.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
        $g2.DrawImage($square, 0, 0, $s, $s)
        $g2.Dispose()
        $ms = New-Object System.IO.MemoryStream
        $bmp.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
        $frames += [pscustomobject]@{ Size = $s; Bytes = $ms.ToArray() }
        $bmp.Dispose()
    }

    # assemble a multi-size .ico with PNG-compressed entries (Vista+ format, accepted by electron-builder)
    $ico = New-Object System.IO.MemoryStream
    $bw = New-Object System.IO.BinaryWriter($ico)
    $bw.Write([UInt16]0)                      # reserved
    $bw.Write([UInt16]1)                      # type: icon
    $bw.Write([UInt16]$frames.Count)          # image count
    $offset = 6 + 16 * $frames.Count
    foreach ($f in $frames) {
        $dim = if ($f.Size -ge 256) { 0 } else { $f.Size }   # 0 means 256
        $bw.Write([Byte]$dim)                 # width
        $bw.Write([Byte]$dim)                 # height
        $bw.Write([Byte]0)                    # palette
        $bw.Write([Byte]0)                    # reserved
        $bw.Write([UInt16]1)                  # color planes
        $bw.Write([UInt16]32)                 # bits per pixel
        $bw.Write([UInt32]$f.Bytes.Length)    # data size
        $bw.Write([UInt32]$offset)            # data offset
        $offset += $f.Bytes.Length
    }
    foreach ($f in $frames) { $bw.Write($f.Bytes) }
    $bw.Flush()

    $icoPath = Join-Path $outDir 'icon.ico'
    [System.IO.File]::WriteAllBytes($icoPath, $ico.ToArray())
    Write-Host ("wrote " + $icoPath + " (" + $sizes -join ',' + " px)")

    # also refresh icon.png as a square 256px version (window / tray / mac-linux fallback)
    $pngPath = Join-Path $outDir 'icon.png'
    $frames | Where-Object { $_.Size -eq 256 } | ForEach-Object {
        [System.IO.File]::WriteAllBytes($pngPath, $_.Bytes)
        Write-Host ("wrote " + $pngPath + " (256x256)")
    }

    $square.Dispose()
} finally {
    $src.Dispose()
}
