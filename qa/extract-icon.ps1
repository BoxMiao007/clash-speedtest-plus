Add-Type -AssemblyName System.Drawing
$exe = Join-Path (Get-Location) 'clash-speedtest-plus.exe'
$icon = [System.Drawing.Icon]::ExtractAssociatedIcon($exe)
if ($null -eq $icon) { Write-Output 'NO-ICON'; exit 1 }
$bitmap = $icon.ToBitmap()
$out = Join-Path (Get-Location) 'qa/shots/icon-from-exe.png'
$bitmap.Save($out, [System.Drawing.Imaging.ImageFormat]::Png)
Write-Output ("OK {0}x{1} -> {2}" -f $bitmap.Width, $bitmap.Height, $out)
