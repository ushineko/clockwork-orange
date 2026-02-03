$ErrorActionPreference = "Stop"

Write-Host "Building Watchdog Frozen Test Executable..."
Write-Host ""

# Clean previous test builds
if (Test-Path "dist/test_watchdog_frozen.exe") {
    Remove-Item -Force "dist/test_watchdog_frozen.exe"
}

# Build argument list - same watchdog hidden imports as main build
$PyArgs = @(
    "--noconfirm",
    "--clean",
    "--onefile",
    "--name", "test_watchdog_frozen",
    "--console",
    # Watchdog requires explicit hidden imports for platform-specific observers
    "--hidden-import", "watchdog",
    "--hidden-import", "watchdog.events",
    "--hidden-import", "watchdog.observers",
    "--hidden-import", "watchdog.observers.api",
    "--hidden-import", "watchdog.observers.winapi",
    "--hidden-import", "watchdog.observers.read_directory_changes",
    "--hidden-import", "watchdog.observers.polling",
    "--hidden-import", "watchdog.utils",
    "--hidden-import", "watchdog.utils.bricks",
    "--hidden-import", "watchdog.utils.delayed_queue",
    "--hidden-import", "watchdog.utils.dirsnapshot",
    "--hidden-import", "watchdog.utils.event_debouncer",
    "--hidden-import", "watchdog.utils.patterns",
    "--hidden-import", "watchdog.utils.platform"
)

# Add main script
$PyArgs += "scripts/test_watchdog_frozen.py"

Write-Host "PyInstaller arguments:"
$PyArgs | ForEach-Object { Write-Host "  $_" }
Write-Host ""

# Run PyInstaller
python -m PyInstaller $PyArgs

if ($LASTEXITCODE -eq 0) {
    Write-Host ""
    Write-Host "Build successful!"
    Write-Host "Executable: dist/test_watchdog_frozen.exe"
    Write-Host ""
    Write-Host "Now run the test:"
    Write-Host "  .\dist\test_watchdog_frozen.exe"
    Write-Host ""
}
else {
    Write-Host "Build failed!"
    exit 1
}
