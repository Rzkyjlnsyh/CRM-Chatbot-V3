@echo off
REM Wrapper Windows (CMD / double-click) → launcher lintas platform
setlocal
cd /d "%~dp0\.."

where node >nul 2>nul
if errorlevel 1 (
  echo Node.js belum terpasang atau tidak ada di PATH.
  echo Install dari https://nodejs.org/ lalu buka ulang terminal.
  exit /b 1
)

node "%~dp0dev.mjs" %*
exit /b %ERRORLEVEL%
