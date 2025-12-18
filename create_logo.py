#!/usr/bin/env python3
"""
Create a logo for Clockwork Orange - resizing from source image
"""

import os
import sys

from PyQt6.QtCore import Qt
from PyQt6.QtGui import QColor, QImage, QPixmap
from PyQt6.QtWidgets import QApplication

SOURCE_IMAGE = "img/clockwork_orange_gemini_full.png"


def create_clockwork_orange_logo(size=256):
    """Load and resize source image"""
    if not os.path.exists(SOURCE_IMAGE):
        print(f"Error: Source image not found: {SOURCE_IMAGE}")
        sys.exit(1)

    image = QImage(SOURCE_IMAGE)
    if image.isNull():
        print(f"Error: Failed to load image: {SOURCE_IMAGE}")
        sys.exit(1)

    # Scale image to requested size with high quality
    scaled_image = image.scaled(
        size,
        size,
        Qt.AspectRatioMode.KeepAspectRatio,
        Qt.TransformationMode.SmoothTransformation,
    )

    # Attempt to remove background
    # We assume the top-left pixel represents the background color
    if not scaled_image.isNull():
        # Convert to ARGB32 to ensure we have an alpha channel
        scaled_image = scaled_image.convertToFormat(QImage.Format.Format_ARGB32)

        # Get background color from pixel (0,0)
        bg_color = scaled_image.pixelColor(0, 0)
        bg_r, bg_g, bg_b = bg_color.red(), bg_color.green(), bg_color.blue()

        width = scaled_image.width()
        height = scaled_image.height()

        # Use color distance for better edge handling
        # threshold: colors closer than this are fully transparent
        # fuzziness: colors between threshold and this get semi-transparent (fade out halo)
        threshold = 30.0
        fuzziness = 70.0

        import math

        for y in range(height):
            for x in range(width):
                pixel_color = scaled_image.pixelColor(x, y)
                r, g, b = pixel_color.red(), pixel_color.green(), pixel_color.blue()

                # Calculate Euclidean distance in RGB space
                dist = math.sqrt((r - bg_r) ** 2 + (g - bg_g) ** 2 + (b - bg_b) ** 2)

                if dist < threshold:
                    # Fully transparent
                    scaled_image.setPixelColor(x, y, QColor(r, g, b, 0))
                elif dist < fuzziness:
                    # Semi-transparent (linear fade) to remove halo
                    # Map [threshold, fuzziness] -> [0, 255]
                    alpha = int(255 * (dist - threshold) / (fuzziness - threshold))
                    scaled_image.setPixelColor(x, y, QColor(r, g, b, alpha))

        return QPixmap.fromImage(scaled_image)

    return QPixmap.fromImage(scaled_image)


def main():
    """Create and save the logo"""
    _ = QApplication(sys.argv)

    # Create icons directory if it doesn't exist
    icons_dir = "gui/icons"
    os.makedirs(icons_dir, exist_ok=True)

    # Create different sizes
    sizes = [16, 32, 48, 64, 128, 256, 512]

    for size in sizes:
        logo = create_clockwork_orange_logo(size)
        filename = os.path.join(icons_dir, f"clockwork-orange-{size}x{size}.png")
        logo.save(filename)
        print(f"Created {filename}")

    # Also create a default icon
    logo = create_clockwork_orange_logo(128)
    default_filename = os.path.join(icons_dir, "clockwork-orange.png")
    logo.save(default_filename)
    print(f"Created {default_filename}")

    print("Logo creation complete!")


if __name__ == "__main__":
    main()
