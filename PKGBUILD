# Maintainer: Your Name <your.email@example.com>
pkgname=clockwork-orange-git
pkgver=r12.6e2e127
pkgrel=1
pkgdesc="A Python script for managing wallpapers and lock screen backgrounds on KDE Plasma 6"
arch=('any')
url="https://github.com/ushineko/clockwork-orange"
license=('MIT')
depends=('python' 'python-requests' 'python-yaml' 'qt6-tools' 'kconfig')
optdepends=('python-pyqt6: for GUI support')
makedepends=('git')
provides=("${pkgname%-git}")
conflicts=("${pkgname%-git}")
source=("git+https://github.com/ushineko/clockwork-orange.git")
sha256sums=('SKIP')
install=clockwork-orange.install

pkgver() {
	cd "${pkgname%-git}"
	printf "r%s.%s" "$(git rev-list --count HEAD)" "$(git rev-parse --short HEAD)"
}

package() {
	cd "${pkgname%-git}"

	# Install main script
	install -Dm755 clockwork-orange.py "${pkgdir}/usr/lib/${pkgname%-git}/clockwork-orange.py"
    
    # Install GUI files
    install -d "${pkgdir}/usr/lib/${pkgname%-git}/gui"
    cp -r gui/* "${pkgdir}/usr/lib/${pkgname%-git}/gui/"
    
    # Create /usr/bin symlink
    install -d "${pkgdir}/usr/bin"
    ln -s "/usr/lib/${pkgname%-git}/clockwork-orange.py" "${pkgdir}/usr/bin/clockwork-orange"

	# Install desktop entry
    # We need to fix the Icon and Exec path in the desktop file or install a new one
    # The provided install-desktop-entry.sh generates one, but we should ship a static one or fix it.
    # Let's create a proper one for the package.
    
    install -Dm644 clockwork-orange.desktop "${pkgdir}/usr/share/applications/clockwork-orange.desktop"
    # Update Exec and Icon lines for system installation
    sed -i 's|Exec=.*|Exec=/usr/bin/clockwork-orange --gui|' "${pkgdir}/usr/share/applications/clockwork-orange.desktop"
    sed -i 's|Icon=.*|Icon=clockwork-orange|' "${pkgdir}/usr/share/applications/clockwork-orange.desktop"

	# Install icons
    for res in 16 32 48 64 128 256 512; do
        if [ -f "gui/icons/clockwork-orange-${res}x${res}.png" ]; then
            install -Dm644 "gui/icons/clockwork-orange-${res}x${res}.png" \
                "${pkgdir}/usr/share/icons/hicolor/${res}x${res}/apps/clockwork-orange.png"
        fi
    done
    # Install main icon as fallback/scalable or just high-res
    install -Dm644 "gui/icons/clockwork-orange.png" "${pkgdir}/usr/share/icons/hicolor/128x128/apps/clockwork-orange.png"

	# Install license and documentation
	install -Dm644 LICENSE "${pkgdir}/usr/share/licenses/${pkgname}/LICENSE"
	install -Dm644 README.md "${pkgdir}/usr/share/doc/${pkgname}/README.md"

    # Generate version.txt
    echo "$pkgver" > "${pkgdir}/usr/lib/${pkgname%-git}/version.txt"
}
