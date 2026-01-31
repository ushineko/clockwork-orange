#!/usr/bin/env python3
"""
Main GUI window for clockwork-orange.
Refactored to use Tree Sidebar navigation.
"""
import subprocess
import sys
from pathlib import Path

import yaml
from PyQt6.QtCore import Qt, QThread, QTimer, pyqtSignal
from PyQt6.QtGui import QAction, QColor, QFont, QIcon, QPainter, QPen, QPixmap
from PyQt6.QtWidgets import (QApplication, QDialog, QHBoxLayout, QLabel,
                             QMainWindow, QMenu, QPushButton, QSplitter,
                             QStackedWidget, QSystemTrayIcon, QTabWidget,
                             QTextBrowser, QTreeWidget, QTreeWidgetItem,
                             QTreeWidgetItemIterator)
from PyQt6.QtWidgets import QVBoxLayout as QVBoxLayoutDialog

from plugin_manager import PluginManager

from .blacklist_tab import BlacklistTab
from .history_tab import HistoryTab
from .plugins_tab import SinglePluginWidget
from .service_manager import ServiceManagerWidget
from .settings_widgets import (AdvancedSettingsWidget, BasicSettingsWidget,
                               YamlEditorWidget)


# Worker thread for wallpaper changes
class WallpaperWorker(QThread):
    """Background worker for changing wallpapers without blocking GUI."""

    log_message = pyqtSignal(str)

    def __init__(self, config_data, plugin_manager):
        super().__init__()
        self.config_data = config_data
        self.plugin_manager = plugin_manager

    def run(self):
        try:
            self.log_message.emit("=== Wallpaper Change Cycle ===")

            import random
            from pathlib import Path

            import platform_utils

            # Collect sources
            sources = []
            plugins_config = self.config_data.get("plugins", {})
            for name, plugin_cfg in plugins_config.items():
                if plugin_cfg.get("enabled", False):
                    self.log_message.emit(f"Executing plugin: {name}")
                    try:
                        result = self.plugin_manager.execute_plugin(name, plugin_cfg)
                        if result.get("status") == "success":
                            path = result.get("path")
                            if path:
                                sources.append(Path(path))
                                self.log_message.emit(
                                    f"✓ Plugin {name} returned: {path}"
                                )
                        else:
                            self.log_message.emit(
                                f"✗ Plugin {name} failed: {result.get('error', 'Unknown')}"
                            )
                    except Exception as e:
                        self.log_message.emit(f"✗ Plugin {name} error: {e}")

            self.log_message.emit(f"Collected {len(sources)} sources")

            if sources:
                # Check if we have multiple monitors (Windows only)
                import platform_utils

                monitor_count = (
                    platform_utils.get_monitor_count()
                    if platform_utils.is_windows()
                    else 1
                )

                if monitor_count > 1:
                    self.log_message.emit(
                        f"Multi-monitor setup detected: {monitor_count} monitors"
                    )

                    # Fair selection: shuffle sources first, then pick from first valid source
                    # This ensures each plugin has equal chance regardless of image count
                    import random

                    random.shuffle(sources)

                    # Collect images for each monitor
                    selected_images = []
                    for monitor_idx in range(monitor_count):
                        # Try each source until we find one with images
                        for source in sources:
                            all_images = []
                            if source.is_dir():
                                all_images.extend(
                                    list(source.glob("*.jpg"))
                                    + list(source.glob("*.png"))
                                )
                            elif source.is_file():
                                all_images.append(source)

                            if all_images:
                                # Pick random image from this source
                                image = random.choice(all_images)
                                selected_images.append(image)
                                self.log_message.emit(
                                    f"Monitor {monitor_idx + 1}: {image.name} from {source.name}"
                                )
                                break

                        # If we couldn't find enough images, reuse sources
                        if len(selected_images) <= monitor_idx:
                            self.log_message.emit(
                                f"Warning: Not enough images, reusing sources"
                            )
                            break

                    # Set wallpapers for all monitors
                    if selected_images:
                        result = platform_utils.set_wallpaper_multi_monitor(
                            selected_images
                        )
                        if result:
                            self.log_message.emit(
                                f"✓ Wallpapers set for {len(selected_images)} monitor(s)"
                            )
                        else:
                            self.log_message.emit(
                                "✗ Failed to set multi-monitor wallpapers"
                            )
                    else:
                        self.log_message.emit(
                            "✗ No images found for multi-monitor setup"
                        )
                else:
                    # Single monitor: use original logic
                    # Fair selection: shuffle sources first, then pick from first valid source
                    import random

                    random.shuffle(sources)

                    selected_image = None
                    for source in sources:
                        all_images = []
                        if source.is_dir():
                            all_images.extend(
                                list(source.glob("*.jpg")) + list(source.glob("*.png"))
                            )
                        elif source.is_file():
                            all_images.append(source)

                        if all_images:
                            selected_image = random.choice(all_images)
                            self.log_message.emit(
                                f"Selected from source: {source.name}"
                            )
                            break

                    if selected_image:
                        result = platform_utils.set_wallpaper(selected_image)
                        if result:
                            self.log_message.emit(
                                f"✓ Wallpaper changed: {selected_image.name}"
                            )
                        else:
                            self.log_message.emit("✗ Failed to set wallpaper")
                    else:
                        self.log_message.emit("✗ No images found in any sources")
            else:
                self.log_message.emit("No wallpaper sources available")

        except Exception as e:
            self.log_message.emit(f"Error: {e}")


