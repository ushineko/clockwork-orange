$ErrorActionPreference = "Stop"

Write-Host "Cleaning previous builds..."
if (Test-Path "build") { Remove-Item -Recurse -Force "build" }
if (Test-Path "dist") { Remove-Item -Recurse -Force "dist" }

Write-Host "Building Clockwork Orange for Windows (Production - No Console)..."
pyinstaller --noconfirm --clean --onefile --noconsole --name "clockwork-orange" `
    --add-data "img;img" `
    --add-data "plugins;plugins" `
    --add-data "gui/icons;gui/icons" `
    --hidden-import "win32timezone" `
    --hidden-import "win32service" `
    --hidden-import "win32serviceutil" `
    --hidden-import "win32event" `
    --hidden-import "servicemanager" `
    --add-binary "C:\Users\nvere\miniforge3\Library\bin\ffi-8.dll;." `
    --add-binary "C:\Users\nvere\miniforge3\python313.dll;." `
    --add-binary "C:\Users\nvere\miniforge3\Library\bin\sqlite3.dll;." `
    --add-binary "C:\Users\nvere\miniforge3\Library\bin\libssl-3-x64.dll;." `
    --add-binary "C:\Users\nvere\miniforge3\Library\bin\libcrypto-3-x64.dll;." `
    --collect-all "gui" `
    --collect-all "plugins" `
    "clockwork-orange.py"

if ($LASTEXITCODE -eq 0) {
    Write-Host "Build successful! Executable is in dist/clockwork-orange.exe"
    Write-Host "This is a PRODUCTION build (no console window)"
}
else {
    Write-Host "Build failed!"
    exit 1
}
