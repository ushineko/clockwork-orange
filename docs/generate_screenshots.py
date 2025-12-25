#!/usr/bin/env python3
"""
Automated screenshot generator for Clockwork Orange GUI documentation.
Launches the application, navigates through tabs, captures screenshots,
redacts sensitive text fields using introspection, and generates GUI.md.
"""
import sys
import time
from pathlib import Path

# Add project root to sys.path
PROJECT_ROOT = Path(__file__).parent.parent.resolve()
sys.path.insert(0, str(PROJECT_ROOT))

from PIL import Image, ImageFilter
from PyQt6.QtCore import QCoreApplication, QRect
from PyQt6.QtWidgets import QApplication, QLineEdit, QPlainTextEdit, QTextEdit

# Mock configuration to avoid needing real config files
from gui.main_window import ClockworkOrangeGUI
from plugin_manager import PluginManager


def find_sensitive_widgets(window):
    """Find all text input widgets that should be redacted."""
    sensitive_widgets = []
    # Find all QLineEdit, QTextEdit, QPlainTextEdit
    for widget_type in [QLineEdit, QTextEdit, QPlainTextEdit]:
        widgets = window.findChildren(widget_type)
        for w in widgets:
            # Skip if empty or read-only and small (likely labels or incidental)
            # But Service Logs are ReadOnly, so we MUST include them.
            # Maybe filter by visibility?
            if w.isVisible():
                sensitive_widgets.append(w)
    return sensitive_widgets


def get_widget_rect(window, widget):
    """Get the widget's geometry relative to the main window, accounting for DPR."""
    if not widget.isVisible():
        return None

    # Get vector from window top-left to widget top-left
    # mapTo(window, ...) works if window is ancestor.
    try:
        top_left = widget.mapTo(window, widget.rect().topLeft())
    except Exception:
        # Fallback to global mapping
        global_pos = widget.mapToGlobal(widget.rect().topLeft())
        top_left = window.mapFromGlobal(global_pos)

    rect = QRect(top_left, widget.size())

    # Scale for Device Pixel Ratio (High DPI)
    dpr = window.devicePixelRatio()
    if dpr != 1.0:
        rect = QRect(
            int(rect.x() * dpr),
            int(rect.y() * dpr),
            int(rect.width() * dpr),
            int(rect.height() * dpr),
        )

    return rect


def apply_blur(pixmap_path, rects):
    """Apply Gaussian blur to specific regions of the image using Pillow."""
    try:
        img = Image.open(pixmap_path)

        # Create a mask or just process regions
        for rect in rects:
            # rect is (x, y, width, height)
            # Ensure coordinates are within bounds
            x1 = max(0, rect.x())
            y1 = max(0, rect.y())
            x2 = min(img.width, rect.x() + rect.width())
            y2 = min(img.height, rect.y() + rect.height())

            if x2 <= x1 or y2 <= y1:
                continue

            box = (x1, y1, x2, y2)

            # Crop the region
            region = img.crop(box)

            # Apply heavy blur
            blurred_region = region.filter(ImageFilter.GaussianBlur(radius=15))

            # Paste back
            img.paste(blurred_region, box)

        img.save(pixmap_path)
    except Exception as e:
        print(f"Error applying blur: {e}")


# Mock configuration dict
DUMMY_CONFIG = {
    "plugins": {
        "local": {"enabled": True},
        # Ensure google_images key exists so we can navigate to it.
        # The key in available_plugins for google_images is "google_images"
        "google_images": {
            "enabled": True,
            "query": [{"term": "cats", "enabled": True}],
        },
        "wallhaven": {"enabled": True, "query": "cyberpunk"},
    },
    "history": [],
    "blacklist": [],
    "image_extensions": ".jpg,.jpeg,.png,.gif",  # Basic defaults
}


class MockPluginManager(PluginManager):
    """Mock plugin manager to return controlled data."""

    def get_available_plugins(self):
        # Return list of plugin names expected
        return ["local", "google_images", "wallhaven", "history"]

    def get_plugin_schema(self, plugin_name):
        # Return empty or basic schema to avoid errors
        return {}

    def get_plugin_description(self, plugin_name):
        return f"Description for {plugin_name}"


class TestGUI(ClockworkOrangeGUI):
    """Subclass to override config loading."""

    def load_config(self):
        self.config_data = DUMMY_CONFIG
        # Also enforce our MockPluginManager if possible,
        # but self.plugin_manager is set in __init__ before load_config.
        # We can swap it here.
        self.plugin_manager = MockPluginManager()


