#!/usr/bin/env python3
"""
Create a logo for Clockwork Orange - a clock that is also an orange
"""

from PyQt6.QtWidgets import QApplication
from PyQt6.QtGui import QPixmap, QPainter, QColor
from PyQt6.QtCore import Qt
import sys


def create_clockwork_orange_logo(size=256):
    """Create a logo - clock that is also an orange"""
    pixmap = QPixmap(size, size)
    pixmap.fill(Qt.GlobalColor.transparent)
    
    painter = QPainter(pixmap)
    painter.setRenderHint(QPainter.RenderHint.Antialiasing)
    
    # Calculate dimensions based on size
    margin = size // 12
    outer_size = size - (2 * margin)
    inner_size = int(outer_size * 0.7)
    inner_margin = (outer_size - inner_size) // 2
    
    # Draw orange circle (outer)
    painter.setBrush(QColor(255, 165, 0))  # Orange
    painter.setPen(QColor(200, 100, 0))    # Darker orange border
    painter.drawEllipse(margin, margin, outer_size, outer_size)
    
    # Draw clock face (inner circle)
    painter.setBrush(QColor(255, 255, 255))  # White
    painter.setPen(QColor(100, 100, 100))    # Gray border
    painter.drawEllipse(margin + inner_margin, margin + inner_margin, inner_size, inner_size)
    
    # Draw clock hands
    painter.setPen(QColor(0, 0, 0))  # Black
    painter.setBrush(QColor(0, 0, 0))
    
    center_x = size // 2
    center_y = size // 2
    
    # Hour hand (pointing to 3)
    hour_hand_length = inner_size // 3
    painter.drawLine(center_x, center_y, center_x + hour_hand_length, center_y)
    painter.drawLine(center_x, center_y, center_x + hour_hand_length - 4, center_y - 4)
    painter.drawLine(center_x, center_y, center_x + hour_hand_length - 4, center_y + 4)
    
    # Minute hand (pointing to 12)
    minute_hand_length = inner_size // 2
    painter.drawLine(center_x, center_y, center_x, center_y - minute_hand_length)
    painter.drawLine(center_x, center_y, center_x - 4, center_y - minute_hand_length + 4)
    painter.drawLine(center_x, center_y, center_x + 4, center_y - minute_hand_length + 4)
    
    # Center dot
    dot_size = max(4, size // 32)
    painter.setBrush(QColor(0, 0, 0))
    painter.drawEllipse(center_x - dot_size//2, center_y - dot_size//2, dot_size, dot_size)
    
    # Draw some orange texture lines (like orange segments)
    painter.setPen(QColor(255, 140, 0))  # Lighter orange
    line_width = max(2, size // 64)
    painter.setPen(QColor(255, 140, 0, 100))  # Semi-transparent
    
    for i in range(0, 360, 30):  # Every 30 degrees
        import math
        angle_rad = math.radians(i)
        start_radius = outer_size // 2 - 10
        end_radius = outer_size // 2 - 2
        
        start_x = center_x + int(start_radius * math.cos(angle_rad))
        start_y = center_y + int(start_radius * math.sin(angle_rad))
        end_x = center_x + int(end_radius * math.cos(angle_rad))
        end_y = center_y + int(end_radius * math.sin(angle_rad))
        
        painter.drawLine(start_x, start_y, end_x, end_y)
    
    painter.end()
    return pixmap


def main():
    """Create and save the logo"""
    app = QApplication(sys.argv)
    
    # Create different sizes
    sizes = [16, 32, 48, 64, 128, 256, 512]
    
    for size in sizes:
        logo = create_clockwork_orange_logo(size)
        filename = f"clockwork-orange-{size}x{size}.png"
        logo.save(filename)
        print(f"Created {filename}")
    
    # Also create a default icon
    logo = create_clockwork_orange_logo(128)
    logo.save("clockwork-orange.png")
    print("Created clockwork-orange.png")
    
    print("Logo creation complete!")


if __name__ == "__main__":
    main()
