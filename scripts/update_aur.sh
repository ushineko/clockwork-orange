#!/bin/bash
# AUR Package Update Script for clockwork-orange-git
# Usage: ./scripts/update_aur.sh [commit message]

set -e

PKGNAME="clockwork-orange-git"
AUR_URL="ssh://aur@aur.archlinux.org/${PKGNAME}.git"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"
AUR_DIR="${REPO_ROOT}/.aur-repo"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== AUR Update Script for ${PKGNAME} ===${NC}"

# Get version from .tag file
VERSION=$(cat "${REPO_ROOT}/.tag" 2>/dev/null || echo "unknown")
echo -e "Current version: ${YELLOW}${VERSION}${NC}"

# Default commit message
COMMIT_MSG="${1:-Update to ${VERSION}}"

# Step 1: Clone or update AUR repo
echo -e "\n${GREEN}[1/5] Setting up AUR repository...${NC}"
if [ -d "$AUR_DIR" ]; then
    echo "Updating existing AUR clone..."
    cd "$AUR_DIR"
    git fetch origin
    git reset --hard origin/master 2>/dev/null || git reset --hard origin/main 2>/dev/null || true
else
    echo "Cloning AUR repository..."
    git clone "$AUR_URL" "$AUR_DIR"
    cd "$AUR_DIR"
fi

# Step 2: Copy package files
echo -e "\n${GREEN}[2/5] Copying package files...${NC}"
cp "${REPO_ROOT}/PKGBUILD" .
cp "${REPO_ROOT}/clockwork-orange.install" .

echo "  - PKGBUILD"
echo "  - clockwork-orange.install"

# Step 3: Generate .SRCINFO
echo -e "\n${GREEN}[3/5] Generating .SRCINFO...${NC}"
makepkg --printsrcinfo > .SRCINFO
echo "  - .SRCINFO generated"

# Step 4: Show diff and confirm
echo -e "\n${GREEN}[4/5] Changes to be committed:${NC}"
git status --short
echo ""
git diff --stat 2>/dev/null || true

# Show the .SRCINFO content summary
echo -e "\n${YELLOW}Package info from .SRCINFO:${NC}"
grep -E "^\s*(pkgname|pkgver|pkgdesc|url)" .SRCINFO | head -10

echo -e "\n${YELLOW}Commit message: ${COMMIT_MSG}${NC}"
read -p "Proceed with commit and push? [y/N] " -n 1 -r
echo

if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo -e "${RED}Aborted.${NC}"
    exit 1
fi

# Step 5: Commit and push
echo -e "\n${GREEN}[5/5] Committing and pushing to AUR...${NC}"
git add PKGBUILD .SRCINFO clockwork-orange.install
git commit -m "$COMMIT_MSG"
git push origin master 2>/dev/null || git push origin main

echo -e "\n${GREEN}✓ Successfully updated ${PKGNAME} on AUR!${NC}"
echo -e "View at: https://aur.archlinux.org/packages/${PKGNAME}"
