#!/usr/bin/env python3
"""
Configuration management GUI widget for clockwork-orange
"""

import yaml
from pathlib import Path
from PyQt6.QtWidgets import (QWidget, QVBoxLayout, QHBoxLayout, QPushButton, 
                             QLabel, QGroupBox, QCheckBox, QLineEdit, QSpinBox,
                             QFileDialog, QMessageBox, QFormLayout, QTabWidget,
                             QTextEdit, QComboBox, QProgressBar)
from PyQt6.QtCore import Qt, pyqtSignal, QTimer
from PyQt6.QtGui import QFont
from .plugins_tab import PluginsTab
from .blacklist_tab import BlacklistTab


class ConfigManagerWidget(QWidget):
    """Widget for managing clockwork-orange configuration"""
    
    config_changed = pyqtSignal()
    
    def __init__(self):
        super().__init__()
        self.config_path = Path.home() / ".config" / "clockwork-orange.yml"
        self.config_data = {}
        self.auto_save_timer = QTimer()
        self.auto_save_timer.setSingleShot(True)
        self.auto_save_timer.setInterval(1000) # 1 second debounce
        self.auto_save_timer.timeout.connect(self.perform_auto_save)
        self.init_ui()
        self.load_config()
    
    def init_ui(self):
        """Initialize the UI"""
        layout = QVBoxLayout()
        


        # Configuration tabs
        self.tab_widget = QTabWidget()
        
        # Plugins tab
        self.plugins_tab = PluginsTab(self.config_data)
        self.plugins_tab.config_changed.connect(self.schedule_auto_save)
        self.tab_widget.addTab(self.plugins_tab, "Plugins")
        
        # Blacklist tab
        self.blacklist_tab = BlacklistTab()
        self.tab_widget.addTab(self.blacklist_tab, "Blacklist Manager")
        
        # Basic settings tab
        self.basic_tab = self.create_basic_tab()
        self.tab_widget.addTab(self.basic_tab, "Basic Settings")
        
        # Advanced settings tab
        self.advanced_tab = self.create_advanced_tab()
        self.tab_widget.addTab(self.advanced_tab, "Advanced Settings")
        
        # Raw YAML tab
        self.raw_tab = self.create_raw_tab()
        self.tab_widget.addTab(self.raw_tab, "Raw YAML")
        
        layout.addWidget(self.tab_widget)
        
        # Control buttons
        button_layout = QHBoxLayout()
        
        self.load_button = QPushButton("Load from File")
        self.load_button.clicked.connect(self.load_from_file)
        button_layout.addWidget(self.load_button)
        
        self.reset_button = QPushButton("Reset to Defaults")
        self.reset_button.clicked.connect(self.reset_to_defaults)
        button_layout.addWidget(self.reset_button)
        
        layout.addLayout(button_layout)
        
        # Progress Bar
        self.progress_bar = QProgressBar()
        self.progress_bar.setVisible(False)
        layout.addWidget(self.progress_bar)
        
        self.setLayout(layout)
        
    def update_progress(self, value: int, message: str = ""):
        """Update the progress bar."""
        if value < 0:
            self.progress_bar.setVisible(False)
        else:
            self.progress_bar.setVisible(True)
            self.progress_bar.setValue(value)
            
        if message:
            self.progress_bar.setFormat(f"%p% - {message}")
        else:
            self.progress_bar.setFormat("%p%")
    
    def schedule_auto_save(self):
        """Schedule an auto-save operation."""
        # Use status bar or progress bar to indicate pending save?
        # For now, just setting the timer
        self.auto_save_timer.start()
        
    def perform_auto_save(self):
        """Perform the auto-save operation (silent)."""
        try:
            config = self.get_config_from_ui()
            
            # Create config directory if it doesn't exist
            self.config_path.parent.mkdir(parents=True, exist_ok=True)
            
            # Save to default location
            with open(self.config_path, 'w') as f:
                yaml.dump(config, f, default_flow_style=False, sort_keys=True)
            
            self.config_data = config
            self.update_yaml_display()
            self.config_changed.emit()
            
            # Indicate saved status subtly
            if self.progress_bar.isVisible():
                # If progress bar is visible (plugin running), don't mess with it
                pass
            else:
                 # Perhaps flash a message somewhere? simpler to just update system tray signal if any
                 pass
            
            print("[DEBUG] Auto-saved configuration")
            
        except Exception as e:
            print(f"[ERROR] Auto-save failed: {e}")

    def create_basic_tab(self):
        """Create basic settings tab"""
        widget = QWidget()
        layout = QFormLayout()
        
        # Wallpaper mode
        self.dual_wallpapers_check = QCheckBox("Enable dual wallpapers (desktop + lock screen)")
        layout.addRow("Wallpaper Mode:", self.dual_wallpapers_check)
        
        self.desktop_only_check = QCheckBox("Desktop wallpaper only")
        layout.addRow("", self.desktop_only_check)
        
        self.lockscreen_only_check = QCheckBox("Lock screen wallpaper only")
        layout.addRow("", self.lockscreen_only_check)
        
        # Connect checkboxes to ensure mutual exclusivity
        self.dual_wallpapers_check.toggled.connect(self.on_dual_wallpapers_toggled)
        self.desktop_only_check.toggled.connect(self.on_desktop_only_toggled)
        self.lockscreen_only_check.toggled.connect(self.on_lockscreen_only_toggled)
        
        # Wait interval
        self.wait_spin = QSpinBox()
        self.wait_spin.setRange(1, 86400)  # 1 second to 24 hours
        self.wait_spin.setValue(300)
        self.wait_spin.setSuffix(" seconds")
        self.wait_spin.valueChanged.connect(self.schedule_auto_save)
        layout.addRow("Wait Interval:", self.wait_spin)
        
        # Connect checkboxes to auto-save
        self.dual_wallpapers_check.toggled.connect(self.schedule_auto_save)
        self.desktop_only_check.toggled.connect(self.schedule_auto_save)
        self.lockscreen_only_check.toggled.connect(self.schedule_auto_save)
        
        widget.setLayout(layout)
        return widget
    
    def create_advanced_tab(self):
        """Create advanced settings tab"""
        widget = QWidget()
        layout = QFormLayout()
        
        # Image extensions
        self.extensions_edit = QLineEdit()
        self.extensions_edit.setText(".jpg,.jpeg,.png,.bmp,.gif,.tiff,.webp,.svg")
        self.extensions_edit.setPlaceholderText("Comma-separated extensions")
        layout.addRow("Image Extensions:", self.extensions_edit)
        
        # Debug mode
        self.debug_check = QCheckBox("Enable debug logging")
        layout.addRow("Debug Mode:", self.debug_check)
        
        # Auto-start
        self.autostart_check = QCheckBox("Start service automatically on login")
        layout.addRow("Auto-start:", self.autostart_check)
        
        # Service restart delay
        self.restart_delay_spin = QSpinBox()
        self.restart_delay_spin.setRange(1, 300)
        self.restart_delay_spin.setValue(10)
        self.restart_delay_spin.setSuffix(" seconds")
        layout.addRow("Restart Delay:", self.restart_delay_spin)
        
        # Logs refresh interval
        self.logs_refresh_spin = QSpinBox()
        self.logs_refresh_spin.setRange(1, 300)
        self.logs_refresh_spin.setValue(5)
        self.logs_refresh_spin.setSuffix(" seconds")
        layout.addRow("Logs Refresh Interval:", self.logs_refresh_spin)
        
        self.auto_update_logs_check = QCheckBox("Auto-update service logs")
        layout.addRow("Auto-update Logs:", self.auto_update_logs_check)
        
        # Window size settings
        window_layout = QHBoxLayout()
        
        self.window_width_spin = QSpinBox()
        self.window_width_spin.setRange(400, 9999)
        self.window_width_spin.setValue(800)
        window_layout.addWidget(QLabel("Width:"))
        window_layout.addWidget(self.window_width_spin)
        
        self.window_height_spin = QSpinBox()
        self.window_height_spin.setRange(300, 9999)
        self.window_height_spin.setValue(600)
        window_layout.addWidget(QLabel("Height:"))
        window_layout.addWidget(self.window_height_spin)
        
        layout.addRow("Window Size:", window_layout)
        
        # Connect signals to auto-save
        self.extensions_edit.textChanged.connect(self.schedule_auto_save)
        self.debug_check.toggled.connect(self.schedule_auto_save)
        self.autostart_check.toggled.connect(self.schedule_auto_save)
        self.restart_delay_spin.valueChanged.connect(self.schedule_auto_save)
        self.logs_refresh_spin.valueChanged.connect(self.schedule_auto_save)
        self.auto_update_logs_check.toggled.connect(self.schedule_auto_save)
        self.window_width_spin.valueChanged.connect(self.schedule_auto_save)
        self.window_height_spin.valueChanged.connect(self.schedule_auto_save)
        
        widget.setLayout(layout)
        return widget
    
    def create_raw_tab(self):
        """Create raw YAML editing tab"""
        widget = QWidget()
        layout = QVBoxLayout()
        
        self.yaml_edit = QTextEdit()
        self.yaml_edit.setFont(QFont("Courier", 10))
        layout.addWidget(self.yaml_edit)
        
        # Raw YAML buttons
        raw_button_layout = QHBoxLayout()
        
        self.validate_yaml_button = QPushButton("Validate YAML")
        self.validate_yaml_button.clicked.connect(self.validate_yaml)
        raw_button_layout.addWidget(self.validate_yaml_button)
        
        self.format_yaml_button = QPushButton("Format YAML")
        self.format_yaml_button.clicked.connect(self.format_yaml)
        raw_button_layout.addWidget(self.format_yaml_button)
        
        layout.addLayout(raw_button_layout)
        
        widget.setLayout(layout)
        return widget
    
    def on_dual_wallpapers_toggled(self, checked):
        """Handle dual wallpapers checkbox toggle"""
        if checked:
            self.desktop_only_check.setChecked(False)
            self.lockscreen_only_check.setChecked(False)
    
    def on_desktop_only_toggled(self, checked):
        """Handle desktop only checkbox toggle"""
        if checked:
            self.dual_wallpapers_check.setChecked(False)
            self.lockscreen_only_check.setChecked(False)
    
    def on_lockscreen_only_toggled(self, checked):
        """Handle lock screen only checkbox toggle"""
        if checked:
            self.dual_wallpapers_check.setChecked(False)
            self.desktop_only_check.setChecked(False)
    
    def load_config(self):
        """Load configuration from file"""
        try:
            if self.config_path.exists():
                with open(self.config_path, 'r') as f:
                    self.config_data = yaml.safe_load(f) or {}
            else:
                self.config_data = {}
            
            # Propagate config to tabs
            if hasattr(self, 'plugins_tab'):
                self.plugins_tab.set_config(self.config_data)
            
            self.update_ui_from_config()
            self.update_yaml_display()
            
        except Exception as e:
            QMessageBox.critical(self, "Error", f"Failed to load configuration: {e}")

    def update_ui_from_config(self):
        """Update UI elements from configuration data"""
        # Wallpaper mode
        if self.config_data.get('dual_wallpapers'):
            self.dual_wallpapers_check.setChecked(True)
        elif self.config_data.get('desktop'):
            self.desktop_only_check.setChecked(True)
        elif self.config_data.get('lockscreen'):
            self.lockscreen_only_check.setChecked(True)
        
        # Default values
        self.wait_spin.setValue(self.config_data.get('default_wait', 300))
        
        # Advanced settings
        extensions = self.config_data.get('image_extensions', '.jpg,.jpeg,.png,.bmp,.gif,.tiff,.webp,.svg')
        self.extensions_edit.setText(extensions)
        self.debug_check.setChecked(self.config_data.get('debug', False))
        self.autostart_check.setChecked(self.config_data.get('autostart', False))
        self.restart_delay_spin.setValue(self.config_data.get('restart_delay', 10))
        self.logs_refresh_spin.setValue(self.config_data.get('logs_refresh_interval', 5))
        self.auto_update_logs_check.setChecked(self.config_data.get('auto_update_logs', False))
        
        # Window size settings
        self.window_width_spin.setValue(self.config_data.get('window_width', 800))
        self.window_height_spin.setValue(self.config_data.get('window_height', 600))
        
        # Update plugins tab
        if hasattr(self, 'plugins_tab'):
            self.plugins_tab.update_config_data(self.config_data)
    
    def update_yaml_display(self):
        """Update YAML display from current config"""
        try:
            yaml_text = yaml.dump(self.config_data, default_flow_style=False, sort_keys=True)
            self.yaml_edit.setText(yaml_text)
        except Exception as e:
            self.yaml_edit.setText(f"Error formatting YAML: {e}")
    
    def get_config_from_ui(self):
        """Get configuration data from UI elements"""
        config = {}
        
        # Wallpaper mode
        if self.dual_wallpapers_check.isChecked():
            config['dual_wallpapers'] = True
        elif self.desktop_only_check.isChecked():
            config['desktop'] = True
        elif self.lockscreen_only_check.isChecked():
            config['lockscreen'] = True
        
        # Default values
        if self.wait_spin.value() != 300:
            config['default_wait'] = self.wait_spin.value()
        
        # Advanced settings
        if self.extensions_edit.text() != '.jpg,.jpeg,.png,.bmp,.gif,.tiff,.webp,.svg':
            config['image_extensions'] = self.extensions_edit.text()
        if self.debug_check.isChecked():
            config['debug'] = True
        if self.autostart_check.isChecked():
            config['autostart'] = True
        if self.restart_delay_spin.value() != 10:
            config['restart_delay'] = self.restart_delay_spin.value()
        if self.logs_refresh_spin.value() != 5:
            config['logs_refresh_interval'] = self.logs_refresh_spin.value()
        if self.auto_update_logs_check.isChecked():
            config['auto_update_logs'] = True
        
        # Window size settings
        if self.window_width_spin.value() != 800:
            config['window_width'] = self.window_width_spin.value()
        if self.window_height_spin.value() != 600:
            config['window_height'] = self.window_height_spin.value()
            
        # Plugin settings
        if hasattr(self, 'plugins_tab'):
            config['plugins'] = self.plugins_tab.get_config()
            config['active_plugin'] = self.plugins_tab.get_active_plugin()
            
            # Clean up invalid plugins
            available = self.plugins_tab.plugin_manager.get_available_plugins()
            valid_plugins = {}
            for name, data in config['plugins'].items():
                if name in available:
                    valid_plugins[name] = data
            config['plugins'] = valid_plugins
        
        return config
    
    def load_from_file(self):
        """Load configuration from a file"""
        file_path, _ = QFileDialog.getOpenFileName(
            self, "Load Configuration", "", "YAML Files (*.yml *.yaml);;All Files (*)"
        )
        if file_path:
            try:
                with open(file_path, 'r') as f:
                    self.config_data = yaml.safe_load(f) or {}
                self.update_ui_from_config()
                self.update_yaml_display()
                QMessageBox.information(self, "Success", "Configuration loaded successfully!")
            except Exception as e:
                QMessageBox.critical(self, "Error", f"Failed to load configuration: {e}")
    
    def save_to_file(self):
        """Save configuration to a file"""
        file_path, _ = QFileDialog.getSaveFileName(
            self, "Save Configuration", "clockwork-orange.yml", "YAML Files (*.yml *.yaml);;All Files (*)"
        )
        if file_path:
            try:
                config = self.get_config_from_ui()
                with open(file_path, 'w') as f:
                    yaml.dump(config, f, default_flow_style=False, sort_keys=True)
                QMessageBox.information(self, "Success", "Configuration saved successfully!")
            except Exception as e:
                QMessageBox.critical(self, "Error", f"Failed to save configuration: {e}")
    
    def reset_to_defaults(self):
        """Reset configuration to defaults"""
        reply = QMessageBox.question(self, "Reset Configuration", 
                                   "Are you sure you want to reset to default configuration?",
                                   QMessageBox.StandardButton.Yes | QMessageBox.StandardButton.No)
        if reply == QMessageBox.StandardButton.Yes:
            self.config_data = {}
            self.update_ui_from_config()
            self.update_yaml_display()
    
    def apply_config(self):
        """Apply current configuration"""
        try:
            config = self.get_config_from_ui()
            
            # Create config directory if it doesn't exist
            self.config_path.parent.mkdir(parents=True, exist_ok=True)
            
            # Save to default location
            with open(self.config_path, 'w') as f:
                yaml.dump(config, f, default_flow_style=False, sort_keys=True)
            
            self.config_data = config
            self.update_yaml_display()
            self.config_changed.emit()
            
            QMessageBox.information(self, "Success", "Configuration applied successfully!")
            
        except Exception as e:
            QMessageBox.critical(self, "Error", f"Failed to apply configuration: {e}")
    
    def validate_yaml(self):
        """Validate YAML syntax"""
        try:
            yaml.safe_load(self.yaml_edit.toPlainText())
            QMessageBox.information(self, "Success", "YAML syntax is valid!")
        except yaml.YAMLError as e:
            QMessageBox.critical(self, "YAML Error", f"Invalid YAML syntax: {e}")
    
    def format_yaml(self):
        """Format YAML text"""
        try:
            data = yaml.safe_load(self.yaml_edit.toPlainText())
            formatted = yaml.dump(data, default_flow_style=False, sort_keys=True)
            self.yaml_edit.setText(formatted)
            QMessageBox.information(self, "Success", "YAML formatted successfully!")
        except yaml.YAMLError as e:
            QMessageBox.critical(self, "YAML Error", f"Invalid YAML syntax: {e}")
