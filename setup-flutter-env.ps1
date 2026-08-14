$flutter = 'D:\Program Files\flutter\bin'
$p = [Environment]::GetEnvironmentVariable('Path','User')
if ($null -eq $p) { $p = '' }
if (($p -split ';') -contains $flutter) {
    Write-Output 'PATH-already-contains-flutter'
} else {
    $new = if ($p) { "$p;$flutter" } else { $flutter }
    [Environment]::SetEnvironmentVariable('Path', $new, 'User')
    Write-Output 'PATH-appended-flutter'
}
[Environment]::SetEnvironmentVariable('PUB_HOSTED_URL','https://pub.flutter-io.cn','User')
[Environment]::SetEnvironmentVariable('FLUTTER_STORAGE_BASE_URL','https://storage.flutter-io.cn','User')
Write-Output ('PUB_HOSTED_URL=' + [Environment]::GetEnvironmentVariable('PUB_HOSTED_URL','User'))
Write-Output ('FLUTTER_STORAGE_BASE_URL=' + [Environment]::GetEnvironmentVariable('FLUTTER_STORAGE_BASE_URL','User'))
$check = [Environment]::GetEnvironmentVariable('Path','User')
Write-Output ('flutter-in-PATH=' + ($check -like '*flutter\bin*'))
