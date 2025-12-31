#!/usr/bin/env python3
"""
Settings widgets for clockwork-orange.
Extracted from legacy config_manager.py.
"""

import yaml
from PyQt6.QtCore import pyqtSignal
from PyQt6.QtGui import QFont
from PyQt6.QtWidgets import (
    QCheckBox,
    QFontComboBox,
    QFormLayout,
    QHBoxLayout,
    QLineEdit,
    QMessageBox,
    QPushButton,
    QSpinBox,
    QTextEdit,
    QVBoxLayout,
    QWidget,
)


class BasicSettingsWidget(QWidget):
    """Widget for basic settings"""

    setting_changed = pyqtSignal()

    def __init__(self, config_data):
        super().__init__()
        self.config_data = config_data
        self.init_ui()
        self.update_from_config()

    def init_ui(self):
        layout = QFormLayout()

        # Wallpaper mode
        self.dual_wallpapers_check = QCheckBox(
            "Enable dual wallpapers (desktop + lock screen)"
        )
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
        layout.addRow("Wait Interval:", self.wait_spin)

        # Connect signals to change emission
        self.dual_wallpapers_check.toggled.connect(self.setting_changed.emit)
        self.desktop_only_check.toggled.connect(self.setting_changed.emit)
        self.lockscreen_only_check.toggled.connect(self.setting_changed.emit)
        self.wait_spin.valueChanged.connect(self.setting_changed.emit)

        # Console Font settings
        self.font_combo = QFontComboBox()
        self.font_combo.setCurrentFont(QFont("Monospace"))
        layout.addRow("Console Font:", self.font_combo)

        self.font_size_spin = QSpinBox()
        self.font_size_spin.setRange(6, 48)
        self.font_size_spin.setValue(10)
        layout.addRow("Console Font Size:", self.font_size_spin)

        self.font_combo.currentFontChanged.connect(self.setting_changed.emit)
        self.font_size_spin.valueChanged.connect(self.setting_changed.emit)

        self.setLayout(layout)

    def on_dual_wallpapers_toggled(self, checked):
        if checked:
            self.desktop_only_check.setChecked(False)
            self.lockscreen_only_check.setChecked(False)

    def on_desktop_only_toggled(self, checked):
        if checked:
            self.dual_wallpapers_check.setChecked(False)
            self.lockscreen_only_check.setChecked(False)

    def on_lockscreen_only_toggled(self, checked):
        if checked:
            self.dual_wallpapers_check.setChecked(False)
            self.desktop_only_check.setChecked(False)

    def update_from_config(self):
        """Update UI from config data"""
        if self.config_data.get("dual_wallpapers"):
            self.dual_wallpapers_check.setChecked(True)
        elif self.config_data.get("desktop"):
            self.desktop_only_check.setChecked(True)
        elif self.config_data.get("lockscreen"):
            self.lockscreen_only_check.setChecked(True)

        self.wait_spin.setValue(self.config_data.get("default_wait", 300))

        font_family = self.config_data.get("console_font_family")
        if font_family:
            self.font_combo.setCurrentFont(QFont(font_family))

        self.font_size_spin.setValue(self.config_data.get("console_font_size", 10))

    def get_config(self):
        """Get config dictionary from UI"""
        config = {}
        if self.dual_wallpapers_check.isChecked():
            config["dual_wallpapers"] = True
        elif self.desktop_only_check.isChecked():
            config["desktop"] = True
        elif self.lockscreen_only_check.isChecked():
            config["lockscreen"] = True

        if self.wait_spin.value() != 300:
            config["default_wait"] = self.wait_spin.value()

        config["console_font_family"] = self.font_combo.currentFont().family()
        config["console_font_size"] = self.font_size_spin.value()

        return config


