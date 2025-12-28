#!/usr/bin/env python3
"""
Plugin management widget (SinglePluginWidget) for the GUI.
"""
import sys
from datetime import datetime
from pathlib import Path

from PyQt6.QtCore import QFileSystemWatcher, Qt, QThread, QUrl, pyqtSignal
from PyQt6.QtGui import QColor, QDesktopServices, QFont, QPainter, QPen, QPixmap
from PyQt6.QtWidgets import (
    QCheckBox,
    QComboBox,
    QDialog,
    QDialogButtonBox,
    QFileDialog,
    QFormLayout,
    QGroupBox,
    QHBoxLayout,
    QLabel,
    QLineEdit,
    QListWidget,
    QListWidgetItem,
    QMessageBox,
    QProgressBar,
    QPushButton,
    QScrollArea,
    QSizePolicy,
    QSpinBox,
    QSplitter,
    QTextEdit,
    QVBoxLayout,
    QWidget,
)

# Ensure we can import the plugin manager
sys.path.append(str(Path(__file__).parent.parent))
from plugin_manager import PluginManager


def get_relative_time(dt):
    """Return a friendly relative time string."""
    now = datetime.now()
    diff = now - dt

    seconds = diff.total_seconds()
    if seconds < 60:
        return "Just now"
    elif seconds < 3600:
        minutes = int(seconds / 60)
        return f"{minutes} min{'s' if minutes != 1 else ''} ago"
    elif seconds < 86400:
        hours = int(seconds / 3600)
        return f"{hours} hour{'s' if hours != 1 else ''} ago"
    elif seconds < 604800:
        days = int(seconds / 86400)
        return f"{days} day{'s' if days != 1 else ''} ago"
    else:
        return dt.strftime("%Y-%m-%d")


class AutoResizingLabel(QLabel):
    """A QLabel that automatically scales its pixmap content to fit resizing."""

    def __init__(self, text="", parent=None):
        super().__init__(text, parent)
        self._pixmap = None
        self.setAlignment(Qt.AlignmentFlag.AlignCenter)
        # Change policy to allow shrinking
        self.setSizePolicy(QSizePolicy.Policy.Ignored, QSizePolicy.Policy.Ignored)
        self.setMinimumHeight(50)  # Reduce minimum height to allow resizing

    def setPixmap(self, pixmap):
        self._pixmap = pixmap
        self._update_scaled_pixmap()

    def setText(self, text):
        self._pixmap = None
        super().setText(text)

    def resizeEvent(self, event):
        self._update_scaled_pixmap()
        super().resizeEvent(event)

    def _update_scaled_pixmap(self):
        if self._pixmap and not self._pixmap.isNull():
            # Scale based on current size
            scaled = self._pixmap.scaled(
                self.size(),
                Qt.AspectRatioMode.KeepAspectRatio,
                Qt.TransformationMode.SmoothTransformation,
            )
            super().setPixmap(scaled)


# ... (SearchTermsWidget and TermSelectionDialog remain unchanged) ...


