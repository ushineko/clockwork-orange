#!/usr/bin/env python3
"""
Create a logo for Clockwork Orange - resizing from source image
"""

from PyQt6.QtWidgets import QApplication
from PyQt6.QtGui import QPainter, QColor, QFont, QPixmap, QImage
from PyQt6.QtCore import Qt
import sys
import os

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
        Qt.TransformationMode.SmoothTransformation
    )
    
    # Attempt to remove background
    # We assume the top-left pixel represents the background color
    if not scaled_image.isNull():
        # Convert to ARGB32 to ensure we have an alpha channel
        scaled_image = scaled_image.convertToFormat(QImage.Format.Format_ARGB32)
        
        # Get background color from pixel (0,0)
        bg_color = scaled_image.pixelColor(0, 0)
        bg_rgb = (bg_color.red(), bg_color.green(), bg_color.blue())
        
        width = scaled_image.width()
        height = scaled_image.height()
        
        # Simple tolerance for background detection (to handle light compression artifacts)
        tolerance = 10
        
        for y in range(height):
            for x in range(width):
                pixel_color = scaled_image.pixelColor(x, y)
                r, g, b = pixel_color.red(), pixel_color.green(), pixel_color.blue()
                
                # Check if pixel is within tolerance of background color
                if (abs(r - bg_rgb[0]) <= tolerance and
                    abs(g - bg_rgb[1]) <= tolerance and
                    abs(b - bg_rgb[2]) <= tolerance):
                    # Set alpha to 0 (fully transparent)
                    # We keep rgb values but set alpha to 0
                    scaled_image.setPixelColor(x, y, QColor(r, g, b, 0))
        
        return QPixmap.fromImage(scaled_image)
    
    return QPixmap.fromImage(scaled_image)


def main():
    """Create and save the logo"""
    app = QApplication(sys.argv)
    
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
