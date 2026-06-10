@echo off
setlocal
slidex doctor --codex --render --json %*
exit /b %ERRORLEVEL%
