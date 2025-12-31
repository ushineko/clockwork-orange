#!/usr/bin/env python3
"""
Blacklist management tab for clockwork-orange.
"""
import sys
from pathlib import Path

from PyQt6.QtCore import QSize
from PyQt6.QtGui import QIcon, QPixmap
from PyQt6.QtWidgets import (QAbstractItemView, QHBoxLayout, QHeaderView,
                             QLabel, QLineEdit, QMessageBox, QPushButton,
                             QTableWidget, QTableWidgetItem, QVBoxLayout,
                             QWidget)

# Adjust path to import plugins
sys.path.append(str(Path(__file__).parent.parent))
from plugins.blacklist import BlacklistManager


class BlacklistTab(QWidget):
    """Tab for managing blacklisted images."""

    def __init__(self):
        super().__init__()
        self.blacklist_manager = BlacklistManager()
        self.init_ui()
        self.load_blacklist()

    def init_ui(self):
        layout = QVBoxLayout()

        # Header / Filters
        filter_layout = QHBoxLayout()
        self.search_input = QLineEdit()
        self.search_input.setPlaceholderText("Search hash, plugin...")
        self.search_input.textChanged.connect(self.filter_table)
        filter_layout.addWidget(QLabel("Filter:"))
        filter_layout.addWidget(self.search_input)

        self.refresh_btn = QPushButton("Refresh")
        self.refresh_btn.clicked.connect(self.load_blacklist)
        filter_layout.addWidget(self.refresh_btn)

        layout.addLayout(filter_layout)

        # Table
        self.table = QTableWidget()
        self.table.setColumnCount(4)
        self.table.setHorizontalHeaderLabels(
            ["Unblock Image", "Date Added", "Source Plugin", "Full Hash"]
        )
        self.table.horizontalHeader().setSectionResizeMode(
            0, QHeaderView.ResizeMode.ResizeToContents
        )  # Thumbnail
        self.table.setIconSize(
            self.table.iconSize() * 4
        )  # Make icons bigger? No, set in load_blacklist
        self.table.verticalHeader().setDefaultSectionSize(
            80
        )  # Default row height for thumbnails
        self.table.horizontalHeader().setSectionResizeMode(
            1, QHeaderView.ResizeMode.ResizeToContents
        )  # Date
        self.table.horizontalHeader().setSectionResizeMode(
            2, QHeaderView.ResizeMode.ResizeToContents
        )  # Plugin
        self.table.horizontalHeader().setSectionResizeMode(
            3, QHeaderView.ResizeMode.Stretch
        )  # Full Hash

        self.table.setSelectionBehavior(QAbstractItemView.SelectionBehavior.SelectRows)
        self.table.setSelectionMode(QAbstractItemView.SelectionMode.ExtendedSelection)
        self.table.setEditTriggers(QAbstractItemView.EditTrigger.NoEditTriggers)

        layout.addWidget(self.table)

        # Actions
        action_layout = QHBoxLayout()
        self.delete_btn = QPushButton("Remove Selected from Blacklist")
        self.delete_btn.setStyleSheet("background-color: #552222;")
        self.delete_btn.clicked.connect(self.remove_selected)
        action_layout.addWidget(self.delete_btn)

        layout.addLayout(action_layout)

        self.setLayout(layout)

    def load_blacklist(self):
        """Load data from BlacklistManager."""
        entries = self.blacklist_manager.get_blacklist_items()

        self.table.setRowCount(0)
        self.table.setRowCount(len(entries))
        self.table.setIconSize(QSize(64, 64))

        for row, item in enumerate(entries):
            h = item["hash"]
            date_str = item["date"]
            plugin_str = item["source"]
            thumb_blob = item["thumbnail"]

            item_thumb = QTableWidgetItem(h[:8])
            if thumb_blob:
                pixmap = QPixmap()
                pixmap.loadFromData(thumb_blob)
                if not pixmap.isNull():
                    item_thumb.setIcon(QIcon(pixmap))

            item_thumb.setToolTip(h)

            item_date = QTableWidgetItem(date_str)
            item_plugin = QTableWidgetItem(plugin_str)
            item_full_hash = QTableWidgetItem(h)

            self.table.setItem(row, 0, item_thumb)
            self.table.setItem(row, 1, item_date)
            self.table.setItem(row, 2, item_plugin)
            self.table.setItem(row, 3, item_full_hash)

            self.table.setRowHeight(row, 70)

        self.filter_table(self.search_input.text())

    def filter_table(self, text):
        """Filter table rows."""
        text = text.lower()
        for i in range(self.table.rowCount()):
            match = False
            for j in range(self.table.columnCount()):
                item = self.table.item(i, j)
                if item and text in item.text().lower():
                    match = True
                    break
            self.table.setRowHidden(i, not match)

    def remove_selected(self):
        """Remove selected items."""
        selected_rows = set()
        for item in self.table.selectedItems():
            selected_rows.add(item.row())

        if not selected_rows:
            return

        reply = QMessageBox.question(
            self,
            "Confirm Removal",
            f"Are you sure you want to remove {len(selected_rows)} items from the blacklist?\n"
            "These images will be allowed to be downloaded again.",
            QMessageBox.StandardButton.Yes | QMessageBox.StandardButton.No,
        )

        if reply == QMessageBox.StandardButton.Yes:
            hashes_to_remove = []
            for row in selected_rows:
                # Get full hash from column 3
                h = self.table.item(row, 3).text()
                hashes_to_remove.append(h)

            for h in hashes_to_remove:
                self.blacklist_manager.remove_from_blacklist(h)

            self.load_blacklist()
