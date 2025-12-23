#!/usr/bin/env python3
"""
Main GUI window for clockwork-orange.
Refactored to use Tree Sidebar navigation.
"""
import subprocess
import sys
from pathlib import Path

import yaml
from PyQt6.QtCore import Qt, QTimer
from PyQt6.QtGui import QAction, QColor, QFont, QIcon, QPainter, QPixmap
from PyQt6.QtWidgets import (
    QApplication,
    QDialog,
    QHBoxLayout,
    QLabel,
    QMainWindow,
    QMenu,
    QPushButton,
    QSplitter,
    QStackedWidget,
    QSystemTrayIcon,
    QTreeWidget,
    QTreeWidgetItem,
)
from PyQt6.QtWidgets import QVBoxLayout as QVBoxLayoutDialog

from plugin_manager import PluginManager

from .blacklist_tab import BlacklistTab
from .history_tab import HistoryTab
from .plugins_tab import SinglePluginWidget
from .service_manager import ServiceManagerWidget
from .settings_widgets import (
    AdvancedSettingsWidget,
    BasicSettingsWidget,
    YamlEditorWidget,
)


class AboutDialog(QDialog):
    """About dialog for Clockwork Orange"""

    def __init__(self, parent=None):
        super().__init__(parent)
        self.setWindowTitle("About Clockwork Orange")
        self.setModal(True)
        self.setFixedSize(400, 300)

        layout = QVBoxLayoutDialog()

        # Logo
        logo_label = QLabel()
        logo_pixmap = self.get_logo()
        logo_label.setPixmap(logo_pixmap)
        logo_label.setAlignment(Qt.AlignmentFlag.AlignCenter)
        layout.addWidget(logo_label)

        # Title
        title_label = QLabel("Clockwork Orange")
        title_label.setFont(QFont("Arial", 16, QFont.Weight.Bold))
        title_label.setAlignment(Qt.AlignmentFlag.AlignCenter)
        layout.addWidget(title_label)

        # Tagline
        tagline_label = QLabel('"My choice is your imperative"')
        tagline_label.setFont(QFont("Arial", 10, QFont.Weight.Normal))
        tagline_label.setAlignment(Qt.AlignmentFlag.AlignCenter)
        layout.addWidget(tagline_label)

        # Copyright
        copyright_label = QLabel("© 2025 github.com/ushineko")
        copyright_label.setFont(QFont("Arial", 9))
        copyright_label.setAlignment(Qt.AlignmentFlag.AlignCenter)
        layout.addWidget(copyright_label)

        # Version
        version_text = self.get_version_string()
        version_label = QLabel(version_text)
        version_label.setFont(QFont("Arial", 9))
        version_label.setAlignment(Qt.AlignmentFlag.AlignCenter)
        layout.addWidget(version_label)

        # Spacer
        layout.addStretch()

        # Close button
        button_layout = QHBoxLayout()
        button_layout.addStretch()
        close_btn = QPushButton("Close")
        close_btn.clicked.connect(self.accept)
        button_layout.addWidget(close_btn)
        button_layout.addStretch()

        layout.addLayout(button_layout)

        self.setLayout(layout)

    def get_version_string(self):
        """Get the application version string with git info if available."""
        project_root = Path(__file__).parent.parent
        git_dir = project_root / ".git"

        # Check for version.txt (packaged version)
        version_file = project_root / "version.txt"
        if version_file.exists():
            try:
                return f"Version: {version_file.read_text().strip()}"
            except Exception:
                pass

        if git_dir.exists():
            # Try to get git version from project root
            try:
                # Use subproccess to get git describe or short hash
                # First try describe for tags
                result = subprocess.run(
                    ["git", "describe", "--tags"],
                    cwd=project_root,
                    capture_output=True,
                    text=True,
                )
                if result.returncode == 0:
                    return f"Version: {result.stdout.strip()}"

                # Fallback to count.short_sba
                count = subprocess.run(
                    ["git", "rev-list", "--count", "HEAD"],
                    cwd=project_root,
                    capture_output=True,
                    text=True,
                ).stdout.strip()

                sha = subprocess.run(
                    ["git", "rev-parse", "--short", "HEAD"],
                    cwd=project_root,
                    capture_output=True,
                    text=True,
                ).stdout.strip()

                return f"Version: r{count}.{sha}"

            except Exception:
                pass

        return "Version: Unknown"

        return "Version: Unknown"

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
        toggle_sidebar_action.setIcon(
            QIcon.fromTheme("view-sidebar")
        )  # Try to use system theme icon or text
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

        # Set initial splitter sizes (Sidebar: 200, Content: Remaining)
        self.splitter.setSizes([200, 600])

    def toggle_sidebar(self):
        """Toggle visibility of the sidebar."""
        if self.tree.isVisible():
            self.tree.hide()
        else:
            self.tree.show()

    def load_config(self):
        """Load global configuration."""
        if self.config_path.exists():
            try:
                with open(self.config_path, "r") as f:
                    self.config_data = yaml.safe_load(f) or {}
            except Exception as e:
                print(f"Error loading config: {e}")
                self.config_data = {}

    def init_pages(self):
        """Initialize all pages and populate the tree."""
        # 1. Service Manager
        self.service_page = ServiceManagerWidget()
        self.add_page(
            "Service Control", self.service_page, icon_name="utilities-system-monitor"
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
        # Implementation from previous file
        logo_paths = [
            Path(__file__).parent / "icons" / "clockwork-orange-128x128.png",
            Path(__file__).parent / "icons" / "clockwork-orange.png",
            Path(__file__).parent / "icons" / "clockwork-orange-64x64.png",
            Path(__file__).parent.parent / "icon.png",
        ]

        for logo_path in logo_paths:
            if logo_path.exists():
                return QIcon(str(logo_path))
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
        if self.tray_icon:
            pass  # Silent update or user feedback handled by auto-save trigger


def main():
    app = QApplication(sys.argv)
    app.setApplicationName("Clockwork Orange")
    app.setApplicationVersion("Rolling")

    # Process check
    try:
        import os

        import psutil

        current_pid = os.getpid()
        for proc in psutil.process_iter(["pid", "name", "cmdline"]):
            if (
                proc.info["name"] == "python3"
                and proc.info["cmdline"]
                and "clockwork-orange.py" in " ".join(proc.info["cmdline"])
                and "--gui" in " ".join(proc.info["cmdline"])
                and proc.info["pid"] != current_pid
            ):
                print("[DEBUG] Another instance is already running")
                return 0
    except Exception:
        pass

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
    return app.exec()


if __name__ == "__main__":
    sys.exit(main())
