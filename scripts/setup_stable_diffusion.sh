#!/bin/bash
# Setup script for Clockwork Orange Stable Diffusion Plugin

set -e

APP_DATA_DIR="$HOME/.local/share/clockwork-orange"
VENV_DIR="$APP_DATA_DIR/venv-sd"

# Pin to Python 3.13 — Python 3.14 has segfaults with scipy/openblas
PYTHON_BIN="python3.13"

echo "========================================================"
echo "  Clockwork Orange - Stable Diffusion Setup"
echo "========================================================"
echo "Installation Target: $VENV_DIR"
echo "Python: $PYTHON_BIN"
echo ""
echo "This script will create an isolated virtual environment and install"
echo "the necessary dependencies for local AI image generation."
echo "WARNING: This requires approx 2-4GB of disk space and a decent internet connection."
echo ""

# Verify pinned Python is available
if ! command -v "$PYTHON_BIN" &> /dev/null; then
    echo "ERROR: $PYTHON_BIN not found."
    echo "On Arch/CachyOS: paru -S python313"
    exit 1
fi

read -p "Do you want to proceed? [y/N] " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Aborted."
    exit 1
fi

echo ""
echo "[1/3] Creating isolated virtual environment..."
mkdir -p "$APP_DATA_DIR"

if [[ -d "$VENV_DIR" ]]; then
    echo "Existing venv found. Re-creating to ensure compatibility..."
    rm -rf "$VENV_DIR"
fi

# Create venv with pinned Python
"$PYTHON_BIN" -m venv "$VENV_DIR"
echo "Created venv at $VENV_DIR ($(\"$PYTHON_BIN\" --version))"

# Activate venv for installation
source "$VENV_DIR/bin/activate"
pip install --upgrade pip

echo ""
echo "[2/3] Installing PyTorch (inside venv)..."

if [[ "$(uname)" == "Darwin" ]]; then
    # MacOS
    pip install torch torchvision torchaudio
elif [[ "$(uname)" == "Linux" ]]; then
    # Linux
    if command -v nvidia-smi &> /dev/null; then
        echo "Detected NVIDIA GPU. Installing PyTorch (standard)..."
        # Using standard PyPI which defaults to CUDA 12.x suitable for modern Arch/CachyOS
        pip install torch torchvision torchaudio
    else
        echo "No NVIDIA GPU detected. Installing CPU-only PyTorch..."
        pip install torch torchvision torchaudio --index-url https://download.pytorch.org/whl/cpu
    fi
else
    # Fallback
    echo "Unknown OS. Installing standard PyTorch..."
    pip install torch torchvision torchaudio
fi

echo ""
echo "[3/3] Installing Diffusers and Transformers..."
pip install diffusers transformers accelerate 'scipy>=1.17.1' -q

echo ""
echo "========================================================"
echo "  Setup Complete!"
echo "========================================================"
echo "The Stable Diffusion plugin will automatically use this isolated environment."
