#!/bin/bash
# Wrapper to run code formatter
# Tries black first, then autopep8

if command -v black &> /dev/null; then
    if command -v isort &> /dev/null; then
        echo "Using isort..."
        isort .
    fi
    echo "Using black..."
    black .
    exit 0
elif command -v autopep8 &> /dev/null; then
    echo "Using autopep8..."
    autopep8 --in-place --recursive .
    exit 0
else
    echo "Error: No formatter found."
    echo "Please install 'black' or 'autopep8' (e.g. sudo pacman -S python-black)"
    exit 1
fi
