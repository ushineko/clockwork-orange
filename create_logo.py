#!/usr/bin/env python3
"""
Create a logo for Clockwork Orange - resizing from source image
"""

from PyQt6.QtWidgets import QApplication
from PyQt6.QtGui import QPixmap, QPainter, QImage
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
    
    return scaled_image


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
