#!/usr/bin/env python3
"""
Service management GUI widget for clockwork-orange
"""

import subprocess
from pathlib import Path

from PyQt6.QtCore import QThread, QTimer, pyqtSignal
from PyQt6.QtGui import QFont
from PyQt6.QtWidgets import (
    QCheckBox,
    QGroupBox,
    QHBoxLayout,
    QLabel,
    QMessageBox,
    QProgressBar,
    QPushButton,
    QSpinBox,
    QTextEdit,
    QVBoxLayout,
    QWidget,
)

import platform_utils


class ServiceStatusThread(QThread):
    """Thread for checking service status"""

    status_updated = pyqtSignal(str, str)  # status, details

    def run(self):
        """Check service status"""
        try:
            status = platform_utils.service_is_active()
            details = platform_utils.service_get_status_details()
            self.status_updated.emit(status, details)
        except Exception as e:
            self.status_updated.emit("error", f"Error checking status: {e}")


class ServiceManagerWidget(QWidget):
    """Widget for managing the clockwork-orange service"""

    status_changed = pyqtSignal(str)

    def __init__(self):
        super().__init__()
        self.status_thread = None
        self.init_ui()
        self.refresh_status()

        # Auto-refresh status every 5 seconds
        self.refresh_timer = QTimer()
        self.refresh_timer.timeout.connect(self.refresh_status)
        self.refresh_timer.start(5000)

    def init_ui(self):
        """Initialize the UI"""
        layout = QVBoxLayout()

        # Service status group
        status_group = QGroupBox("Service Status")
        status_layout = QVBoxLayout()

        self.status_label = QLabel("Checking status...")
        self.status_label.setFont(QFont("Arial", 12, QFont.Weight.Bold))
        status_layout.addWidget(self.status_label)

        self.status_details = QTextEdit()
        self.status_details.setReadOnly(True)
        self.status_details.setMinimumHeight(
            150
        )  # Reduced height to give more space to logs
        status_layout.addWidget(self.status_details)

        status_group.setLayout(status_layout)
        layout.addWidget(status_group, 1)  # Give status group stretch factor of 1

        # Service control group
        control_group = QGroupBox("Service Control")
        control_layout = QVBoxLayout()

        # Button layout
        button_layout = QHBoxLayout()

        self.start_button = QPushButton("Start Service")
        self.start_button.clicked.connect(self.start_service)
        button_layout.addWidget(self.start_button)

        self.stop_button = QPushButton("Stop Service")
        self.stop_button.clicked.connect(self.stop_service)
        button_layout.addWidget(self.stop_button)

        self.restart_button = QPushButton("Restart Service")
        self.restart_button.clicked.connect(self.restart_service)
        button_layout.addWidget(self.restart_button)

        control_layout.addLayout(button_layout)

        # Service management buttons
        mgmt_layout = QHBoxLayout()

        self.install_button = QPushButton("Install Service")
        self.install_button.clicked.connect(self.install_service)
        self.install_button.setToolTip(
            "Install the service (only available when service is stopped)"
        )
        mgmt_layout.addWidget(self.install_button)

        self.uninstall_button = QPushButton("Uninstall Service")
        self.uninstall_button.clicked.connect(self.uninstall_service)
        self.uninstall_button.setToolTip(
            "Uninstall the service (only available when service is stopped)"
        )
        mgmt_layout.addWidget(self.uninstall_button)

        control_layout.addLayout(mgmt_layout)

        # Progress bar
        self.progress_bar = QProgressBar()
        self.progress_bar.setVisible(False)
        control_layout.addWidget(self.progress_bar)

        control_group.setLayout(control_layout)
        layout.addWidget(control_group, 0)  # Give control group no stretch (fixed size)

        # Logs group
        logs_group = QGroupBox("Service Logs")
        logs_layout = QVBoxLayout()

        # Logs controls
        logs_controls_layout = QHBoxLayout()

        self.auto_update_check = QCheckBox("Auto-update logs")
        self.auto_update_check.toggled.connect(self.toggle_auto_update)
        self.auto_update_check.toggled.connect(self.save_auto_update_state)
        logs_controls_layout.addWidget(self.auto_update_check)

        logs_controls_layout.addWidget(QLabel("Refresh interval:"))

        self.refresh_interval_spin = QSpinBox()
        self.refresh_interval_spin.setRange(1, 300)  # 1 second to 5 minutes
        self.refresh_interval_spin.setValue(5)
        self.refresh_interval_spin.setSuffix(" seconds")
        self.refresh_interval_spin.valueChanged.connect(self.update_refresh_interval)
        logs_controls_layout.addWidget(self.refresh_interval_spin)

        logs_controls_layout.addStretch()  # Push controls to the left

        refresh_logs_button = QPushButton("Refresh Now")
        refresh_logs_button.clicked.connect(self.refresh_logs)
        logs_controls_layout.addWidget(refresh_logs_button)

        logs_layout.addLayout(logs_controls_layout)

        self.logs_text = QTextEdit()
        self.logs_text.setReadOnly(True)
        self.logs_text.setMinimumHeight(300)  # Much larger minimum height for logs
        logs_layout.addWidget(self.logs_text)

        logs_group.setLayout(logs_layout)
        layout.addWidget(
            logs_group, 2
        )  # Give logs group stretch factor of 2 (more space)

        self.setLayout(layout)

        # Auto-update timer for logs
        self.logs_timer = QTimer()
        self.logs_timer.timeout.connect(self.refresh_logs)
        self.auto_update_enabled = False

        # Load configuration
        self.load_config()

    def load_config(self):
        """Load configuration from file"""
        try:
            from pathlib import Path

            import yaml

            config_path = Path.home() / ".config" / "clockwork-orange.yml"
            if config_path.exists():
                with open(config_path, "r") as f:
                    config = yaml.safe_load(f) or {}

                # Load logs refresh interval
                logs_refresh = config.get("logs_refresh_interval", 5)
                self.refresh_interval_spin.setValue(logs_refresh)

                # Load auto-update state
                auto_update = config.get("auto_update_logs", False)
                self.auto_update_check.setChecked(auto_update)
                if auto_update:
                    self.toggle_auto_update(True)

        except Exception as e:
            print(f"[DEBUG] Failed to load configuration: {e}")

    def save_auto_update_state(self, enabled):
        """Save auto-update state to configuration file"""
        try:
            from pathlib import Path

            import yaml

            config_path = Path.home() / ".config" / "clockwork-orange.yml"
            config = {}

            # Load existing config if it exists
            if config_path.exists():
                with open(config_path, "r") as f:
                    config = yaml.safe_load(f) or {}

            # Update auto-update state
            if enabled:
                config["auto_update_logs"] = True
            else:
                config.pop("auto_update_logs", None)  # Remove if False (default)

            # Save config
            config_path.parent.mkdir(parents=True, exist_ok=True)
            with open(config_path, "w") as f:
                yaml.dump(config, f, default_flow_style=False, sort_keys=True)

        except Exception as e:
            print(f"[DEBUG] Failed to save auto-update state: {e}")

    def refresh_status(self):
        """Refresh service status"""
        if self.status_thread and self.status_thread.isRunning():
            return

        self.status_thread = ServiceStatusThread()
        self.status_thread.status_updated.connect(self.update_status)
        self.status_thread.start()

    def update_status(self, status, details):
        """Update status display"""
        if status == "active":
            self.status_label.setText("🟢 Service is Running")
            self.status_label.setStyleSheet("color: green;")
            self.start_button.setEnabled(False)
            self.stop_button.setEnabled(True)
            self.restart_button.setEnabled(True)
            # Disable install/uninstall when service is running
            self.install_button.setEnabled(False)
            self.uninstall_button.setEnabled(False)
            self.install_button.setToolTip(
                "Install service (disabled - service is running)"
            )
            self.uninstall_button.setToolTip(
                "Uninstall service (disabled - service is running)"
            )
        elif status == "inactive":
            self.status_label.setText("🔴 Service is Stopped")
            self.status_label.setStyleSheet("color: red;")
            self.start_button.setEnabled(True)
            self.stop_button.setEnabled(False)
            self.restart_button.setEnabled(False)
            # Enable install/uninstall when service is stopped
            self.install_button.setEnabled(True)
            self.uninstall_button.setEnabled(True)
            self.install_button.setToolTip("Install the service")
            self.uninstall_button.setToolTip("Uninstall the service")
        elif status == "activating":
            self.status_label.setText("🟡 Service is Starting")
            self.status_label.setStyleSheet("color: #FFC107;")  # Amber
            self.start_button.setEnabled(False)
            self.stop_button.setEnabled(True)
            self.restart_button.setEnabled(False)
            self.install_button.setEnabled(False)
            self.uninstall_button.setEnabled(False)
        elif status == "deactivating":
            self.status_label.setText("🟡 Service is Stopping")
            self.status_label.setStyleSheet("color: #FFC107;")
            self.start_button.setEnabled(False)
            self.stop_button.setEnabled(False)
            self.restart_button.setEnabled(False)
            # Probably wait until inactive
            self.install_button.setEnabled(False)
            self.uninstall_button.setEnabled(False)
        elif status == "failed":
            self.status_label.setText("⚠️ Service Failed")
            self.status_label.setStyleSheet("color: orange;")
            self.start_button.setEnabled(True)
            self.stop_button.setEnabled(True)
            self.restart_button.setEnabled(True)
            # Enable install/uninstall when service has failed
            self.install_button.setEnabled(True)
            self.uninstall_button.setEnabled(True)
            self.install_button.setToolTip("Install the service")
            self.uninstall_button.setToolTip("Uninstall the service")
        else:
            self.status_label.setText("❓ Service Status Unknown")
            self.status_label.setStyleSheet("color: gray;")
            self.start_button.setEnabled(True)
            self.stop_button.setEnabled(True)
            self.restart_button.setEnabled(True)
            # Enable install/uninstall when status is unknown
            self.install_button.setEnabled(True)
            self.uninstall_button.setEnabled(True)
            self.install_button.setToolTip("Install the service")
            self.uninstall_button.setToolTip("Uninstall the service")

        self.status_details.setText(details)
        self.status_changed.emit(status)

    def start_service(self):
        """Start the service"""
        self._sync_config_to_public()
        self._wrap_service_action(platform_utils.service_start, "Starting service...")

    def stop_service(self):
        """Stop the service"""
        self._wrap_service_action(platform_utils.service_stop, "Stopping service...")

    def restart_service(self):
        """Restart the service"""
        self._sync_config_to_public()
        self._wrap_service_action(
            platform_utils.service_restart, "Restarting service..."
        )

    def install_service(self):
        """Install the service"""
        try:
            self._sync_config_to_public()
            base_path = Path(__file__).parent.parent
            platform_utils.service_install(base_path)
            QMessageBox.information(
                self, "Success", "Service installed and enabled successfully!"
            )
            self.refresh_status()

        except Exception as e:
            QMessageBox.critical(self, "Error", f"Failed to install service: {e}")

    def _sync_config_to_public(self):
        """Copy user config to public location for service access."""
        if not platform_utils.is_windows():
            return

        try:
            user_config = Path.home() / ".config" / "clockwork-orange.yml"
            public_config = Path("C:/Users/Public/clockwork_config.yml")

            if user_config.exists():
                import shutil

                shutil.copy2(user_config, public_config)
                print(f"[DEBUG] Synced config to {public_config}")
        except Exception as e:
            print(f"[ERROR] Failed to sync config: {e}")
            # Don't show popup as this is background op, just log

    def uninstall_service(self):
        """Uninstall the service"""
        try:
            platform_utils.service_uninstall()
            QMessageBox.information(
                self, "Success", "Service uninstalled successfully!"
            )
            self.refresh_status()

        except Exception as e:
            QMessageBox.critical(self, "Error", f"Failed to uninstall service: {e}")

    def _wrap_service_action(self, action_func, message):
        """Run a service action with progress indication"""
        self.progress_bar.setVisible(True)
        self.progress_bar.setRange(0, 0)  # Indeterminate progress

        try:
            action_func()
            # Success logic handled by caller refreshing usually, but here we just return
        except Exception as e:
            QMessageBox.critical(self, "Error", f"Failed to {message.lower()}: {e}")
        finally:
            self.progress_bar.setVisible(False)
            self.refresh_status()

    def toggle_auto_update(self, enabled):
        """Toggle auto-update for logs"""
        self.auto_update_enabled = enabled
        if enabled:
            self.logs_timer.start(self.refresh_interval_spin.value() * 1000)
            # Immediately refresh logs when enabling auto-update
            self.refresh_logs()
        else:
            self.logs_timer.stop()

    def update_refresh_interval(self, seconds):
        """Update the refresh interval for auto-update"""
        if self.auto_update_enabled:
            self.logs_timer.stop()
            self.logs_timer.start(seconds * 1000)

    def refresh_logs(self):
        """Refresh service logs"""
        try:
            # Use Public logs path on Windows
            if platform_utils.is_windows():
                log_path = Path("C:/Users/Public/clockwork_service.log")
                if log_path.exists():
                    with open(log_path, "r") as f:
                        logs = f.read()[-10000:]  # Last 10k chars
                else:
                    logs = "Log file not found."
            else:
                logs = platform_utils.service_get_logs()

            if not logs or not logs.strip():

                self.logs_text.setText("No logs found.")
            else:
                self.logs_text.setText(logs)

                # Store current scroll position
                scrollbar = self.logs_text.verticalScrollBar()
                was_at_bottom = scrollbar.value() >= scrollbar.maximum() - 10

                # Auto-scroll to bottom if auto-update is enabled or if user was already at bottom
                if self.auto_update_enabled or was_at_bottom:
                    scrollbar.setValue(scrollbar.maximum())

        except Exception as e:
            self.logs_text.setText(f"Error retrieving logs: {e}")