class PluginExecutionDialog(QDialog):
    """Dialog to run a plugin and show progress/logs."""

    def __init__(
        self,
        manager,
        plugin_name,
        config,
        title="Running Plugin",
        search_terms_config=None,
        parent=None,
    ):
        super().__init__(parent)
        self.manager = manager
        self.plugin_name = plugin_name
        self.config = config
        self.search_terms_config = search_terms_config
        self.runner = None

        self.setWindowTitle(title)
        self.resize(600, 450)  # Slightly larger
        self.setModal(True)

        self.layout = QVBoxLayout(self)

        # 1. Search Terms Selection (if applicable)
        self.list_widget = None
        if self.search_terms_config:
            self.layout.addWidget(QLabel("Select terms to include:"))
            self.list_widget = QListWidget()
            terms_data = self.search_terms_config.get("data", [])
            for item_data in terms_data:
                if isinstance(item_data, dict):
                    term = item_data.get("term", "")
                    enabled = item_data.get("enabled", True)
                    if term:
                        item = QListWidgetItem(term)
                        item.setFlags(item.flags() | Qt.ItemFlag.ItemIsUserCheckable)
                        item.setCheckState(
                            Qt.CheckState.Checked
                            if enabled
                            else Qt.CheckState.Unchecked
                        )
                        self.list_widget.addItem(item)
            self.layout.addWidget(self.list_widget)

        # 2. Progress
        self.progress_bar = QProgressBar()
        self.progress_bar.setRange(0, 100)
        self.progress_bar.setValue(0)
        self.layout.addWidget(self.progress_bar)

        # 3. Content Area (Splitter: Logs | Preview)
        content_splitter = QSplitter(Qt.Orientation.Horizontal)
        self.layout.addWidget(content_splitter)

        # Left: Logs
        self.log_viewer = QTextEdit()
        self.log_viewer.setReadOnly(True)
        self.log_viewer.setPlaceholderText("Waiting to start...")

        # Apply font
        font_family = config.get("console_font_family", "Monospace")
        font_size = config.get("console_font_size", 10)
        self.log_viewer.setFont(QFont(font_family, font_size))

        content_splitter.addWidget(self.log_viewer)

        # Right: Image Preview
        self.preview_label = AutoResizingLabel("Checking for images...")
        self.preview_label.setStyleSheet(
            "border: 1px solid #444; background-color: #222;"
        )
        self.preview_label.setMinimumWidth(200)
        self.preview_label.setSizePolicy(
            QSizePolicy.Policy.Ignored, QSizePolicy.Policy.Ignored
        )
        content_splitter.addWidget(self.preview_label)

        # Set splitter proportions (Logs getting more space initially)
        content_splitter.setSizes([400, 200])
        content_splitter.setCollapsible(1, True)

        # 4. Buttons
        btn_layout = QHBoxLayout()
        self.start_btn = QPushButton(
            "Start Download" if search_terms_config else "Start"
        )
        self.start_btn.clicked.connect(self.start_run)
        btn_layout.addWidget(self.start_btn)

        self.close_btn = QPushButton("Close")
        self.close_btn.clicked.connect(self.close)
        self.close_btn.setEnabled(True)
        btn_layout.addWidget(self.close_btn)

        self.layout.addLayout(btn_layout)

        # Auto-start checks
        if not self.search_terms_config:
            self.start_btn.hide()
            from PyQt6.QtCore import QTimer

            QTimer.singleShot(100, self.start_run)
        else:
            self.log_viewer.setPlaceholderText(
                "Select terms and click Start Download..."
            )

    def start_run(self):
        self.start_btn.setEnabled(False)
        self.close_btn.setEnabled(False)
        self.log_viewer.clear()
        self.log_viewer.append(f"Starting {self.plugin_name}...")
        self.preview_label.setText("Waiting for download...")
        self.preview_label.setPixmap(QPixmap())  # Clear previous image

        run_config = self.config.copy()
        if self.search_terms_config and self.list_widget:
            selected_terms = []
            for i in range(self.list_widget.count()):
                item = self.list_widget.item(i)
                if item.checkState() == Qt.CheckState.Checked:
                    selected_terms.append(item.text())

            key = self.search_terms_config["key"]
            run_config[key] = selected_terms

        self.runner = PluginRunner(self.manager, self.plugin_name, run_config)
        self.runner.log_signal.connect(self.log_viewer.append)
        self.runner.progress_signal.connect(self.update_progress)
        self.runner.image_saved_signal.connect(self.update_preview)
        self.runner.finished_signal.connect(self.on_finished)
        self.runner.start()

    def update_progress(self, percent, message):
        self.progress_bar.setValue(percent)
        self.progress_bar.setFormat(f"{percent}% - {message}" if message else "%p%")

    def update_preview(self, image_path):
        """Update the preview label with the newly downloaded image."""
        pixmap = QPixmap(image_path)
        if not pixmap.isNull():
            self.preview_label.setPixmap(pixmap)
        else:
            self.preview_label.setText(f"Failed to load: {Path(image_path).name}")

    def on_finished(self, result):
        self.log_viewer.append("\nDone.")
        self.close_btn.setEnabled(True)
        self.start_btn.setEnabled(True)

        if result.get("status") == "error":
            self.log_viewer.append(f"Error: {result.get('message')}")