class AboutDialog(QDialog):
    """About dialog for Clockwork Orange"""

    def __init__(self, parent=None):
        super().__init__(parent)
        self.setWindowTitle("About Clockwork Orange")
        self.setModal(True)
        self.setMinimumSize(600, 500)
        self.resize(700, 550)

        layout = QVBoxLayoutDialog()

        # Tab widget for About and README
        tab_widget = QTabWidget()

        # === About Tab ===
        about_widget = QLabel()
        about_layout = QVBoxLayoutDialog(about_widget)

        # Logo
        logo_label = QLabel()
        logo_pixmap = self.get_logo()
        logo_label.setPixmap(logo_pixmap)
        logo_label.setAlignment(Qt.AlignmentFlag.AlignCenter)
        about_layout.addWidget(logo_label)

        # Title
        title_label = QLabel("Clockwork Orange")
        title_label.setFont(QFont("Arial", 16, QFont.Weight.Bold))
        title_label.setAlignment(Qt.AlignmentFlag.AlignCenter)
        about_layout.addWidget(title_label)

        # Tagline
        tagline_label = QLabel('"My choice is your imperative"')
        tagline_label.setFont(QFont("Arial", 10, QFont.Weight.Normal))
        tagline_label.setAlignment(Qt.AlignmentFlag.AlignCenter)
        about_layout.addWidget(tagline_label)

        # Copyright
        copyright_label = QLabel("© 2025 github.com/ushineko")
        copyright_label.setFont(QFont("Arial", 9))
        copyright_label.setAlignment(Qt.AlignmentFlag.AlignCenter)
        about_layout.addWidget(copyright_label)

        # Version
        version_text = self.get_version_string()
        version_label = QLabel(version_text)
        version_label.setFont(QFont("Arial", 9))
        version_label.setAlignment(Qt.AlignmentFlag.AlignCenter)
        about_layout.addWidget(version_label)

        about_layout.addStretch()
        tab_widget.addTab(about_widget, "About")

        # === README Tab ===
        readme_browser = QTextBrowser()
        readme_browser.setOpenExternalLinks(True)
        readme_content = self.load_readme()
        readme_browser.setMarkdown(readme_content)
        tab_widget.addTab(readme_browser, "README")

        layout.addWidget(tab_widget)

        # Close button
        button_layout = QHBoxLayout()
        button_layout.addStretch()
        close_btn = QPushButton("Close")
        close_btn.clicked.connect(self.accept)
        button_layout.addWidget(close_btn)
        button_layout.addStretch()

        layout.addLayout(button_layout)

        self.setLayout(layout)

    def load_readme(self):
        """Load README.md content"""
        readme_paths = [
            Path(__file__).parent.parent / "README.md",  # Development
            Path("/usr/share/doc/clockwork-orange-git/README.md"),  # Arch package
            Path("/usr/share/doc/clockwork-orange/README.md"),  # Debian package
        ]

        # For frozen apps (PyInstaller)
        if getattr(sys, "frozen", False):
            readme_paths.insert(0, Path(sys._MEIPASS) / "README.md")

        for readme_path in readme_paths:
            try:
                if readme_path.exists():
                    return readme_path.read_text(encoding="utf-8")
            except Exception:
                pass

        return "# README\n\nREADME file not found."

    def get_version_string(self):
        """Get the application version string"""
        base_path = Path(__file__).parent.parent
        if getattr(sys, "frozen", False):
            # In frozen mode, PyInstaller unpacks to sys._MEIPASS
            base_path = Path(sys._MEIPASS)

        # 1. Check for packaged version.txt (PKGBUILD/Debian/Windows Frozen)
        try:
            version_file = base_path / "version.txt"
            if version_file.exists():
                return version_file.read_text().strip()
        except Exception:
            pass

        # 2. Check for .tag file (Development mode)
        tag_version = "Unknown"
        try:
            tag_file = Path(__file__).parent.parent / ".tag"
            if tag_file.exists():
                tag_version = tag_file.read_text().strip()
        except Exception:
            pass

        # 3. Append Git info if available
        try:
            # Check if running from git repo
            if (Path(__file__).parent.parent / ".git").exists():
                rev_count = subprocess.check_output(
                    ["git", "rev-list", "--count", "HEAD"],
                    text=True,
                    stderr=subprocess.DEVNULL,
                ).strip()
                short_hash = subprocess.check_output(
                    ["git", "rev-parse", "--short", "HEAD"],
                    text=True,
                    stderr=subprocess.DEVNULL,
                ).strip()

                return f"{tag_version}-r{rev_count}-{short_hash}"
        except Exception:
            pass

        return tag_version

    def get_logo(self):
        """Get the logo from file or create a default one"""
        logo_paths = [
            Path(__file__).parent / "icons" / "clockwork-orange-128x128.png",
            Path(__file__).parent / "icons" / "clockwork-orange.png",
            Path(__file__).parent / "icons" / "clockwork-orange-64x64.png",
        ]

        for logo_path in logo_paths:
            if logo_path.exists():
                pixmap = QPixmap(str(logo_path))
                if not pixmap.isNull():
                    return pixmap.scaled(
                        128,
                        128,
                        Qt.AspectRatioMode.KeepAspectRatio,
                        Qt.TransformationMode.SmoothTransformation,
                    )

        # Fallback: create a simple logo
        return self.create_simple_logo()

    def create_simple_logo(self):
        """Create a simple fallback logo"""
        pixmap = QPixmap(128, 128)
        pixmap.fill(Qt.GlobalColor.transparent)

        painter = QPainter(pixmap)
        painter.setRenderHint(QPainter.RenderHint.Antialiasing)

        # Draw orange circle (outer)
        painter.setBrush(QColor(255, 165, 0))  # Orange
        painter.setPen(QColor(200, 100, 0))  # Darker orange border
        painter.drawEllipse(10, 10, 108, 108)

        # Draw clock face (inner circle)
        painter.setBrush(QColor(255, 255, 255))  # White
        painter.setPen(QColor(100, 100, 100))  # Gray border
        painter.drawEllipse(20, 20, 88, 88)

        # Draw clock hands
        painter.setPen(QColor(0, 0, 0))  # Black
        painter.setBrush(QColor(0, 0, 0))

        # Hour hand (pointing to 3)
        painter.drawLine(64, 64, 90, 64)
        painter.drawLine(64, 64, 88, 60)
        painter.drawLine(64, 64, 88, 68)

        # Minute hand (pointing to 12)
        painter.drawLine(64, 64, 64, 30)
        painter.drawLine(64, 64, 60, 34)
        painter.drawLine(64, 64, 68, 34)

        # Center dot
        painter.setBrush(QColor(0, 0, 0))
        painter.drawEllipse(60, 60, 8, 8)

        # Draw some orange texture lines
        painter.setPen(QColor(255, 140, 0))  # Lighter orange
        for i in range(0, 108, 8):
            painter.drawArc(10, 10, 108, 108, i * 16, 4 * 16)

        painter.end()
        return pixmap


