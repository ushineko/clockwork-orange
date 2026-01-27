#!/bin/bash
set -e

# Read version from .tag file
target_version=$(cat .tag 2>/dev/null | tr -d '[:space:]')

if [[ -z "$target_version" ]]; then
    echo "Error: .tag file is missing or empty."
    exit 1
fi

echo "Target version from .tag: $target_version"

# Check if .tag needs to be committed
if git status --porcelain | grep -q ".tag"; then
    echo "Committing .tag update..."
    git add .tag
    git commit -m "Bump version to $target_version"
else
    echo ".tag is clean."
fi

# Check if tag already exists
if git rev-parse "$target_version" >/dev/null 2>&1; then
    echo "Tag $target_version already exists."
else
    echo "Creating git tag $target_version..."
    git tag -a "$target_version" -m "Release $target_version"
fi

echo "Pushing changes and tags to remote..."
git push origin main
git push origin "$target_version"

echo "Release $target_version complete."
