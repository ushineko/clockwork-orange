$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path $ScriptDir -Parent
$SourceFile = "$ScriptDir\test_app.py"
$BuildDir = "$ProjectRoot\build_test"
$DistDir = "$ProjectRoot\dist_test"

Write-Host "Building Test App..." -ForegroundColor Cyan
Write-Host "Source: $SourceFile"
Write-Host "Dist: $DistDir"

# Clean previous builds
if (Test-Path $BuildDir) { Remove-Item -Recurse -Force $BuildDir }
if (Test-Path $DistDir) { Remove-Item -Recurse -Force $DistDir }

# Build
# --onedir: Easier to debug than OneFile
# --windowed: No console window (normally, but useful to have console for debugging this test so we see output. user asked for verify it works.)
# actually for *verification* of the test script we WANT console output. 
# But for a real GUI app we usually want --noconsole.
# Let's stick to --console for this verification test so we can see the [TEST] logs.
# If the user wants a GUI build, they'd use --noconsole. 
# I will use --console here to ensure we capture the output.

pyinstaller --noconfirm --onedir --console --clean `
    --name "test_app" `
    --workpath "$BuildDir" `
    --distpath "$DistDir" `
    "$SourceFile"

if ($LASTEXITCODE -eq 0) {
    Write-Host "Build Successful!" -ForegroundColor Green
    Write-Host "Executable: $DistDir\test_app\test_app.exe"
}
else {
    Write-Error "Build Failed!"
    exit 1
}
