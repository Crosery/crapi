@echo off
rem crapi installer for cmd.exe (double-click also works).
rem   install.cmd            install, then enter the interactive setup
rem   install.cmd sk-xxx     install and configure with this API key
rem Pure ASCII on purpose: cmd.exe reads .cmd files in the OEM code page.
setlocal
if not "%~1"=="" set "CRAPI_KEY=%~1"
if "%CRAPI_PS1_URL%"=="" set "CRAPI_PS1_URL=https://raw.githubusercontent.com/crosery/crapi/main/install.ps1"
powershell -NoProfile -ExecutionPolicy Bypass -Command "irm '%CRAPI_PS1_URL%' | iex"
echo.
pause
endlocal
