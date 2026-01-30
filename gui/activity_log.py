#!/usr/bin/env python3
"""
Activity Log Widget for Windows - shows wallpaper switching activity and logs.
"""
import logging
import queue
from pathlib import Path

from PyQt6.QtCore import Qt, QTimer
from PyQt6.QtGui import QFont
from PyQt6.QtWidgets import (QHBoxLayout, QLabel, QPushButton, QTextEdit,
                             QVBoxLayout, QWidget)


class ActivityLogWidget(QWidget):
    """Widget to display real-time activity logs on Windows."""

    # Maximum number of log lines to keep in memory
    MAX_LOG_LINES = 1000

    def __init__(self, parent=None):
        super().__init__(parent)
        self.log_queue = queue.Queue()
        self.init_ui()
        self.setup_logging()

        # Auto-refresh timer
        self.refresh_timer = QTimer()
        self.refresh_timer.timeout.connect(self.refresh_logs)
        self.refresh_timer.start(500)  # Refresh every 500ms

    def init_ui(self):
        """Initialize the UI components."""
        layout = QVBoxLayout(self)

        # Header
        header_layout = QHBoxLayout()
        header_label = QLabel("Application Activity")
        header_label.setStyleSheet("font-weight: bold; font-size: 14px;")
        header_layout.addWidget(header_label)
        header_layout.addStretch()

        # Control buttons
        clear_btn = QPushButton("Clear")
        clear_btn.clicked.connect(self.clear_log)
        # Refresh button is less useful now that it's incremental, but keep for manual trigger if needed
        # or maybe just remove it? The user UI might expect it. Let's keep it but it just processes queue.
        refresh_btn = QPushButton("Refresh")
        refresh_btn.clicked.connect(self.refresh_logs)

        header_layout.addWidget(refresh_btn)
        header_layout.addWidget(clear_btn)
        layout.addLayout(header_layout)

        # Log display
        self.log_display = QTextEdit()
        self.log_display.setReadOnly(True)
        self.log_display.setLineWrapMode(QTextEdit.LineWrapMode.NoWrap)

        # Native limiting prevents flashing from full rewrites
        self.log_display.document().setMaximumBlockCount(self.MAX_LOG_LINES)

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
        # Create a custom handler that writes to our queue
        handler = LogBufferHandler(self.log_queue)
        handler.setLevel(logging.DEBUG)
        formatter = logging.Formatter(
            "%(asctime)s [%(levelname)s] %(message)s", datefmt="%H:%M:%S"
        )
        handler.setFormatter(formatter)

        # Add to root logger
        logging.getLogger().addHandler(handler)

    def refresh_logs(self):
        """Refresh the log display by appending new items from the queue."""
        has_updates = False
        while not self.log_queue.empty():
            try:
                msg = self.log_queue.get_nowait()
                self.log_display.append(msg)
                has_updates = True
            except queue.Empty:
                break

        if has_updates:
            # Scroll to bottom if we added content
            scrollbar = self.log_display.verticalScrollBar()
            scrollbar.setValue(scrollbar.maximum())

            # Update status
            current_count = self.log_display.document().blockCount()
            self.status_label.setText(
                f"Last updated: {self._get_timestamp()} | {current_count} entries"
            )

    def add_log_message(self, message):
        """Add a log message directly to the queue."""
        self.log_queue.put(message)
        self.refresh_logs()

    def clear_log(self):
        """Clear the log buffer and display."""
        # Clear the queue first
        while not self.log_queue.empty():
            try:
                self.log_queue.get_nowait()
            except queue.Empty:
                break

        self.log_display.clear()
        self.status_label.setText("Log cleared")
        logging.info("Activity log cleared by user")

    def _get_timestamp(self):
        """Get current timestamp string."""
        from datetime import datetime

        return datetime.now().strftime("%H:%M:%S")


class LogBufferHandler(logging.Handler):
    """Custom logging handler that writes to a thread-safe queue."""

    def __init__(self, log_queue):
        super().__init__()
        self.log_queue = log_queue

    def emit(self, record):
        """Emit a log record to the queue."""
        try:
            msg = self.format(record)
            self.log_queue.put(msg)
        except Exception:
            self.handleError(record)
