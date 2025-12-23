#!/bin/bash
# Wrapper to run flake8 on the project
# Usage: ./run_linter.sh

if ! command -v flake8 &> /dev/null; then
    echo "Error: flake8 is not installed."
    echo "Please install it with: sudo pacman -S python-flake8"
    exit 1
fi

echo "Running flake8..."
# Exclude venv, ignored files, and specific errors if needed
# E501: Line too long (we have wide monitors)
flake8 . --count --select=E9,F63,F7,F82 --show-source --statistics
# Exclude pkg/ (build artifacts), venv, and hidden files
flake8 . --count --exit-zero --max-complexity=10 --max-line-length=127 --ignore=F541,W291,W293,E501,E203,W503,W504,E128,E129,E226,E302,E402 --exclude=pkg,venv,.git,__pycache__,build,dist,debian --statistics
