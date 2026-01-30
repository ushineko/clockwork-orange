#!/usr/bin/env python3
"""
History management tab for the GUI.
"""
import sys
from pathlib import Path

from PyQt6.QtCore import Qt, QTimer
from PyQt6.QtWidgets import (QFormLayout, QGroupBox, QLabel, QMessageBox,
                             QPushButton, QVBoxLayout, QWidget)

# Ensure we can import the plugin manager and history manager
sys.path.append(str(Path(__file__).parent.parent))
from plugins.history import HistoryManager


class HistoryTab(QWidget):
    def __init__(self, config_data):
        super().__init__()
        self.config_data = config_data
        self.history_manager = HistoryManager()
        self.init_ui()

        # Auto-refresh stats when tab is shown
        self.refresh_timer = QTimer(self)
        self.refresh_timer.timeout.connect(self.refresh_stats)
        self.refresh_timer.start(5000)  # Refresh every 5s

    def showEvent(self, event):
        self.refresh_stats()
        super().showEvent(event)

    def init_ui(self):
        layout = QVBoxLayout()
        layout.setContentsMargins(20, 20, 20, 20)

        # Title
        title = QLabel("History Manager")
        title.setStyleSheet("font-size: 18px; font-weight: bold; margin-bottom: 10px;")
        layout.addWidget(title)

        desc = QLabel(
            "The History Manager tracks all downloaded images to prevent duplicates, "
            "even if you delete the files."
        )
        desc.setWordWrap(True)
        desc.setStyleSheet("color: #aaa; margin-bottom: 20px;")
        layout.addWidget(desc)

        # Stats Group
        stats_group = QGroupBox("Database Statistics")
        stats_layout = QFormLayout()

        self.lbl_records = QLabel("0")
        self.lbl_unique = QLabel("0")
        self.lbl_size = QLabel("0 B")

        stats_layout.addRow("Total Downloads Tracked:", self.lbl_records)
        stats_layout.addRow("Unique Images:", self.lbl_unique)
        stats_layout.addRow("Database Size:", self.lbl_size)

        stats_group.setLayout(stats_layout)
        layout.addWidget(stats_group)

        layout.addStretch()

        # Actions Group
        action_group = QGroupBox("Actions")
        action_layout = QVBoxLayout()

        self.reset_btn = QPushButton("Reset History Database")
        self.reset_btn.setStyleSheet("background-color: #552222; padding: 8px;")
        self.reset_btn.clicked.connect(self.reset_history)
        action_layout.addWidget(self.reset_btn)

        self.import_btn = QPushButton("Scan && Import Existing Files")
        self.import_btn.clicked.connect(self.import_existing_files)
        action_layout.addWidget(self.import_btn)

        reset_desc = QLabel(
            "Warning: Resetting history will allow previously deleted images to be downloaded again."
        )
        reset_desc.setWordWrap(True)
        reset_desc.setStyleSheet("color: #888; font-style: italic;")
        action_layout.addWidget(reset_desc)

        action_group.setLayout(action_layout)
        layout.addWidget(action_group)

        self.setLayout(layout)

    def refresh_stats(self):
        stats = self.history_manager.get_stats()

        self.lbl_records.setText(f"{stats['total_records']:,}")
        self.lbl_unique.setText(f"{stats['unique_images']:,}")

        # Format size
        size = stats["db_size_bytes"]
        if size < 1024:
            size_str = f"{size} B"
        elif size < 1024 * 1024:
            size_str = f"{size/1024:.1f} KB"
        else:
            size_str = f"{size/(1024*1024):.1f} MB"

        self.lbl_size.setText(size_str)

    def reset_history(self):
        reply = QMessageBox.warning(
            self,
            "Confirm Reset",
            "Are you sure you want to clear the download history?\n\n"
            "This action cannot be undone.",
            QMessageBox.StandardButton.Yes | QMessageBox.StandardButton.No,
            QMessageBox.StandardButton.No,
        )

        if reply == QMessageBox.StandardButton.Yes:
            try:
                self.history_manager.clear_history()
                self.refresh_stats()
                QMessageBox.information(
                    self, "Success", "History database has been cleared."
                )
            except Exception as e:
                QMessageBox.critical(self, "Error", f"Failed to reset history:\n{e}")

    def import_existing_files(self):
        """Scan existing download directory and populate history."""
        # Determine download directory from config or default
        plugin_config = self.config_data.get("google_images", {})
        download_dir_str = plugin_config.get("download_dir")

        if not download_dir_str:
            # Fallback to default
            download_dir = Path.home() / "Pictures" / "Wallpapers" / "GoogleImages"
        else:
            download_dir = Path(download_dir_str)

        if not download_dir.exists():
            QMessageBox.warning(
                self, "Migration", f"Directory not found: {download_dir}"
            )
            return

        # Scan
        files = list(
            download_dir.glob("*.[jJ][pP]*[gG]")
        )  # jpg, peg, etc simple regex-ish glob
        if not files:
            QMessageBox.information(
                self, "Migration", f"No images found in {download_dir}"
            )
            return

        from PyQt6.QtWidgets import QProgressDialog

        progress = QProgressDialog(
            "Scanning existing images...", "Cancel", 0, len(files), self
        )
        progress.setWindowModality(Qt.WindowModality.WindowModal)
        progress.setMinimumDuration(0)

        count = 0
        skipped = 0

        for i, file_path in enumerate(files):
            if progress.wasCanceled():
                break

            # Add to history
            # We don't have the original URL, so we use a placeholder.
            # The important part is the IMAGE HASH, which add_entry calculates.
            # This ensures if we download this same image again from any URL, it gets blocked.
            url = f"file://imported/{file_path.name}"

            # Note: add_entry returns True if added, False if duplicate URL hash
            # If image content is duplicate, add_entry doesn't check natively (it's up to caller usually),
            # but wait, add_entry INSERTs.
            # Our HistoryManager schema has UNIQUE(url_hash).
            # It does NOT have UNIQUE(image_hash).
            # So we CAN insert multiple entries with same image hash but different URLs.
            # This is fine. We want to 'poison' the DB with this content hash so `seen_image` returns true.

            if self.history_manager.add_entry(url, file_path, source="imported"):
                count += 1
            else:
                skipped += 1

            progress.setValue(i + 1)

        self.refresh_stats()
        QMessageBox.information(
            self,
            "Migration Complete",
            f"Imported {count} images.\nSkipped {skipped} duplicates.",
        )
