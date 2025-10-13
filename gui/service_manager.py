#!/usr/bin/env python3
"""
Service management GUI widget for clockwork-orange
"""

import subprocess
import os
from pathlib import Path
from PyQt6.QtWidgets import (QWidget, QVBoxLayout, QHBoxLayout, QPushButton, 
                             QLabel, QTextEdit, QGroupBox, QMessageBox, QProgressBar)
from PyQt6.QtCore import Qt, QThread, pyqtSignal, QTimer
from PyQt6.QtGui import QFont


class ServiceStatusThread(QThread):
    """Thread for checking service status"""
    status_updated = pyqtSignal(str, str)  # status, details
    
    def run(self):
        """Check service status"""
        try:
            result = subprocess.run(['systemctl', '--user', 'is-active', 'clockwork-orange.service'],
                                  capture_output=True, text=True, timeout=5)
            status = result.stdout.strip()
            
            # Get more detailed status
            detail_result = subprocess.run(['systemctl', '--user', 'status', 'clockwork-orange.service', '--no-pager'],
                                         capture_output=True, text=True, timeout=5)
            details = detail_result.stdout
            
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
        self.status_details.setMinimumHeight(200)  # Set minimum height instead of maximum
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
        self.install_button.setToolTip("Install the service (only available when service is stopped)")
        mgmt_layout.addWidget(self.install_button)
        
        self.uninstall_button = QPushButton("Uninstall Service")
        self.uninstall_button.clicked.connect(self.uninstall_service)
        self.uninstall_button.setToolTip("Uninstall the service (only available when service is stopped)")
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
        
        self.logs_text = QTextEdit()
        self.logs_text.setReadOnly(True)
        self.logs_text.setMinimumHeight(100)  # Set minimum height instead of maximum
        logs_layout.addWidget(self.logs_text)
        
        refresh_logs_button = QPushButton("Refresh Logs")
        refresh_logs_button.clicked.connect(self.refresh_logs)
        logs_layout.addWidget(refresh_logs_button)
        
        logs_group.setLayout(logs_layout)
        layout.addWidget(logs_group, 1)  # Give logs group stretch factor of 1
        
        self.setLayout(layout)
    
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
            self.install_button.setToolTip("Install service (disabled - service is running)")
            self.uninstall_button.setToolTip("Uninstall service (disabled - service is running)")
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
        self._run_service_command(['systemctl', '--user', 'start', 'clockwork-orange.service'], "Starting service...")
    
    def stop_service(self):
        """Stop the service"""
        self._run_service_command(['systemctl', '--user', 'stop', 'clockwork-orange.service'], "Stopping service...")
    
    def restart_service(self):
        """Restart the service"""
        self._run_service_command(['systemctl', '--user', 'restart', 'clockwork-orange.service'], "Restarting service...")
    
    def install_service(self):
        """Install the service"""
        try:
            # Check if service file exists
            service_file = Path(__file__).parent.parent / "clockwork-orange.service"
            if not service_file.exists():
                QMessageBox.critical(self, "Error", "Service file not found!")
                return
            
            # Create systemd user directory
            systemd_dir = Path.home() / ".config" / "systemd" / "user"
            systemd_dir.mkdir(parents=True, exist_ok=True)
            
            # Copy service file
            import shutil
            shutil.copy2(service_file, systemd_dir / "clockwork-orange.service")
            
            # Reload systemd and enable service
            subprocess.run(['systemctl', '--user', 'daemon-reload'], check=True)
            subprocess.run(['systemctl', '--user', 'enable', 'clockwork-orange.service'], check=True)
            
            QMessageBox.information(self, "Success", "Service installed and enabled successfully!")
            self.refresh_status()
            
        except Exception as e:
            QMessageBox.critical(self, "Error", f"Failed to install service: {e}")
    
    def uninstall_service(self):
        """Uninstall the service"""
        try:
            # Stop and disable service
            subprocess.run(['systemctl', '--user', 'stop', 'clockwork-orange.service'], check=False)
            subprocess.run(['systemctl', '--user', 'disable', 'clockwork-orange.service'], check=False)
            
            # Remove service file
            service_file = Path.home() / ".config" / "systemd" / "user" / "clockwork-orange.service"
            if service_file.exists():
                service_file.unlink()
            
            # Reload systemd
            subprocess.run(['systemctl', '--user', 'daemon-reload'], check=True)
            
            QMessageBox.information(self, "Success", "Service uninstalled successfully!")
            self.refresh_status()
            
        except Exception as e:
            QMessageBox.critical(self, "Error", f"Failed to uninstall service: {e}")
    
    def _run_service_command(self, command, message):
        """Run a service command with progress indication"""
        self.progress_bar.setVisible(True)
        self.progress_bar.setRange(0, 0)  # Indeterminate progress
        
        try:
            result = subprocess.run(command, capture_output=True, text=True, timeout=30)
            if result.returncode == 0:
                QMessageBox.information(self, "Success", f"{message} completed successfully!")
            else:
                QMessageBox.warning(self, "Warning", f"{message} completed with warnings:\n{result.stderr}")
        except subprocess.TimeoutExpired:
            QMessageBox.critical(self, "Error", f"{message} timed out!")
        except Exception as e:
            QMessageBox.critical(self, "Error", f"Failed to {message.lower()}: {e}")
        finally:
            self.progress_bar.setVisible(False)
            self.refresh_status()
    
    def refresh_logs(self):
        """Refresh service logs"""
        try:
            result = subprocess.run(['journalctl', '--user', '-u', 'clockwork-orange.service', 
                                   '--no-pager', '-n', '50'], capture_output=True, text=True, timeout=10)
            if result.returncode == 0:
                self.logs_text.setText(result.stdout)
            else:
                self.logs_text.setText(f"Error retrieving logs: {result.stderr}")
        except Exception as e:
            self.logs_text.setText(f"Error retrieving logs: {e}")
