#!/bin/bash

# Clockwork Orange Service Runner Script
# This script runs the clockwork-orange wallpaper service with proper environment setup

# Set up environment variables for desktop access
export DISPLAY=:0
export XDG_RUNTIME_DIR=/run/user/$(id -u)
export DBUS_SESSION_BUS_ADDRESS=unix:path=$XDG_RUNTIME_DIR/bus

# Change to the script directory
cd /home/nverenin/git/qdbus6_setwallpaper

# Run the clockwork-orange service with the specific Python interpreter
exec /home/nverenin/miniforge3/bin/python clockwork-orange.py "$@"