class AdvancedSettingsWidget(QWidget):
    """Widget for advanced settings"""

    setting_changed = pyqtSignal()

    def __init__(self, config_data):
        super().__init__()
        self.config_data = config_data
        self.init_ui()
        self.update_from_config()

    def init_ui(self):
        layout = QFormLayout()

        # Image extensions
        self.extensions_edit = QLineEdit()
        self.extensions_edit.setPlaceholderText("Comma-separated extensions")
        layout.addRow("Image Extensions:", self.extensions_edit)

        # Debug mode
        self.debug_check = QCheckBox("Enable debug logging")
        layout.addRow("Debug Mode:", self.debug_check)

        # Service-related settings (Linux only)
        import platform_utils
        if not platform_utils.is_windows():
            # Auto-start
            self.autostart_check = QCheckBox("Start service automatically on login")
            layout.addRow("Auto-start:", self.autostart_check)

            # Service restart delay
            self.restart_delay_spin = QSpinBox()
            self.restart_delay_spin.setRange(1, 300)
            self.restart_delay_spin.setSuffix(" seconds")
            layout.addRow("Restart Delay:", self.restart_delay_spin)

            # Logs refresh interval
            self.logs_refresh_spin = QSpinBox()
            self.logs_refresh_spin.setRange(1, 300)
            self.logs_refresh_spin.setSuffix(" seconds")
            layout.addRow("Logs Refresh Interval:", self.logs_refresh_spin)

            self.auto_update_logs_check = QCheckBox("Auto-update service logs")
            layout.addRow("Auto-update Logs:", self.auto_update_logs_check)
            
            # Connect service-related signals
            self.autostart_check.toggled.connect(self.setting_changed.emit)
            self.restart_delay_spin.valueChanged.connect(self.setting_changed.emit)
            self.logs_refresh_spin.valueChanged.connect(self.setting_changed.emit)
            self.auto_update_logs_check.toggled.connect(self.setting_changed.emit)
        else:
            # Set to None on Windows so update_from_config doesn't crash
            self.autostart_check = None
            self.restart_delay_spin = None
            self.logs_refresh_spin = None
            self.auto_update_logs_check = None

        # Connect common signals
        self.extensions_edit.textChanged.connect(self.setting_changed.emit)
        self.debug_check.toggled.connect(self.setting_changed.emit)

        self.setLayout(layout)


    def update_from_config(self):
        extensions = self.config_data.get(
            "image_extensions", ".jpg,.jpeg,.png,.bmp,.gif,.tiff,.webp,.svg"
        )
        self.extensions_edit.setText(extensions)
        self.debug_check.setChecked(self.config_data.get("debug", False))
        
        # Service settings (Linux only)
        if self.autostart_check:
            self.autostart_check.setChecked(self.config_data.get("autostart", False))
        if self.restart_delay_spin:
            self.restart_delay_spin.setValue(self.config_data.get("restart_delay", 10))
        if self.logs_refresh_spin:
            self.logs_refresh_spin.setValue(
                self.config_data.get("logs_refresh_interval", 5)
            )
        if self.auto_update_logs_check:
            self.auto_update_logs_check.setChecked(
                self.config_data.get("auto_update_logs", False)
            )


    def get_config(self):
        config = {}
        if self.extensions_edit.text() != ".jpg,.jpeg,.png,.bmp,.gif,.tiff,.webp,.svg":
            config["image_extensions"] = self.extensions_edit.text()
        if self.debug_check.isChecked():
            config["debug"] = True
            
        # Service settings (Linux only)
        if self.autostart_check and self.autostart_check.isChecked():
            config["autostart"] = True
        if self.restart_delay_spin and self.restart_delay_spin.value() != 10:
            config["restart_delay"] = self.restart_delay_spin.value()
        if self.logs_refresh_spin and self.logs_refresh_spin.value() != 5:
            config["logs_refresh_interval"] = self.logs_refresh_spin.value()
        if self.auto_update_logs_check and self.auto_update_logs_check.isChecked():
            config["auto_update_logs"] = True

        return config


class YamlEditorWidget(QWidget):
    """Widget for raw YAML editing"""

    def __init__(self, config_data):
        super().__init__()
        self.config_data = config_data
        self.init_ui()
        self.update_display()

    def init_ui(self):
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
        self.setLayout(layout)

    def update_display(self):
        try:
            yaml_text = yaml.dump(
                self.config_data, default_flow_style=False, sort_keys=True
            )
            self.yaml_edit.setText(yaml_text)
        except Exception as e:
            self.yaml_edit.setText(f"Error formatting YAML: {e}")

    def update_data(self, config_data):
        self.config_data = config_data
        self.update_display()

    def validate_yaml(self):
        try:
            yaml.safe_load(self.yaml_edit.toPlainText())
            QMessageBox.information(self, "Success", "YAML syntax is valid!")
        except yaml.YAMLError as e:
            QMessageBox.critical(self, "YAML Error", f"Invalid YAML syntax: {e}")

    def format_yaml(self):
        try:
            data = yaml.safe_load(self.yaml_edit.toPlainText())
            formatted = yaml.dump(data, default_flow_style=False, sort_keys=True)
            self.yaml_edit.setText(formatted)
            QMessageBox.information(self, "Success", "YAML formatted successfully!")
        except yaml.YAMLError as e:
            QMessageBox.critical(self, "YAML Error", f"Invalid YAML syntax: {e}")
