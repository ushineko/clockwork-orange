$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path $ScriptDir -Parent
$SourceFile = "$ScriptDir\research_service.py"
$BuildDir = "$ProjectRoot\build_test_service"
$DistDir = "$ProjectRoot\dist_test_service"

Write-Host "Building Test Service..." -ForegroundColor Cyan
Write-Host "Source: $SourceFile"

if (Test-Path $BuildDir) { Remove-Item -Recurse -Force $BuildDir }
if (Test-Path $DistDir) { Remove-Item -Recurse -Force $DistDir }

# Service builds need detailed hidden imports
# Also typically need --uac-admin if it were installing itself, but we can restart powershell as admin if needed.
# For now just building.
# IMPORTANT: Services must be consoleless? Actually debugging services often easier with console? 
# Standard practice: No console for services, but for testing let's stick with console (though SCM hides it).
# However, for PyInstaller services, usually we want --noconsole OR it doesn't matter much as SCM runs it in background.
# But `servicemanager` logic requires it.

pyinstaller --noconfirm --onedir --clean --noconsole `
    --name "ClockworkOrangeTestService" `
    --workpath "$BuildDir" `
    --distpath "$DistDir" `
    --hidden-import=win32timezone `
    --hidden-import=win32service `
    --hidden-import=win32serviceutil `
    --hidden-import=servicemanager `
    "$SourceFile"

if ($LASTEXITCODE -eq 0) {
    Write-Host "Build Successful!" -ForegroundColor Green
    Write-Host "Executable: $DistDir\ClockworkOrangeTestService\ClockworkOrangeTestService.exe"
}
else {
    Write-Error "Build Failed!"
    exit 1
}