class ClockworkOrangeGUI(QMainWindow):
    """Main GUI window for clockwork-orange"""

    def __init__(self):
        super().__init__()
        self.setWindowTitle("Clockwork Orange - Wallpaper Manager")
        self.setGeometry(100, 100, 800, 600)

        # Initialize managers
        self.config_path = Path.home() / ".config" / "clockwork-orange.yml"
        self.config_data = {}
        self.plugin_manager = PluginManager()
        self.auto_save_timer = QTimer()
        self.auto_save_timer.setSingleShot(True)
        self.auto_save_timer.setInterval(1000)
        self.auto_save_timer.timeout.connect(self.perform_auto_save)

        # Load config
        self.load_config()

        # Set application icon
        self.setWindowIcon(self._get_icon())

        # Create menu bar
        self.create_menu_bar()

        # Create Toolbar
        self.toolbar = self.addToolBar("Main Toolbar")
        self.toolbar.setMovable(False)

        toggle_sidebar_action = QAction("Toggle Sidebar", self)
        toggle_sidebar_action.setIcon(self._get_hamburger_icon())
        toggle_sidebar_action.triggered.connect(self.toggle_sidebar)
        self.toolbar.addAction(toggle_sidebar_action)

        # Main Layout: Splitter (Tree | Stack)
        self.splitter = QSplitter(Qt.Orientation.Horizontal)
        self.setCentralWidget(self.splitter)

        # Left: Tree Widget
        self.tree = QTreeWidget()
        self.tree.setHeaderHidden(True)
        self.tree.setIndentation(20)
        # Remove fixed width to allow resizing via splitter
        # self.tree.setFixedWidth(250)
        self.tree.itemClicked.connect(self.on_tree_item_clicked)
        self.splitter.addWidget(self.tree)

        # Right: Stacked Widget
        self.stack = QStackedWidget()
        self.splitter.addWidget(self.stack)

        # Initialize Pages & Populated Tree
        self.init_pages()

        # Create system tray icon
        self.tray_icon = self._create_tray_icon()

        # Set up signal handling with geometry tracking (rest of init)
        self._init_window_state()

        # Expand all tree items by default
        self.tree.expandAll()

        # Set up signal handling for proper cleanup
        import signal

        signal.signal(signal.SIGINT, self._signal_handler)
        signal.signal(signal.SIGTERM, self._signal_handler)

        # Connect signals
        self._connect_signals()

        # Set initial splitter sizes based on content
        # Calculate optimal sidebar width using font metrics (resizeColumnToContents is unreliable here)
        fm = self.tree.fontMetrics()
        max_width = 180  # Minimum start width

        iterator = QTreeWidgetItemIterator(self.tree)
        while iterator.value():
            item = iterator.value()
            text_width = fm.boundingRect(item.text(0)).width()

            # Calculate depth for indentation
            depth = 0
            parent = item.parent()
            while parent:
                depth += 1
                parent = parent.parent()

            # depth * indentation (20) + icon/gap (25) + text + right_padding (35)
            # 35px covers scrollbar and visual breathing room
            item_width = (depth * 20) + 25 + text_width + 35
            if item_width > max_width:
                max_width = item_width

            iterator += 1

        self.splitter.setSizes([int(max_width), 600])

        # Initialize automatic wallpaper changing
        self._init_wallpaper_timer()

    def closeEvent(self, event):
        """Override close event to minimize to tray instead of exiting."""
        event.ignore()
        self.hide()
        if self.tray_icon and self.tray_icon.isVisible():
            self.tray_icon.showMessage(
                "Clockwork Orange",
                "Application minimized to tray. Wallpaper changes continue in background.",
                QSystemTrayIcon.MessageIcon.Information,
                2000,
            )

    def _init_wallpaper_timer(self):
        """Initialize timer for automatic wallpaper changes."""
        self.wallpaper_worker = None
        self.wallpaper_timer = QTimer()
        self.wallpaper_timer.timeout.connect(self._trigger_wallpaper_change)

        # Get interval from config
        self._update_timer_interval()

        # Do first change after 2 seconds
        QTimer.singleShot(2000, self._trigger_wallpaper_change)

    def _update_timer_interval(self):
        """Update timer interval from config."""
        interval_seconds = self.config_data.get("default_wait", 300)
        self.wallpaper_timer.start(interval_seconds * 1000)

        # Log to Activity Log if it exists
        if hasattr(self, "service_page") and self.service_page:
            message = f"Wallpaper timer: every {interval_seconds} seconds"

            if hasattr(self.service_page, "add_log_message"):
                self.service_page.add_log_message(message)
            elif hasattr(self.service_page, "log_buffer"):
                self.service_page.log_buffer.append(message)
                if hasattr(self.service_page, "refresh_logs"):
                    self.service_page.refresh_logs()

    def _trigger_wallpaper_change(self):
        """Trigger wallpaper change in background thread."""
        # Don't start new worker if one is already running
        if self.wallpaper_worker and self.wallpaper_worker.isRunning():
            return

        self.wallpaper_worker = WallpaperWorker(self.config_data, self.plugin_manager)
        self.wallpaper_worker.log_message.connect(self._on_wallpaper_log)
        self.wallpaper_worker.start()

    def _on_wallpaper_log(self, message):
        """Handle log messages from wallpaper worker."""
        # Add to Activity Log widget
        if hasattr(self, "service_page") and self.service_page:
            from datetime import datetime

            timestamp = datetime.now().strftime("%H:%M:%S")
            full_message = f"{timestamp} {message}"

            # Check for new method (ActivityLogWidget)
            if hasattr(self.service_page, "add_log_message"):
                self.service_page.add_log_message(full_message)
            # Fallback for ServiceManagerWidget (Linux) or older setup
            elif hasattr(self.service_page, "log_buffer"):
                self.service_page.log_buffer.append(full_message)

                # Trim buffer if too large (prevent memory bloat)
                max_lines = getattr(self.service_page, "MAX_LOG_LINES", 1000)
                if len(self.service_page.log_buffer) > max_lines:
                    self.service_page.log_buffer.pop(0)

                if hasattr(self.service_page, "refresh_logs"):
                    self.service_page.refresh_logs()

    def toggle_sidebar(self):
        """Toggle visibility of the sidebar."""
        if self.tree.isVisible():
            self.tree.hide()
        else:
            self.tree.show()

    def _get_hamburger_icon(self):
        """Get a hamburger menu icon, falling back to manual drawing if needed."""
        # 1. Try standard theme icons
        for icon_name in ["open-menu", "application-menu", "view-list"]:
            if QIcon.hasThemeIcon(icon_name):
                return QIcon.fromTheme(icon_name)

        # 2. Fallback: Draw manually
        pixmap = QPixmap(24, 24)
        pixmap.fill(Qt.GlobalColor.transparent)

        painter = QPainter(pixmap)
        painter.setRenderHint(QPainter.RenderHint.Antialiasing)

        # Use a visible color (assuming dark theme based on other UI hints, or neutral grey)
        # Using a light grey which works on dark backgrounds and readable on light ones
        pen = QPen(QColor(200, 200, 200))
        pen.setWidth(2)
        painter.setPen(pen)

        # Draw 3 horizontal lines
        # 24x24 canvas
        margin = 4
        w = 24 - 2 * margin

        # Top
        painter.drawLine(margin, 7, margin + w, 7)
        # Middle
        painter.drawLine(margin, 12, margin + w, 12)
        # Bottom
        painter.drawLine(margin, 17, margin + w, 17)

        painter.end()
        return QIcon(pixmap)

    def load_config(self):
        """Load global configuration."""
        if self.config_path.exists():
            try:
                with open(self.config_path, "r") as f:
                    self.config_data = yaml.safe_load(f) or {}

                # Update wallpaper timer interval if it exists
                if hasattr(self, "wallpaper_timer") and self.wallpaper_timer:
                    self._update_timer_interval()
            except Exception as e:
                print(f"Error loading config: {e}")
                self.config_data = {}

    def init_pages(self):
        """Initialize all pages and populate the tree."""
        # 1. Service Manager (Linux) / Activity Log (Windows)
        import platform_utils

        if not platform_utils.is_windows():
            # Linux: Show Service Control
            self.service_page = ServiceManagerWidget()
            self.add_page(
                "Service Control",
                self.service_page,
                icon_name="utilities-system-monitor",
            )
        else:
            # Windows: Show Activity Log instead
            from gui.activity_log import ActivityLogWidget

            self.service_page = ActivityLogWidget()
            self.add_page(
                "Activity Log", self.service_page, icon_name="utilities-system-monitor"
            )

        # 2. Plugins (Group)
        plugins_root = QTreeWidgetItem(self.tree, ["Plugins"])
        plugins_root.setIcon(0, QIcon.fromTheme("preferences-plugin"))
        available_plugins = self.plugin_manager.get_available_plugins()

        # Add dynamic plugin pages
        for plugin_name in available_plugins:
            if plugin_name == "history":
                continue  # Hide history from plugins list (it has its own tab)

            friendly_name = plugin_name.replace("_", " ").title()
            page = SinglePluginWidget(
                plugin_name, self.config_data, self.plugin_manager
            )
            page.config_changed.connect(self.schedule_auto_save)
            self.add_page(
                friendly_name,
                page,
                parent_item=plugins_root,
                icon_name="image-x-generic",
            )

        # 3. History
        self.history_page = HistoryTab(self.config_data)
        self.add_page("History", self.history_page, icon_name="view-history")

        # 4. Blacklist
        self.blacklist_page = BlacklistTab()
        self.add_page("Blacklist", self.blacklist_page, icon_name="dialog-error")

        # 5. Settings (Group)
        settings_root = QTreeWidgetItem(self.tree, ["Settings"])
        settings_root.setIcon(0, QIcon.fromTheme("preferences-system"))

        self.basic_settings = BasicSettingsWidget(self.config_data)
        self.basic_settings.setting_changed.connect(self.schedule_auto_save)
        self.add_page(
            "Basic",
            self.basic_settings,
            parent_item=settings_root,
            icon_name="preferences-desktop",
        )

        self.advanced_settings = AdvancedSettingsWidget(self.config_data)
        self.advanced_settings.setting_changed.connect(self.schedule_auto_save)
        self.add_page(
            "Advanced",
            self.advanced_settings,
            parent_item=settings_root,
            icon_name="preferences-other",
        )

        self.yaml_editor = YamlEditorWidget(self.config_data)
        self.add_page(
            "Raw YAML",
            self.yaml_editor,
            parent_item=settings_root,
            icon_name="text-x-script",
        )

    def add_page(self, name, widget, parent_item=None, icon_name=None):
        """Add a page to the stack and tree."""
        self.stack.addWidget(widget)
        index = self.stack.count() - 1

        if parent_item:
            item = QTreeWidgetItem(parent_item, [name])
        else:
            item = QTreeWidgetItem(self.tree, [name])

        if icon_name:
            item.setIcon(0, QIcon.fromTheme(icon_name))

        item.setData(0, Qt.ItemDataRole.UserRole, index)

        # Select first item by default
        if index == 0:
            self.tree.setCurrentItem(item)

    def on_tree_item_clicked(self, item, column):
        """Handle tree navigation."""
        index = item.data(0, Qt.ItemDataRole.UserRole)
        if index is not None:
            self.stack.setCurrentIndex(index)

            # Auto-refresh pages if needed
            current_widget = self.stack.widget(index)
            if current_widget == self.blacklist_page:
                self.blacklist_page.load_blacklist()
            elif current_widget == self.history_page:
                self.history_page.refresh_stats()
            elif isinstance(current_widget, SinglePluginWidget):
                # Ensure latest config (including fonts) is applied
                current_widget.set_config(self.config_data)
                # Auto-enter review mode
                current_widget.scan_for_review()

    def schedule_auto_save(self):
        self.auto_save_timer.start()

    def perform_auto_save(self):
        """Gather config from all widgets and save."""
        print("[DEBUG] Auto-saving...")

        # Gather from settings widgets
        self.config_data.update(self.basic_settings.get_config())
        self.config_data.update(self.advanced_settings.get_config())

        # Update plugins from widgets
        if "plugins" not in self.config_data:
            self.config_data["plugins"] = {}

        for i in range(self.stack.count()):
            widget = self.stack.widget(i)
            if isinstance(widget, SinglePluginWidget):
                plugin_name = widget.plugin_name
                # Ensure we write strictly to this plugin's key
                self.config_data["plugins"][plugin_name] = widget.get_config()

        try:
            self.config_path.parent.mkdir(parents=True, exist_ok=True)
            with open(self.config_path, "w") as f:
                yaml.dump(self.config_data, f, default_flow_style=False, sort_keys=True)

            # Notify
            if self.tray_icon:
                self.tray_icon.showMessage(
                    "Saved",
                    "Configuration saved",
                    QSystemTrayIcon.MessageIcon.Information,
                    1000,
                )

            # Sync YAML editor
            self.yaml_editor.update_data(self.config_data)

            # Signal update
            self._on_config_changed()

        except Exception as e:
            print(f"Save failed: {e}")

    def create_menu_bar(self):
        menubar = self.menuBar()
        file_menu = menubar.addMenu("&File")

        exit_action = QAction("&Exit", self)
        exit_action.setShortcut("Ctrl+Q")
        exit_action.triggered.connect(self.quit_application)
        file_menu.addAction(exit_action)

        view_menu = menubar.addMenu("&View")
        refresh_action = QAction("&Refresh Status", self)
        refresh_action.setShortcut("F5")
        refresh_action.triggered.connect(self.refresh_all)
        view_menu.addAction(refresh_action)

        help_menu = menubar.addMenu("&Help")
        about_action = QAction("&About Clockwork Orange", self)
        about_action.triggered.connect(self.show_about)
        help_menu.addAction(about_action)

        about_qt = QAction("About &Qt", self)
        about_qt.triggered.connect(QApplication.aboutQt)
        help_menu.addAction(about_qt)

    def _create_tray_icon(self):
        if not QSystemTrayIcon.isSystemTrayAvailable():
            return None

        tray_icon = QSystemTrayIcon(self._get_icon(), self)
        tray_icon.setToolTip("Clockwork Orange")

        # Create tray menu
        tray_menu = QMenu()

        show_action = QAction("Show", self)
        show_action.triggered.connect(self.show_window)
        tray_menu.addAction(show_action)
        tray_menu.addSeparator()

        about_action = QAction("About", self)
        about_action.triggered.connect(self.show_about)
        tray_menu.addAction(about_action)
        tray_menu.addSeparator()

        quit_action = QAction("Quit", self)
        quit_action.triggered.connect(self.quit_application)
        tray_menu.addAction(quit_action)

        tray_icon.setContextMenu(tray_menu)
        tray_icon.activated.connect(self._tray_icon_activated)

        tray_icon.show()
        return tray_icon

    def _tray_icon_activated(self, reason):
        if reason == QSystemTrayIcon.ActivationReason.DoubleClick:
            self.show_window()
        elif reason == QSystemTrayIcon.ActivationReason.Trigger:
            if self.isVisible():
                self.hide()
            else:
                self.show_window()

    def _get_icon(self):
        """Get the application icon, handling frozen state."""
        if getattr(sys, "frozen", False):
            # In frozen PyInstaller bundle
            # We added data 'gui/icons;gui/icons' so it should be in sys._MEIPASS/gui/icons
            base_path = Path(sys._MEIPASS) / "gui"
        else:
            # In development: gui/main_window.py -> gui/
            base_path = Path(__file__).parent

        logo_paths = [
            base_path / "icons" / "clockwork-orange-128x128.png",
            base_path / "icons" / "clockwork-orange.png",
            base_path / "icons" / "clockwork-orange-64x64.png",
            # Fallback to root (less likely in frozen but good for dev)
            base_path.parent / "icon.png",
        ]

        for logo_path in logo_paths:
            if logo_path.exists():
                return QIcon(str(logo_path))

        print(f"[WARNING] No icon found. Searched: {[str(p) for p in logo_paths]}")
        return QIcon()

    def _init_window_state(self):
        self._geometry_timer = QTimer()
        self._geometry_timer.setSingleShot(True)
        self._geometry_timer.timeout.connect(self.save_window_geometry)
        self._geometry_timer.setInterval(500)

        QTimer.singleShot(100, self.restore_window_geometry)

    def restore_window_geometry(self):
        width = self.config_data.get("window_width", 800)
        height = self.config_data.get("window_height", 600)
        self.resize(width, height)
        self.center_window()
        print(f"[DEBUG] Window restored to {width}x{height} at {self.pos()}")

    def save_window_geometry(self):
        self.config_data["window_width"] = self.width()
        self.config_data["window_height"] = self.height()
        self.schedule_auto_save()

    def center_window(self):
        screen = QApplication.primaryScreen().geometry()
        window = self.geometry()
        x = (screen.width() - window.width()) // 2
        y = (screen.height() - window.height()) // 2
        self.move(x, y)

    def resizeEvent(self, event):
        super().resizeEvent(event)
        self._geometry_timer.stop()
        self._geometry_timer.start()

    def changeEvent(self, event):
        super().changeEvent(event)
        if event.type() == event.Type.WindowStateChange:
            QTimer.singleShot(100, self.save_window_geometry)

    def quit_application(self):
        if self.tray_icon:
            self.tray_icon.hide()
        QApplication.quit()

    def _signal_handler(self, signum, frame):
        print(f"\n[DEBUG] Received signal {signum}, shutting down gracefully...")
        self.quit_application()

    def show_window(self):
        self.show()
        self.raise_()
        self.activateWindow()
        self.setWindowState(
            self.windowState() & ~Qt.WindowState.WindowMinimized
            | Qt.WindowState.WindowActive
        )

    def show_about(self):
        dialog = AboutDialog(self)
        dialog.exec()

    def refresh_all(self):
        if hasattr(self.service_page, "refresh_status"):
            self.service_page.refresh_status()
        self.load_config()

    def _connect_signals(self):
        """Connect internal signals"""
        if hasattr(self.service_page, "status_changed"):
            self.service_page.status_changed.connect(self._on_service_status_changed)

    def _on_service_status_changed(self, status):
        if self.tray_icon:
            if status == "running":
                self.tray_icon.showMessage(
                    "Clockwork Orange",
                    "Service is running",
                    QSystemTrayIcon.MessageIcon.Information,
                    2000,
                )
            elif status == "stopped":
                self.tray_icon.showMessage(
                    "Clockwork Orange",
                    "Service stopped",
                    QSystemTrayIcon.MessageIcon.Warning,
                    2000,
                )

    def _on_config_changed(self):
        """Handle configuration changes by updating UI and behavior."""
        # Update wallpaper timer interval
        self._update_timer_interval()

        # Other pages might need updates here in the future
        if self.tray_icon:
            pass


