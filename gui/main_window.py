#!/usr/bin/env python3
"""
Main GUI window for clockwork-orange
"""

import sys
import os
import subprocess
from pathlib import Path
from PyQt6.QtWidgets import (QApplication, QMainWindow, QTabWidget, QVBoxLayout, 
                             QWidget, QMessageBox, QSystemTrayIcon, QMenu, QMenuBar,
                             QDialog, QLabel, QVBoxLayout as QVBoxLayoutDialog, QHBoxLayout,
                             QPushButton, QTextEdit)
from PyQt6.QtCore import Qt, QTimer, pyqtSignal, QCoreApplication
from PyQt6.QtGui import QIcon, QAction, QPixmap, QPainter, QColor, QFont

from .service_manager import ServiceManagerWidget
from .config_manager import ConfigManagerWidget


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
        tagline_label.setStyleSheet("color: #666; font-style: italic;")
        layout.addWidget(tagline_label)
        
        # Copyright
        copyright_label = QLabel("© 2025 github.com/ushineko")
        copyright_label.setFont(QFont("Arial", 9))
        copyright_label.setAlignment(Qt.AlignmentFlag.AlignCenter)
        layout.addWidget(copyright_label)
        
        # Description
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
        base_version = "1.0"
        git_ref = ""
        
        try:
            # Try to get git version from project root
            project_root = Path(__file__).parent.parent
            result = subprocess.run(
                ['git', 'describe', '--tags', '--always', '--dirty'], 
                cwd=project_root,
                capture_output=True, 
                text=True, 
                timeout=1
            )
            if result.returncode == 0:
                git_ref = result.stdout.strip()
        except Exception:
            pass
            
        if git_ref:
            return f"Version: {base_version} ({git_ref})"
        return f"Version: {base_version}"
        
    def get_logo(self):
        """Get the logo from file or create a default one"""
        # Try to load the actual logo file from gui/icons directory
        logo_paths = [
            Path(__file__).parent / "icons" / "clockwork-orange-128x128.png",
            Path(__file__).parent / "icons" / "clockwork-orange.png",
            Path(__file__).parent / "icons" / "clockwork-orange-64x64.png"
        ]
        
        for logo_path in logo_paths:
            if logo_path.exists():
                pixmap = QPixmap(str(logo_path))
                if not pixmap.isNull():
                    return pixmap.scaled(128, 128, Qt.AspectRatioMode.KeepAspectRatio, Qt.TransformationMode.SmoothTransformation)
        
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
        painter.setPen(QColor(200, 100, 0))    # Darker orange border
        painter.drawEllipse(10, 10, 108, 108)
        
        # Draw clock face (inner circle)
        painter.setBrush(QColor(255, 255, 255))  # White
        painter.setPen(QColor(100, 100, 100))    # Gray border
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
        
        # Set application icon (if available)
        self.setWindowIcon(self._get_icon())
        
        # Create menu bar
        self.create_menu_bar()
        
        # Create central widget with tabs
        self.central_widget = QWidget()
        self.setCentralWidget(self.central_widget)
        
        # Create tab widget
        self.tab_widget = QTabWidget()
        
        # Create service manager tab
        self.service_manager = ServiceManagerWidget()
        self.tab_widget.addTab(self.service_manager, "Service Management")
        
        # Create config manager tab
        self.config_manager = ConfigManagerWidget()
        self.tab_widget.addTab(self.config_manager, "Configuration")
        
        # Layout
        layout = QVBoxLayout()
        layout.addWidget(self.tab_widget)
        self.central_widget.setLayout(layout)
        
        # Create system tray icon
        self.tray_icon = self._create_tray_icon()
        
        # Connect signals
        self._connect_signals()
        
        # Set up signal handling for proper cleanup
        import signal
        signal.signal(signal.SIGINT, self._signal_handler)
        signal.signal(signal.SIGTERM, self._signal_handler)
        
        # Set up geometry change tracking
        self._geometry_timer = QTimer()
        self._geometry_timer.setSingleShot(True)
        self._geometry_timer.timeout.connect(self.save_window_geometry)
        self._geometry_timer.setInterval(500)  # 500ms delay
        
        # Note: Position tracking variables removed for Wayland compatibility
        
        # Restore window position and size after window is shown
        QTimer.singleShot(100, self.restore_window_geometry)
        
        # Note: Position monitoring disabled for Wayland compatibility
        # Wayland doesn't allow applications to detect their own window position changes
    
    def create_menu_bar(self):
        """Create the menu bar"""
        menubar = self.menuBar()
        
        # File menu
        file_menu = menubar.addMenu('&File')
        
        # Save window size action
        save_size_action = QAction('&Save Window Size', self)
        save_size_action.triggered.connect(self.save_window_geometry)
        file_menu.addAction(save_size_action)
        
        file_menu.addSeparator()
        
        # Exit action
        exit_action = QAction('&Exit', self)
        exit_action.setShortcut('Ctrl+Q')
        exit_action.triggered.connect(self.quit_application)
        file_menu.addAction(exit_action)
        
        # View menu
        view_menu = menubar.addMenu('&View')
        
        # Refresh status action
        refresh_action = QAction('&Refresh Status', self)
        refresh_action.setShortcut('F5')
        refresh_action.triggered.connect(self.refresh_all)
        view_menu.addAction(refresh_action)
        
        # Help menu
        help_menu = menubar.addMenu('&Help')
        
        # About action
        about_action = QAction('&About Clockwork Orange', self)
        about_action.triggered.connect(self.show_about)
        help_menu.addAction(about_action)
        
        # About Qt action
        about_qt_action = QAction('About &Qt', self)
        about_qt_action.triggered.connect(QApplication.aboutQt)
        help_menu.addAction(about_qt_action)
    
    def show_about(self):
        """Show the about dialog"""
        dialog = AboutDialog(self)
        dialog.exec()
    
    def quit_application(self):
        """Quit the application completely"""
        # Hide tray icon if it exists
        if self.tray_icon:
            self.tray_icon.hide()
        
        # Close the application
        QApplication.quit()
    
    def _signal_handler(self, signum, frame):
        """Handle system signals for graceful shutdown"""
        print(f"\n[DEBUG] Received signal {signum}, shutting down gracefully...")
        self.quit_application()
    
    def refresh_all(self):
        """Refresh all tabs"""
        if hasattr(self.service_manager, 'refresh_status'):
            self.service_manager.refresh_status()
        if hasattr(self.config_manager, 'load_config'):
            self.config_manager.load_config()
        
    def _get_icon(self):
        """Get application icon"""
        # Try to find an icon file in the gui/icons directory
        icon_paths = [
            Path(__file__).parent / "icons" / "clockwork-orange-128x128.png",
            Path(__file__).parent / "icons" / "clockwork-orange.png",
            Path(__file__).parent / "icons" / "clockwork-orange-64x64.png",
            Path(__file__).parent / "icons" / "clockwork-orange-32x32.png",
            Path(__file__).parent.parent / "icon.png",
            Path(__file__).parent.parent / "assets" / "icon.png"
        ]
        
        for icon_path in icon_paths:
            if icon_path.exists():
                return QIcon(str(icon_path))
        
        # Return default icon if none found
        return QIcon()
    
    def _create_tray_icon(self):
        """Create system tray icon"""
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
        
        return tray_icon
    
    def _tray_icon_activated(self, reason):
        """Handle tray icon activation"""
        if reason == QSystemTrayIcon.ActivationReason.DoubleClick:
            self.show_window()
        elif reason == QSystemTrayIcon.ActivationReason.Trigger:
            # Single click - toggle window visibility
            if self.isVisible():
                self.hide()
            else:
                self.show_window()
    
    def show_window(self):
        """Show and activate the window"""
        self.show()
        self.raise_()
        self.activateWindow()
        # Ensure window is on top
        self.setWindowState(self.windowState() & ~Qt.WindowState.WindowMinimized | Qt.WindowState.WindowActive)
    
    def _connect_signals(self):
        """Connect internal signals"""
        # Connect service manager signals
        if hasattr(self.service_manager, 'status_changed'):
            self.service_manager.status_changed.connect(self._on_service_status_changed)
        
        # Connect config manager signals
        if hasattr(self.config_manager, 'config_changed'):
            self.config_manager.config_changed.connect(self._on_config_changed)
    
    def _on_service_status_changed(self, status):
        """Handle service status changes"""
        if self.tray_icon:
            if status == "running":
                self.tray_icon.showMessage("Clockwork Orange", "Service is running", 
                                         QSystemTrayIcon.MessageIcon.Information, 2000)
            elif status == "stopped":
                self.tray_icon.showMessage("Clockwork Orange", "Service stopped", 
                                         QSystemTrayIcon.MessageIcon.Warning, 2000)
    
    def _on_config_changed(self):
        """Handle configuration changes"""
        if self.tray_icon:
            self.tray_icon.showMessage("Clockwork Orange", "Configuration updated", 
                                     QSystemTrayIcon.MessageIcon.Information, 2000)
    
    def closeEvent(self, event):
        """Handle window close event"""
        # Save window geometry before closing
        self.save_window_geometry()
        
        if self.tray_icon and self.tray_icon.isVisible():
            # Just hide the window, don't show message every time
            self.hide()
            event.ignore()
        else:
            event.accept()
    
    def restore_window_geometry(self):
        """Restore window size from configuration and center the window"""
        try:
            import yaml
            from pathlib import Path
            
            config_path = Path.home() / ".config" / "clockwork-orange.yml"
            if config_path.exists():
                with open(config_path, 'r') as f:
                    config = yaml.safe_load(f) or {}
                
                # Get window size with defaults
                width = config.get('window_width', 800)
                height = config.get('window_height', 600)
                
                # Set window size
                self.resize(width, height)
                
                # Always center the window
                self.center_window()
                    
        except Exception as e:
            print(f"[DEBUG] Failed to restore window geometry: {e}")
            # Center the window as fallback
            self.center_window()
    
    def save_window_geometry(self):
        """Save current window size to configuration"""
        print(f"[DEBUG] Timer fired - saving window size")
        try:
            import yaml
            from pathlib import Path
            
            config_path = Path.home() / ".config" / "clockwork-orange.yml"
            config = {}
            
            # Load existing config if it exists
            if config_path.exists():
                with open(config_path, 'r') as f:
                    config = yaml.safe_load(f) or {}
            
            # Get current window size only
            geometry = self.geometry()
            width, height = geometry.width(), geometry.height()
            
            # Save size
            config['window_width'] = width
            config['window_height'] = height
            
            # Save config
            config_path.parent.mkdir(parents=True, exist_ok=True)
            with open(config_path, 'w') as f:
                yaml.dump(config, f, default_flow_style=False, sort_keys=True)
                
        except Exception as e:
            print(f"[DEBUG] Failed to save window geometry: {e}")
    
    def center_window(self):
        """Center the window on the screen"""
        screen = QApplication.primaryScreen().geometry()
        window = self.geometry()
        x = (screen.width() - window.width()) // 2
        y = (screen.height() - window.height()) // 2
        self.move(x, y)
    
    def resizeEvent(self, event):
        """Handle window resize event"""
        super().resizeEvent(event)
        # Restart the timer to save geometry after user stops resizing
        print(f"[DEBUG] Window resized - restarting timer")
        self._geometry_timer.stop()
        self._geometry_timer.start()
    
    def showEvent(self, event):
        """Handle window show event"""
        super().showEvent(event)
        print(f"[DEBUG] Window shown - saving geometry")
        # Save geometry when window is shown (in case it was moved while hidden)
        QTimer.singleShot(100, self.save_window_geometry)
    
    def changeEvent(self, event):
        """Handle window state changes"""
        super().changeEvent(event)
        if event.type() == event.Type.WindowStateChange:
            print(f"[DEBUG] Window state changed - saving geometry")
            QTimer.singleShot(100, self.save_window_geometry)
    
    # Position monitoring methods removed for Wayland compatibility
    # Wayland doesn't allow applications to detect their own window position changes
    
    # moveEvent removed - position tracking disabled for Wayland compatibility


