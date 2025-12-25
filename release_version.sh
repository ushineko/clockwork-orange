#!/bin/bash
set -e

# Read current version
current_version=$(cat .tag 2>/dev/null || echo "v0.0.0")

echo "Current version: $current_version"
read -p "Enter new version (e.g. v2.3.0): " new_version

if [[ -z "$new_version" ]]; then
    echo "Version cannot be empty."
    exit 1
fi

# Update .tag file
echo "$new_version" > .tag

# Commit change
git add .tag
git commit -m "Bump version to $new_version"

# Create git tag
git tag -a "$new_version" -m "Release $new_version"

echo "Version bumped to $new_version"
echo "Ready to push. Press Enter to push changes (or Ctrl+C to cancel)..."
read

# Push changes and tags
git push origin main
git push origin "$new_version"

echo "Successfully released $new_version!"