class SearchTermsWidget(QWidget):
    """Widget for managing a list of search terms with enable/disable toggles."""

    changed = pyqtSignal()

    def __init__(self):
        super().__init__()
        self.init_ui()

    def init_ui(self):
        layout = QVBoxLayout()
        layout.setContentsMargins(0, 0, 0, 0)

        # List
        self.list_widget = QListWidget()
        self.list_widget.setMaximumHeight(120)
        layout.addWidget(self.list_widget)

        # Input area
        input_layout = QHBoxLayout()
        self.input_field = QComboBox()
        self.input_field.setEditable(True)
        self.input_field.setPlaceholderText("Enter search term...")
        self.input_field.setSizePolicy(
            QSizePolicy.Policy.Expanding, QSizePolicy.Policy.Fixed
        )

        self.add_btn = QPushButton("Add")
        self.add_btn.clicked.connect(self.add_term)
        self.input_field.lineEdit().returnPressed.connect(self.add_term)

        input_layout.addWidget(self.input_field)
        input_layout.addWidget(self.add_btn)
        layout.addLayout(input_layout)

        # Remove button
        self.remove_btn = QPushButton("Remove Selected")
        self.remove_btn.clicked.connect(self.remove_term)
        layout.addWidget(self.remove_btn)

        self.setLayout(layout)

        # Signals
        self.list_widget.itemChanged.connect(lambda: self.changed.emit())

    def add_term(self):
        text = self.input_field.currentText().strip()
        if text:
            item = QListWidgetItem(text)
            item.setFlags(
                item.flags()
                | Qt.ItemFlag.ItemIsUserCheckable
                | Qt.ItemFlag.ItemIsEnabled
                | Qt.ItemFlag.ItemIsSelectable
            )
            item.setCheckState(Qt.CheckState.Checked)
            self.list_widget.addItem(item)
            self.input_field.setEditText("")
            self.input_field.setCurrentIndex(-1)
            self.changed.emit()

    def remove_term(self):
        for item in self.list_widget.selectedItems():
            row = self.list_widget.row(item)
            self.list_widget.takeItem(row)
        self.changed.emit()

    def get_value(self):
        """Return list of dicts: [{'term': 'foo', 'enabled': True}, ...]"""
        result = []
        for i in range(self.list_widget.count()):
            item = self.list_widget.item(i)
            result.append(
                {
                    "term": item.text(),
                    "enabled": item.checkState() == Qt.CheckState.Checked,
                }
            )
        return result

    def set_value(self, data):
        """Set value from config (list of dicts OR string)."""
        self.list_widget.clear()

        if isinstance(data, str):
            # Legacy string parsing
            terms = [t.strip() for t in data.split(",")]
            for t in terms:
                if t:
                    self.add_item(t, True)
        elif isinstance(data, list):
            for item_data in data:
                if isinstance(item_data, dict):
                    term = item_data.get("term")
                    enabled = item_data.get("enabled", True)
                    if term:
                        self.add_item(term, enabled)
                elif isinstance(item_data, str):
                    self.add_item(item_data, True)

    def add_item(self, text, enabled):
        item = QListWidgetItem(text)
        item.setFlags(
            item.flags()
            | Qt.ItemFlag.ItemIsUserCheckable
            | Qt.ItemFlag.ItemIsEnabled
            | Qt.ItemFlag.ItemIsSelectable
        )
        item.setCheckState(
            Qt.CheckState.Checked if enabled else Qt.CheckState.Unchecked
        )
        self.list_widget.addItem(item)

    def set_suggestions(self, suggestions):
        self.input_field.clear()
        if suggestions:
            self.input_field.addItems(suggestions)
            self.input_field.setCurrentIndex(-1)
            self.input_field.setEditText("")


class TermSelectionDialog(QDialog):
    """Dialog to select which search terms to run."""

    def __init__(self, terms_data, parent=None):
        super().__init__(parent)
        self.setWindowTitle("Select Search Terms")
        self.setFixedWidth(400)
        self.setModal(True)
        self.selected_terms = []

        layout = QVBoxLayout(self)

        layout.addWidget(QLabel("Select terms to include in this run:"))

        self.list_widget = QListWidget()

        # Populate list
        # terms_data is list of dicts: [{'term': 'foo', 'enabled': True}, ...]
        for item_data in terms_data:
            if isinstance(item_data, dict):
                term = item_data.get("term", "")
                enabled = item_data.get("enabled", True)
                if term:
                    item = QListWidgetItem(term)
                    item.setFlags(item.flags() | Qt.ItemFlag.ItemIsUserCheckable)
                    item.setCheckState(
                        Qt.CheckState.Checked if enabled else Qt.CheckState.Unchecked
                    )
                    self.list_widget.addItem(item)

        layout.addWidget(self.list_widget)

        # Buttons
        buttons = QDialogButtonBox(
            QDialogButtonBox.StandardButton.Ok | QDialogButtonBox.StandardButton.Cancel
        )
        buttons.accepted.connect(self.accept)
        buttons.rejected.connect(self.reject)
        layout.addWidget(buttons)

    def accept(self):
        # Gather checked items
        self.selected_terms = []
        for i in range(self.list_widget.count()):
            item = self.list_widget.item(i)
            if item.checkState() == Qt.CheckState.Checked:
                self.selected_terms.append(item.text())
        super().accept()

    def get_selected_terms(self):
        return self.selected_terms


