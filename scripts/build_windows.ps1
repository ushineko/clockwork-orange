$ErrorActionPreference = "Stop"

Write-Host "Cleaning previous builds..."
if (Test-Path "build") { Remove-Item -Recurse -Force "build" }
if (Test-Path "dist") { Remove-Item -Recurse -Force "dist" }

# Generate version info
try {
    $Version = git describe --tags --always --dirty
    $Branch = git rev-parse --abbrev-ref HEAD
    if ($LASTEXITCODE -ne 0) { throw "Git failed" }
    
    $FullVersion = "$Version-$Branch"
    $FullVersion | Out-File -Encoding ascii "version.txt"
    Write-Host "Detected version: $FullVersion"
}
catch {
    Write-Host "Warning: Could not detect version from git, using 'Unknown'"
    "Unknown" | Out-File -Encoding ascii "version.txt"
}

Write-Host "Building Clockwork Orange for Windows..."

# Build argument list
$PyArgs = @(
    "--noconfirm",
    "--clean",
    "--onefile",
    "--name", "clockwork-orange",
    "--icon", "gui/icons/icon.ico",
    "--add-data", "version.txt;.",
    "--add-data", "img;img",
    "--add-data", "plugins;plugins",
    "--add-data", "gui/icons;gui/icons",
    "--hidden-import", "win32timezone",
    "--hidden-import", "win32service",
    "--hidden-import", "win32serviceutil",
    "--hidden-import", "win32event",
    "--hidden-import", "servicemanager",
    "--collect-all", "gui",
    "--collect-all", "plugins",
    "--collect-all", "watchdog"
)

# Helper to find and add DLLs
function Add-PythonDlls {
    param($ArgsList)
    
    # Get Python Root
    try {
        $PyRoot = python -c "import sys; print(sys.prefix)"
        Write-Host "Python Root determined as: $PyRoot"
    }
    catch {
        Write-Host "Warning: Could not determine Python root. DLL discovery might fail."
        return $ArgsList
    }

    # Files to look for (patterns)
    $Patterns = @("ffi-*.dll", "libssl-*.dll", "libcrypto-*.dll", "sqlite3.dll")
    
    # Search locations: distinct paths to check
    $SearchPaths = @(
        "$PyRoot",
        "$PyRoot\DLLs",
        "$PyRoot\Library\bin", # Conda style
        "$PyRoot\bin"          # Unix/Posix style shim
    ) | Select-Object -Unique

    foreach ($Pattern in $Patterns) {
        $Found = $false
        foreach ($Loc in $SearchPaths) {
            if (Test-Path $Loc) {
                $dllMatches = Get-ChildItem -Path $Loc -Filter $Pattern -File -ErrorAction SilentlyContinue
                if ($dllMatches) {
                    foreach ($File in $dllMatches) {
                        Write-Host "  Found required binary: $($File.FullName)"
                        $ArgsList += "--add-binary", "$($File.FullName);."
                        $Found = $true
                        # Keep only the first match per pattern to avoid duplicates
                        break
                    }
                }
            }
            if ($Found) { break }
        }
        if (-not $Found) {
            Write-Host "  Warning: Could not find binary matching '$Pattern' (Standard PyInstaller discovery will attempt to find it)"
        }
    }
    return $ArgsList
}

# Conditionally add Miniforge binaries only if the environment requires it
# Or rather, ALWAYS try to find them dynamically now.
$PyArgs = Add-PythonDlls $PyArgs

# Add main script
$PyArgs += "clockwork-orange.py"

# Run PyInstaller
python -m PyInstaller $PyArgs

if ($LASTEXITCODE -eq 0) {
    Write-Host "Build successful! Executable is in dist/clockwork-orange.exe"
    Write-Host "This is a DEBUG build (with console window for troubleshooting)"
    
    # Clean up temporary version file
    if (Test-Path "version.txt") { Remove-Item "version.txt" }
}
else {
    Write-Host "Build failed!"
    exit 1
}