def main():
    """Main entry point for GUI"""
    app = QApplication(sys.argv)
    app.setApplicationName("Clockwork Orange")
    app.setApplicationVersion("1.0")
    
    # Set application properties for better integration
    app.setQuitOnLastWindowClosed(False)  # Don't quit when window is closed (tray mode)
    
    # Simple single-instance check using process name
    import os
    import psutil
    current_pid = os.getpid()
    
    for proc in psutil.process_iter(['pid', 'name', 'cmdline']):
        try:
            if (proc.info['name'] == 'python3' and 
                proc.info['cmdline'] and 
                'clockwork-orange.py' in ' '.join(proc.info['cmdline']) and
                '--gui' in ' '.join(proc.info['cmdline']) and
                proc.info['pid'] != current_pid):
                print("[DEBUG] Another instance is already running")
                return 0
        except (psutil.NoSuchProcess, psutil.AccessDenied, psutil.ZombieProcess):
            pass
    
    # Check if system tray is available
    if not QSystemTrayIcon.isSystemTrayAvailable():
        QMessageBox.critical(None, "System Tray",
                           "System tray is not available on this system.")
        return 1
    
    window = ClockworkOrangeGUI()
    
    # Show tray icon
    if window.tray_icon:
        window.tray_icon.show()
        # Show initial message
        window.tray_icon.showMessage("Clockwork Orange", 
                                   "Application started. Click the tray icon to show/hide the window.",
                                   QSystemTrayIcon.MessageIcon.Information, 3000)
    
    # Show main window
    window.show()
    
    # Handle application exit properly
    try:
        return app.exec()
    except KeyboardInterrupt:
        # Handle Ctrl+C gracefully
        if window.tray_icon:
            window.tray_icon.hide()
        return 0
    finally:
        # Cleanup (no lock file to clean up)
        pass


if __name__ == "__main__":
    sys.exit(main())