class PluginRunner(QThread):
    progress_signal = pyqtSignal(int, str)
    log_signal = pyqtSignal(str)
    image_saved_signal = pyqtSignal(str)
    finished_signal = pyqtSignal(dict)

    def __init__(self, manager, name, config):
        super().__init__()
        self.manager = manager
        self.name = name
        self.config = config

    def run(self):
        try:
            # Use streaming execution
            iterator = self.manager.execute_plugin_stream(self.name, self.config)

            final_result = None

            for item in iterator:
                if isinstance(item, dict):
                    # This is the final result
                    final_result = item
                else:
                    # This is a log line
                    self._handle_log_line(str(item))

            if final_result:
                self.finished_signal.emit(final_result)
            else:
                self.finished_signal.emit(
                    {"status": "error", "message": "No result returned"}
                )
        except Exception as e:
            self.log_signal.emit(f"Error in thread: {str(e)}")
            self.finished_signal.emit(
                {"status": "error", "message": str(e), "logs": str(e)}
            )

    def _handle_log_line(self, line):
        """Process a log line, checking for special markers."""
        self.log_signal.emit(line)

        # Check for progress marker
        # Format: ::PROGRESS:: <percent> :: <message>
        if "::PROGRESS::" in line:
            self._parse_progress(line)

        # Check for image saved marker
        # Format: ::IMAGE_SAVED:: <path>
        if "::IMAGE_SAVED::" in line:
            self._parse_image_saved(line)

    def _parse_progress(self, line):
        try:
            parts = line.split("::")
            if len(parts) >= 4:
                percent = int(parts[2].strip())
                message = parts[3].strip()
                self.progress_signal.emit(percent, message)
        except Exception:
            pass

    def _parse_image_saved(self, line):
        try:
            parts = line.split("::IMAGE_SAVED::")
            if len(parts) >= 2:
                image_path = parts[1].strip()
                self.image_saved_signal.emit(image_path)
        except Exception:
            pass


