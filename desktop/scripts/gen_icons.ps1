# Generate Android launcher icons (all densities) from ShellSync.png
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing

$srcPath = 'E:\code\app\ShellSync\ShellSync.png'
$resDir  = 'E:\code\app\ShellSync\mobile\android\app\src\main\res'

$src = [System.Drawing.Image]::FromFile($srcPath)
Write-Host ("src loaded: " + $src.Width + "x" + $src.Height)

try {
    # center the non-square source on a transparent square to avoid distortion
    $side = [int][Math]::Max([int]$src.Width, [int]$src.Height)
    Write-Host ("side = " + $side)

    $square = New-Object -TypeName System.Drawing.Bitmap -ArgumentList $side, $side
    $g = [System.Drawing.Graphics]::FromImage($square)
    $g.Clear([System.Drawing.Color]::Transparent)
    $g.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
    $g.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
    $g.DrawImage($src, [int](($side - $src.Width) / 2), [int](($side - $src.Height) / 2), $src.Width, $src.Height)
    $g.Dispose()

    $sizes = @{
        'mdpi'    = 48
        'hdpi'    = 72
        'xhdpi'   = 96
        'xxhdpi'  = 144
        'xxxhdpi' = 192
    }
    foreach ($entry in $sizes.GetEnumerator()) {
        $dpi = $entry.Key
        $px  = [int]$entry.Value
        $out = New-Object -TypeName System.Drawing.Bitmap -ArgumentList $px, $px
        $g2 = [System.Drawing.Graphics]::FromImage($out)
        $g2.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
        $g2.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
        $g2.DrawImage($square, 0, 0, $px, $px)
        $g2.Dispose()
        $target = Join-Path $resDir ("mipmap-" + $dpi + "\ic_launcher.png")
        $out.Save($target, [System.Drawing.Imaging.ImageFormat]::Png)
        $out.Dispose()
        Write-Host ("wrote " + $target + " (" + $px + "x" + $px + ")")
    }
    $square.Dispose()
} finally {
    $src.Dispose()
}
