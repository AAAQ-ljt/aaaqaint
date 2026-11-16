@echo off
chcp 65001 >nul
title AINT + frpc
setlocal
rem 本脚本放在 deploy 下，aint.exe 与 frpc.exe 放在仓库根目录。
set "ROOT=%~dp0.."
set "AINT_ALLOWED_HOSTS=<你的域名>"
set "URL=http://127.0.0.1:7860"

set "LISTEN=0"
for /f "delims=" %%L in ('netstat -ano ^| findstr ":7860" ^| findstr "LISTENING"') do set "LISTEN=1"
set "FRP=0"
tasklist /FI "IMAGENAME eq frpc.exe" 2>nul | findstr /I "frpc.exe" >nul && set "FRP=1"

if "%LISTEN%"=="0" (
  echo   Starting AINT on 127.0.0.1:7860 ...
  start "AINT" /D "%ROOT%" "%ROOT%\aint.exe" serve --listen 127.0.0.1 --port 7860 --no-browser
)

if "%FRP%"=="0" (
  echo   Starting frpc ...
  start "frpc" /D "%~dp0" "%ROOT%\frpc.exe" -c "%~dp0frpc.toml"
)

echo   Open %URL%
start "" "%URL%"
endlocal