class SinglePluginWidget(QWidget):
    """Widget for configuring a single specific plugin."""

    config_changed = pyqtSignal()

    def __init__(
        self, plugin_name: str, config_data: dict, plugin_manager: PluginManager
    ):
        super().__init__()
        self.plugin_name = plugin_name
        self.config_data = config_data
        self.plugin_manager = plugin_manager

        # Internal state
        self.current_plugin_widgets = {}
        self.review_images = []
        self.review_index = 0
        self.blacklisted_indices = set()

        self.watcher = QFileSystemWatcher()
        self.watcher.directoryChanged.connect(self.on_directory_changed)

        self.init_ui()
        self.load_plugin_ui()

    def init_ui(self):
        """Initialize the single plugin UI."""

        # Enable keyboard focus for review navigation
        self.setFocusPolicy(Qt.FocusPolicy.StrongFocus)

        self.layout = QVBoxLayout()
        self.layout.setContentsMargins(10, 10, 10, 10)

        # 1. Header with description
        self.description_label = QLabel()
        self.description_label.setWordWrap(True)
        self.description_label.setStyleSheet("color: #666; font-style: italic;")
        self.description_label.setSizePolicy(
            QSizePolicy.Policy.Expanding, QSizePolicy.Policy.Fixed
        )
        self.description_label.setMaximumHeight(60)  # Constraint height
        self.layout.addWidget(self.description_label)

        # 2. Main content area (Splitter - Vertical)
        # User requested three vertically stacked sections: Description, Config, Preview
        # We achieve this by putting Config and Preview in a vertical splitter below the description.
        content_splitter = QSplitter(Qt.Orientation.Vertical)

        # Top Section of Splitter: Configuration
        self.config_group = QGroupBox("Configuration")
        self.config_layout = QFormLayout()
        self.config_group.setLayout(self.config_layout)

        scroll = QScrollArea()
        scroll.setWidget(self.config_group)
        scroll.setWidgetResizable(True)
        content_splitter.addWidget(scroll)

        # Bottom Section of Splitter: Preview & Actions
        # Structure:
        # [ VBox ]
        #   [ Progress Bar ]
        #   [ HSplitter ]
        #     [ Preview ] | [ Actions ]

        bottom_column = QWidget()
        bottom_layout = QVBoxLayout(bottom_column)
        bottom_layout.setContentsMargins(0, 0, 0, 0)

        # 2. Splitter (Preview | Review)
        bottom_splitter = QSplitter(Qt.Orientation.Horizontal)
        bottom_layout.addWidget(bottom_splitter)

        # Left: Preview
        self.preview_label = AutoResizingLabel("No preview available")
        self.preview_label.setStyleSheet(
            "border: 1px solid #ccc; background-color: #222;"
        )
        bottom_splitter.addWidget(self.preview_label)

        # Right: Actions
        action_group = QGroupBox("Review")
        action_group.setMinimumWidth(250)

        action_layout = QVBoxLayout()

        self.log_viewer = QTextEdit()
        self.log_viewer.setReadOnly(True)
        # Constrain height effectively for 3 lines (approx 80px)
        self.log_viewer.setMaximumHeight(80)
        self.log_viewer.setPlaceholderText("Review status...")
        action_layout.addWidget(self.log_viewer)

        self.navigate_legend = QLabel("←/→: Navigate | Space: Mark/Unmark")
        self.navigate_legend.setAlignment(Qt.AlignmentFlag.AlignCenter)
        self.navigate_legend.setStyleSheet(
            "color: cadetblue; font-style: italic; font-weight: bold;"
        )
        self.navigate_legend.setVisible(False)
        action_layout.addWidget(self.navigate_legend)

        # Spacer to push buttons down
        action_layout.addStretch()

        btn_layout = QVBoxLayout()

        self.run_btn = QPushButton("Download Now")
        self.run_btn.setToolTip("Run plugin manually with optional query selection")
        self.run_btn.clicked.connect(self.run_current_plugin)
        btn_layout.addWidget(self.run_btn)

        self.reset_btn = QPushButton("Reset & Run")
        self.reset_btn.clicked.connect(self.reset_current_plugin)
        btn_layout.addWidget(self.reset_btn)

        # Review buttons (Review Images removed as it's auto)

        self.apply_blacklist_btn = QPushButton("Apply Blacklist (0)")
        self.apply_blacklist_btn.clicked.connect(self.apply_blacklist)
        self.apply_blacklist_btn.setEnabled(False)
        self.apply_blacklist_btn.setStyleSheet("background-color: #552222;")
        btn_layout.addWidget(self.apply_blacklist_btn)

        action_layout.addLayout(btn_layout)
        action_group.setLayout(action_layout)

        bottom_splitter.addWidget(action_group)

        # Set initial splitter sizes (give priority to image)
        bottom_splitter.setSizes([550, 250])
        bottom_splitter.setCollapsible(1, False)  # Don't collapse actions completely

        content_splitter.addWidget(bottom_column)

        # Set initial sizes (give more space to config/preview)
        content_splitter.setSizes([400, 400])
        content_splitter.setCollapsible(0, False)
        content_splitter.setCollapsible(1, False)

        self.layout.addWidget(content_splitter)
        self.setLayout(self.layout)

    def load_plugin_ui(self):
        """Load the specific plugin's UI."""
        # Clear existing config widgets
        while self.config_layout.count():
            item = self.config_layout.takeAt(0)
            if item.widget():
                item.widget().deleteLater()

        self.current_plugin_widgets = {}

        # Get schema and description
        try:
            schema = self.plugin_manager.get_plugin_schema(self.plugin_name)
            description = self.plugin_manager.get_plugin_description(self.plugin_name)
            self.description_label.setText(description)

            self.load_plugin_config_ui(self.plugin_name, schema)
        except Exception as e:
            self.config_layout.addRow(QLabel(f"Error loading plugin: {e}"))

    def set_config(self, config_data):
        """Update global config data."""
        self.config_data = config_data

        # Update fonts
        font_family = self.config_data.get("console_font_family", "Monospace")
        font_size = self.config_data.get("console_font_size", 10)
        self.log_viewer.setFont(QFont(font_family, font_size))

    def on_directory_changed(self, path):
        """Handle directory changes for auto-refresh."""
        if not self.isVisible():
            return

        # Refresh review if we are in review mode (or effectively so)
        # We can just call scan_for_review as it updates the internal list
        print(f"[DEBUG] Directory changed: {path} - Refreshing review...")
        self.scan_for_review()

    def load_plugin_config_ui(self, plugin_name, schema):
        plugin_config = self.config_data.get("plugins", {}).get(plugin_name, {})

        # Enabled Checkbox
        self._add_enabled_checkbox(plugin_config.get("enabled", False))

        # Config Fields
        if schema:
            keys = list(schema.keys())
            i = 0
            while i < len(keys):
                key = keys[i]
                field_info = schema[key]
                group_name = field_info.get("group")

                if group_name:
                    # Collect all consecutive fields in this group
                    group_keys = [key]
                    i += 1
                    while i < len(keys):
                        next_key = keys[i]
                        next_info = schema[next_key]
                        if next_info.get("group") == group_name:
                            group_keys.append(next_key)
                            i += 1
                        else:
                            break

                    self._add_grouped_config_fields(
                        group_name, group_keys, schema, plugin_config
                    )
                else:
                    self._add_config_field(key, field_info, plugin_config)
                    i += 1
        else:
            self.config_layout.addRow(QLabel("No configuration options."))

        # Configure buttons
        self._configure_action_buttons(plugin_name)

    def _add_grouped_config_fields(self, group_name, keys, schema, current_config):
        # Create a horizontal layout for the group
        container = QWidget()
        layout = QHBoxLayout(container)
        layout.setContentsMargins(0, 0, 0, 0)

        # Add label for the group
        group_label = QLabel(f"{group_name}:")
        group_label.setStyleSheet("font-weight: bold;")
        layout.addWidget(group_label)

        for key in keys:
            props = schema[key]
            # Use short description or label
            label_text = props.get("description", key)

            field_type = props.get("type")
            default_value = props.get("default")
            value = current_config.get(key, default_value)

            if field_type == "boolean":
                # compact checkbox
                cb = QCheckBox(label_text)
                if value:
                    cb.setChecked(True)
                cb.toggled.connect(lambda: self.config_changed.emit())
                self.current_plugin_widgets[key] = cb
                layout.addWidget(cb)
            else:
                layout.addWidget(QLabel(label_text))
                widget = self._create_input_widget(key, field_type, props, value)
                if widget:
                    layout.addWidget(widget)

        layout.addStretch()
        self.config_layout.addRow(container)

    def _add_enabled_checkbox(self, is_enabled):
        cb = QCheckBox("Enable this plugin")
        cb.setChecked(is_enabled)
        cb.toggled.connect(lambda: self.config_changed.emit())
        self.config_layout.addRow(cb)
        self.current_plugin_widgets["enabled"] = cb

    def _add_config_field(self, field, props, current_config):
        label = QLabel(props.get("description", field))
        label.setWordWrap(True)

        field_type = props.get("type")
        default_value = props.get("default")
        value = current_config.get(field, default_value)

        widget = self._create_input_widget(field, field_type, props, value)
        if widget:
            self.config_layout.addRow(label, widget)

    def _create_input_widget(self, field, field_type, props, value):
        if field_type == "string":
            return self._create_string_widget(field, props, value)
        elif field_type == "boolean":
            return self._create_bool_widget(field, value)
        elif field_type == "string_list":
            return self._create_list_widget(field, props, value)
        elif field_type == "integer":
            return self._create_int_widget(field, value)
        return None

    def _create_string_widget(self, field, props, value):
        suggestions = props.get("suggestions")
        enum_options = props.get("enum")
        widget_type = props.get("widget")

        if suggestions or enum_options:
            widget = QComboBox()
            if enum_options:
                widget.addItems(enum_options)
            else:
                widget.setEditable(True)
                widget.addItems(suggestions)

            if value:
                widget.setCurrentText(str(value))

            widget.currentTextChanged.connect(lambda: self.config_changed.emit())
            self.current_plugin_widgets[field] = widget
            return widget

        widget = QLineEdit()
        if value:
            widget.setText(str(value))
        widget.textChanged.connect(lambda: self.config_changed.emit())

        if widget_type == "file_path":
            container = QWidget()
            layout = QHBoxLayout(container)
            layout.setContentsMargins(0, 0, 0, 0)
            layout.addWidget(widget)
            btn = QPushButton("...")
            btn.clicked.connect(lambda: self.browse_file(widget))
            layout.addWidget(btn)
            self.current_plugin_widgets[field] = widget
            return container
        elif widget_type == "directory_path":
            container = QWidget()
            layout = QHBoxLayout(container)
            layout.setContentsMargins(0, 0, 0, 0)
            layout.addWidget(widget)

            # Browse button
            btn_browse = QPushButton("...")
            btn_browse.setToolTip("Browse...")
            btn_browse.setMaximumWidth(40)
            btn_browse.clicked.connect(lambda: self.browse_directory(widget))
            layout.addWidget(btn_browse)

            # Open Folder button
            btn_open = QPushButton("Open")
            btn_open.setToolTip("Open in File Manager")
            btn_open.clicked.connect(lambda: self.open_file_manager(widget.text()))
            layout.addWidget(btn_open)

            self.current_plugin_widgets[field] = widget
            return container

        self.current_plugin_widgets[field] = widget
        return widget

    def _create_bool_widget(self, field, value):
        widget = QCheckBox()
        if value:
            widget.setChecked(True)
        widget.toggled.connect(lambda: self.config_changed.emit())
        self.current_plugin_widgets[field] = widget
        return widget

    def _create_list_widget(self, field, props, value):
        widget = SearchTermsWidget()
        if props.get("suggestions"):
            widget.set_suggestions(props.get("suggestions"))
        widget.set_value(value if value is not None else props.get("default"))
        widget.changed.connect(lambda: self.config_changed.emit())
        self.current_plugin_widgets[field] = widget
        return widget

    def _create_int_widget(self, field, value):
        widget = QSpinBox()
        widget.setRange(0, 10000)
        if value is not None:
            widget.setValue(int(value))
        widget.valueChanged.connect(lambda: self.config_changed.emit())
        self.current_plugin_widgets[field] = widget
        return widget

    def browse_file(self, widget):
        path, _ = QFileDialog.getOpenFileName(self, "Select File", widget.text())
        if path:
            widget.setText(path)

    def browse_directory(self, widget):
        path = QFileDialog.getExistingDirectory(self, "Select Directory", widget.text())
        if path:
            widget.setText(path)

    def open_file_manager(self, path_str):
        """Open the directory in the default file manager."""
        if not path_str:
            return

        path = Path(path_str).expanduser().resolve()
        if path.exists() and path.is_dir():
            QDesktopServices.openUrl(QUrl.fromLocalFile(str(path)))
        else:
            QMessageBox.warning(self, "Error", f"Directory does not exist:\n{path}")

    def _configure_action_buttons(self, plugin_name):
        is_runnable = plugin_name != "local"
        self.run_btn.setEnabled(is_runnable)
        self.reset_btn.setEnabled(is_runnable)

    def get_config(self):
        """Get the configuration for this plugin."""
        config = {}
        for field, widget in self.current_plugin_widgets.items():
            if isinstance(widget, QLineEdit):
                config[field] = widget.text()
            elif isinstance(widget, QComboBox):
                config[field] = widget.currentText()
            elif isinstance(widget, QCheckBox):
                config[field] = widget.isChecked()
            elif isinstance(widget, QSpinBox):
                config[field] = widget.value()
            elif isinstance(widget, SearchTermsWidget):
                config[field] = widget.get_value()

        # Merge global font settings for the execution dialog
        config["console_font_family"] = self.config_data.get(
            "console_font_family", "Monospace"
        )
        config["console_font_size"] = self.config_data.get("console_font_size", 10)

        return config

    def run_current_plugin(self):
        current_config = self.get_config()
        current_config["force"] = True  # Always force on demand

        # Check for search terms widget
        search_terms_config = None
        for key, widget in self.current_plugin_widgets.items():
            if isinstance(widget, SearchTermsWidget):
                search_terms_config = {"key": key, "data": current_config.get(key, [])}
                break

        # Disable watcher during run to prevent crash/conflict
        original_watcher_paths = self.watcher.directories()
        if original_watcher_paths:
            self.watcher.removePaths(original_watcher_paths)

        try:
            # Launch Dialog
            dialog = PluginExecutionDialog(
                self.plugin_manager,
                self.plugin_name,
                current_config,
                title="Download Now",
                search_terms_config=search_terms_config,
                parent=self,
            )
            dialog.exec()
        finally:
            # Re-enable watcher
            self.scan_for_review()

    def reset_current_plugin(self):
        reply = QMessageBox.warning(
            self,
            "Confirm Reset",
            "Delete all files and run fresh?",
            QMessageBox.StandardButton.Yes | QMessageBox.StandardButton.No,
        )
        if reply == QMessageBox.StandardButton.Yes:
            current_config = self.get_config()
            current_config["force"] = True
            current_config["reset"] = True

            # Disable watcher
            original_watcher_paths = self.watcher.directories()
            if original_watcher_paths:
                self.watcher.removePaths(original_watcher_paths)

            try:
                dialog = PluginExecutionDialog(
                    self.plugin_manager,
                    self.plugin_name,
                    current_config,
                    title="Reset & Run",
                    parent=self,
                )
                dialog.exec()
            finally:
                self.scan_for_review()

    # display_result, update_progress removed

    def display_preview_image(self, path):
        pixmap = QPixmap(path)
        if not pixmap.isNull():
            self.preview_label.setPixmap(pixmap)

    def scan_for_review(self):
        """Scan download directory for images to review."""
        current_config = self.get_config()

        # Determine directory
        download_dir_str = ""
        if self.plugin_name == "local":
            download_dir_str = current_config.get("path", "")
        else:
            download_dir_str = current_config.get("download_dir", "")

        if not download_dir_str:
            # Fallback to schema default
            schema = self.plugin_manager.get_plugin_schema(self.plugin_name)
            if self.plugin_name == "local":
                download_dir_str = schema.get("path", {}).get("default", "")
            else:
                download_dir_str = schema.get("download_dir", {}).get("default", "")

        download_dir = Path(download_dir_str).expanduser().resolve()

        if not download_dir.exists():
            QMessageBox.warning(self, "Error", f"Directory not found:\n{download_dir}")
            return

        # Find images

        # Update Watcher
        if self.watcher.directories():
            self.watcher.removePaths(self.watcher.directories())
        self.watcher.addPath(str(download_dir))

        self.review_images = sorted(
            [
                f
                for f in download_dir.glob("*")
                if f.suffix.lower() in (".jpg", ".jpeg", ".png", ".webp", ".bmp")
            ],
            key=lambda f: f.stat().st_mtime,
            reverse=True,
        )

        if not self.review_images:
            self.preview_label.setText("No images found for review.")
            self.review_index = 0
            self.blacklisted_indices = set()
            self.update_review_ui()
            return

        self.review_index = 0
        self.blacklisted_indices = set()
        self.show_review_image()
        self.update_review_ui()

        # Grab focus for keyboard navigation
        self.setFocus()
        self.navigate_legend.setVisible(True)

    def show_review_image(self):
        """Display current review image."""
        if not self.review_images or self.review_index >= len(self.review_images):
            return

        img_path = self.review_images[self.review_index]
        pixmap = QPixmap(str(img_path))

        if pixmap.isNull():
            self.preview_label.setText(f"Error loading: {img_path.name}")
            return

        # Draw overlay if blacklisted
        if self.review_index in self.blacklisted_indices:
            painter = QPainter(pixmap)
            painter.setRenderHint(QPainter.RenderHint.Antialiasing)

            pen_width = max(5, int(min(pixmap.width(), pixmap.height()) * 0.02))
            painter.setPen(QPen(QColor(255, 0, 0), pen_width))
            painter.drawRect(0, 0, pixmap.width(), pixmap.height())
            painter.drawLine(0, 0, pixmap.width(), pixmap.height())
            painter.drawLine(pixmap.width(), 0, 0, pixmap.height())
            painter.end()

        self.preview_label.setPixmap(pixmap)

        # Update Info
        try:
            mtime = datetime.fromtimestamp(img_path.stat().st_mtime)
            relative_time = get_relative_time(mtime)
            date_str = f"{relative_time} ({mtime.strftime('%Y-%m-%d %H:%M')})"
        except Exception:
            date_str = "Unknown"

        status = (
            "[MARKED FOR DELETION]"
            if self.review_index in self.blacklisted_indices
            else ""
        )

        info_html = f"""
        <b>Image {self.review_index + 1} of {len(self.review_images)}</b><br>
        File: {img_path.name}<br>
        Date: {date_str}<br>
        <b style="color:red">{status}</b>
        <br><br>
        """
        self.log_viewer.setHtml(info_html)

    def update_review_ui(self):
        count = len(self.blacklisted_indices)
        self.apply_blacklist_btn.setText(f"Apply Blacklist ({count})")
        self.apply_blacklist_btn.setEnabled(count > 0)

    def keyPressEvent(self, event):
        if not self.review_images:
            super().keyPressEvent(event)
            return

        key = event.key()
        if key == Qt.Key.Key_Left:
            self.review_index = max(0, self.review_index - 1)
            self.show_review_image()
        elif key == Qt.Key.Key_Right:
            self.review_index = min(len(self.review_images) - 1, self.review_index + 1)
            self.show_review_image()
        elif key == Qt.Key.Key_Space:
            if self.review_index in self.blacklisted_indices:
                self.blacklisted_indices.remove(self.review_index)
            else:
                self.blacklisted_indices.add(self.review_index)
            self.show_review_image()
            self.update_review_ui()
        else:
            super().keyPressEvent(event)

    def apply_blacklist(self):
        """Apply blacklist logic (delete/move files)."""
        if not self.blacklisted_indices:
            return

        targets = [str(self.review_images[i]) for i in self.blacklisted_indices]

        # Create a temp config to run the blacklist action
        current_config = self.get_config()
        current_config["action"] = "process_blacklist"
        current_config["targets"] = targets
        current_config["force"] = True

        dialog = PluginExecutionDialog(
            self.plugin_manager,
            self.plugin_name,
            current_config,
            title="Applying Blacklist",
            parent=self,
        )
        dialog.exec()

        # Rescan to refresh list
        self.scan_for_review()

    # on_blacklist_complete removed