class ScreenshotGenerator:
    def __init__(self):
        self.app = QApplication(sys.argv)

        # Initialize GUI with mocked config via subclass
        self.window = TestGUI()
        self.window.resize(1100, 750)
        self.window.show()

        self.output_dir = PROJECT_ROOT / "docs" / "img"
        self.output_dir.mkdir(parents=True, exist_ok=True)

        self.markdown_lines = ["# Clockwork Orange GUI Documentation\n\n"]
        self.markdown_lines.append(
            "This document provides a tour of the user interface. Sensitive text areas in the screenshots have been automatically redacted.\n"
        )

    def capture_screenshot(self, name, title, description):
        """Capture the current window state."""
        print(f"Capturing: {name}")

        # Allow GUI events to process and render
        for _ in range(10):
            QCoreApplication.processEvents()
            time.sleep(0.05)

        # 1. Identify sensitive areas
        sensitive_widgets = find_sensitive_widgets(self.window)
        rects = []
        for w in sensitive_widgets:
            rect = get_widget_rect(self.window, w)
            if rect:
                rects.append(rect)

        # 2. Grab Window
        pixmap = self.window.grab()
        file_path = self.output_dir / f"{name}.png"
        pixmap.save(str(file_path))

        # 3. Apply Blur
        apply_blur(file_path, rects)

        # 4. Add to Markdown
        self.markdown_lines.append(f"## {title}\n")
        self.markdown_lines.append(f"{description}\n\n")
        self.markdown_lines.append(f"![{title}](docs/img/{name}.png)\n\n")

    def navigate_to(self, item_name):
        """Navigate the sidebar tree to the specified item."""
        # Find item in tree
        # The tree is user-friendly names?
        # Let's search the tree
        found = False
        iterator = QTreeWidgetItemIterator(self.window.tree)
        while iterator.value():
            item = iterator.value()
            if item.text(0) == item_name:
                self.window.tree.setCurrentItem(item)
                self.window.on_tree_item_clicked(item, 0)
                found = True
                break
            iterator += 1

        if not found:
            print(f"Warning: Could not navigate to '{item_name}'")

    def run(self):
        # 1. Service Control
        self.navigate_to("Service Control")
        self.capture_screenshot(
            "01_service_control",
            "Service Control",
            "The main dashboard for managing the background daemon. You can start/stop the service and view recent logs.",
        )

        # 2. Plugins
        # Expand plugins if needed, usually they are expanded by default or top level
        # Assuming "Local" or similar exists if enabled. The script in main_window inits them.
        # "Google Images" is a good one
        self.navigate_to("Google Images")
        self.capture_screenshot(
            "02_plugin_google",
            "Plugin Configuration",
            "Configure individual plugins. Features include search terms, enablement toggles, and on-demand execution controls.",
        )

        self.navigate_to("Wallhaven")
        self.capture_screenshot(
            "02b_plugin_wallhaven",
            "Wallhaven Plugin",
            "Download high-quality wallpapers from Wallhaven.cc with filters for categories, purity, and ratios.",
        )

        # 3. History
        self.navigate_to("History")
        self.capture_screenshot(
            "03_history",
            "Wallpaper History",
            "View a log of previously set wallpapers. Includes stats on plugin usage.",
        )

        # 4. Blacklist
        self.navigate_to("Blacklist")
        self.capture_screenshot(
            "04_blacklist",
            "Blacklist Manager",
            "Review and manage blacklisted images. Blacklisted images will definitely be excluded from rotation.",
        )

        # 5. Settings - Basic
        self.navigate_to("Basic")
        self.capture_screenshot(
            "05_settings_basic",
            "Basic Settings",
            "Configure general application behavior like theme and startup options.",
        )

        # 6. Settings - Advanced
        self.navigate_to("Advanced")
        self.capture_screenshot(
            "06_settings_advanced",
            "Advanced Settings",
            "Fine-tune low-level options, debug flags, and file extension filters.",
        )

        # 7. Settings - YAML
        self.navigate_to("Raw YAML")
        self.capture_screenshot(
            "07_settings_yaml",
            "Raw Configuration Editor",
            "Directly edit the underlying configuration file with syntax validation.",
        )

        # Write Markdown
        md_path = PROJECT_ROOT / "GUI.md"
        with open(md_path, "w") as f:
            f.writelines(self.markdown_lines)

        print(f"Documentation generated at {md_path}")
        self.app.quit()


# Helper for tree iteration since PyQt6 might handle iterators differently
from PyQt6.QtWidgets import QTreeWidgetItemIterator

if __name__ == "__main__":
    generator = ScreenshotGenerator()
    generator.run()
