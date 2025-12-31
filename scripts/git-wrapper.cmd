@echo off
rem Wrapper to allow using git-ssh.ps1 as a drop-in replacement for git.exe in IDEs (VS Code, etc.)
rem Set your "git.path" setting to the full path of this file.

powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0git-ssh.ps1" %*
