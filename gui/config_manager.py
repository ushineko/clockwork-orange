#!/usr/bin/env python3
"""
Configuration management GUI widget for clockwork-orange
"""

import yaml
from pathlib import Path
from PyQt6.QtWidgets import (QWidget, QVBoxLayout, QHBoxLayout, QPushButton, 
                             QLabel, QGroupBox, QCheckBox, QLineEdit, QSpinBox,
                             QFileDialog, QMessageBox, QFormLayout, QTabWidget,
                             QTextEdit, QComboBox)
from PyQt6.QtCore import Qt, pyqtSignal
from PyQt6.QtGui import QFont


class ConfigManagerWidget(QWidget):
    """Widget for managing clockwork-orange configuration"""
    
    config_changed = pyqtSignal()
    
    def __init__(self):
        super().__init__()
        self.config_path = Path.home() / ".config" / "clockwork-orange.yml"
        self.config_data = {}
        self.init_ui()
        self.load_config()
    
    def init_ui(self):
        """Initialize the UI"""
        layout = QVBoxLayout()
        
        # Configuration tabs
        self.tab_widget = QTabWidget()
        
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
        
        self.save_button = QPushButton("Save to File")
        self.save_button.clicked.connect(self.save_to_file)
        button_layout.addWidget(self.save_button)
        
        self.reset_button = QPushButton("Reset to Defaults")
        self.reset_button.clicked.connect(self.reset_to_defaults)
        button_layout.addWidget(self.reset_button)
        
        self.apply_button = QPushButton("Apply Configuration")
        self.apply_button.clicked.connect(self.apply_config)
        button_layout.addWidget(self.apply_button)
        
        layout.addLayout(button_layout)
        
        self.setLayout(layout)
    
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
        
        # Default directory
        dir_layout = QHBoxLayout()
        self.directory_edit = QLineEdit()
        self.directory_edit.setPlaceholderText("/path/to/wallpapers")
        dir_layout.addWidget(self.directory_edit)
        
        self.browse_dir_button = QPushButton("Browse...")
        self.browse_dir_button.clicked.connect(self.browse_directory)
        dir_layout.addWidget(self.browse_dir_button)
        
        layout.addRow("Default Directory:", dir_layout)
        
        # Default file
        file_layout = QHBoxLayout()
        self.file_edit = QLineEdit()
        self.file_edit.setPlaceholderText("/path/to/wallpaper.jpg")
        file_layout.addWidget(self.file_edit)
        
        self.browse_file_button = QPushButton("Browse...")
        self.browse_file_button.clicked.connect(self.browse_file)
        file_layout.addWidget(self.browse_file_button)
        
        layout.addRow("Default File:", file_layout)
        
        # Default URL
        self.url_edit = QLineEdit()
        self.url_edit.setPlaceholderText("https://pic.re/image")
        layout.addRow("Default URL:", self.url_edit)
        
        # Wait interval
        self.wait_spin = QSpinBox()
        self.wait_spin.setRange(1, 86400)  # 1 second to 24 hours
        self.wait_spin.setValue(300)
        self.wait_spin.setSuffix(" seconds")
        layout.addRow("Wait Interval:", self.wait_spin)
        
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
    
    def browse_directory(self):
        """Browse for wallpaper directory"""
        directory = QFileDialog.getExistingDirectory(self, "Select Wallpaper Directory")
        if directory:
            self.directory_edit.setText(directory)
    
    def browse_file(self):
        """Browse for wallpaper file"""
        file_path, _ = QFileDialog.getOpenFileName(
            self, "Select Wallpaper File", "", 
            "Image Files (*.jpg *.jpeg *.png *.bmp *.gif *.tiff *.webp *.svg);;All Files (*)"
        )
        if file_path:
            self.file_edit.setText(file_path)
    
    def load_config(self):
        """Load configuration from file"""
        try:
            if self.config_path.exists():
                with open(self.config_path, 'r') as f:
                    self.config_data = yaml.safe_load(f) or {}
            else:
                self.config_data = {}
            
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
        self.directory_edit.setText(self.config_data.get('default_directory', ''))
        self.file_edit.setText(self.config_data.get('default_file', ''))
        self.url_edit.setText(self.config_data.get('default_url', ''))
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
        if self.directory_edit.text():
            config['default_directory'] = self.directory_edit.text()
        if self.file_edit.text():
            config['default_file'] = self.file_edit.text()
        if self.url_edit.text():
            config['default_url'] = self.url_edit.text()
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
