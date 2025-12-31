#!/usr/bin/env python3
"""
Activity Log Widget for Windows - shows wallpaper switching activity and logs.
"""
from PyQt6.QtWidgets import (
    QWidget, QVBoxLayout, QTextEdit, QPushButton, QHBoxLayout, QLabel
)
from PyQt6.QtCore import QTimer, Qt
from PyQt6.QtGui import QFont
import logging
from pathlib import Path


class ActivityLogWidget(QWidget):
    """Widget to display application activity logs on Windows."""
    
    def __init__(self, parent=None):
        super().__init__(parent)
        self.log_buffer = []
        self.max_buffer_size = 1000
        self.init_ui()
        self.setup_logging()
        
        # Auto-refresh timer
        self.refresh_timer = QTimer()
        self.refresh_timer.timeout.connect(self.refresh_logs)
        self.refresh_timer.start(2000)  # Refresh every 2 seconds
        
    def init_ui(self):
        """Initialize the UI components."""
        layout = QVBoxLayout(self)
        
        # Header
        header_layout = QHBoxLayout()
        header_label = QLabel("<b>Application Activity Log</b>")
        header_layout.addWidget(header_label)
        header_layout.addStretch()
        
        # Control buttons
        clear_btn = QPushButton("Clear Log")
        clear_btn.clicked.connect(self.clear_log)
        refresh_btn = QPushButton("Refresh")
        refresh_btn.clicked.connect(self.refresh_logs)
        
        header_layout.addWidget(refresh_btn)
        header_layout.addWidget(clear_btn)
        layout.addLayout(header_layout)
        
        # Log display
        self.log_display = QTextEdit()
        self.log_display.setReadOnly(True)
        self.log_display.setLineWrapMode(QTextEdit.LineWrapMode.NoWrap)
        
        # Use monospace font for logs
        font = QFont("Consolas", 9)
        if not font.exactMatch():
            font = QFont("Courier New", 9)
        self.log_display.setFont(font)
        
        layout.addWidget(self.log_display)
        
        # Status bar
        self.status_label = QLabel("Activity log ready")
        self.status_label.setStyleSheet("color: gray; font-size: 10px;")
        layout.addWidget(self.status_label)
        
    def setup_logging(self):
        """Set up logging handler to capture application logs."""
        # Create a custom handler that writes to our buffer
        handler = LogBufferHandler(self.log_buffer, self.max_buffer_size)
        handler.setLevel(logging.DEBUG)
        formatter = logging.Formatter('%(asctime)s [%(levelname)s] %(message)s', 
                                     datefmt='%H:%M:%S')
        handler.setFormatter(formatter)
        
        # Add to root logger
        logging.getLogger().addHandler(handler)
        
    def refresh_logs(self):
        """Refresh the log display with current buffer contents."""
        if self.log_buffer:
            self.log_display.setPlainText('\n'.join(self.log_buffer))
            # Scroll to bottom
            scrollbar = self.log_display.verticalScrollBar()
            scrollbar.setValue(scrollbar.maximum())
            
            self.status_label.setText(f"Last updated: {self._get_timestamp()} | {len(self.log_buffer)} entries")
        else:
            self.log_display.setPlainText("No activity yet. Waiting for wallpaper changes...")
            self.status_label.setText("No activity")
            
    def clear_log(self):
        """Clear the log buffer and display."""
        self.log_buffer.clear()
        self.log_display.clear()
        self.status_label.setText("Log cleared")
        logging.info("Activity log cleared by user")
        
    def _get_timestamp(self):
        """Get current timestamp string."""
        from datetime import datetime
        return datetime.now().strftime("%H:%M:%S")


class LogBufferHandler(logging.Handler):
    """Custom logging handler that writes to a buffer."""
    
    def __init__(self, buffer, max_size):
        super().__init__()
        self.buffer = buffer
        self.max_size = max_size
        
    def emit(self, record):
        """Emit a log record to the buffer."""
        try:
            msg = self.format(record)
            self.buffer.append(msg)
            
            # Trim buffer if too large
            if len(self.buffer) > self.max_size:
                self.buffer.pop(0)
        except Exception:
            self.handleError(record)