def main():
    app = QApplication(sys.argv)
    app.setApplicationName("clockwork-orange")
    app.setApplicationDisplayName("Clockwork Orange")
    app.setDesktopFileName("clockwork-orange")
    app.setApplicationVersion("Rolling")

    # Process check
    # Process check (Singleton)
    try:
        import time

        import platform_utils

        start_time = time.time()
        print(f"[DEBUG] Checking for existing instances...")

        # Use fast Named Mutex (Windows) or File Lock (Linux)
        # This is instant compared to iterating processes
        if not platform_utils.acquire_instance_lock("clockwork_orange_gui_lock"):
            print("[DEBUG] Another instance is already running (Lock held)")
            return 0

        print(f"[DEBUG] Instance check took {time.time() - start_time:.4f}s")
    except Exception as e:
        print(f"[WARNING] Failed to check for existing instances: {e}")

    window = ClockworkOrangeGUI()
    if window.tray_icon:
        window.tray_icon.show()
        window.tray_icon.showMessage(
            "Clockwork Orange",
            "Application started",
            QSystemTrayIcon.MessageIcon.Information,
            3000,
        )

    window.show()
    print(f"[DEBUG] Window shown. Geometry: {window.geometry()}")

    # Force window to front
    window.raise_()
    window.activateWindow()

    return app.exec()


if __name__ == "__main__":
    sys.exit(main())
