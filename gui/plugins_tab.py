#!/usr/bin/env python3
"""
Plugin management tab for the GUI.
"""
import sys
import json
from pathlib import Path
from PyQt6.QtWidgets import (QWidget, QVBoxLayout, QHBoxLayout, QPushButton, 
                             QLabel, QComboBox, QGroupBox, QLineEdit, QCheckBox,
                             QScrollArea, QFormLayout, QFileDialog, QMessageBox, 
                             QTextEdit, QListWidget, QSpinBox, QSizePolicy)
from PyQt6.QtCore import Qt, pyqtSignal, QThread
from PyQt6.QtGui import QPixmap, QFont, QPainter, QColor, QPen

# Ensure we can import the plugin manager
sys.path.append(str(Path(__file__).parent.parent))
from plugin_manager import PluginManager

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
                    line = str(item)
                    self.log_signal.emit(line)
                    
                    # Check for progress marker
                    # Format: ::PROGRESS:: <percent> :: <message>
                    if "::PROGRESS::" in line:
                        try:
                            parts = line.split("::")
                            # [..., "PROGRESS", " 50 ", " Message"]
                            if len(parts) >= 4:
                                percent = int(parts[2].strip())
                                message = parts[3].strip()
                                self.progress_signal.emit(percent, message)
                        except Exception:
                            pass
                            
                    # Check for image saved marker
                    # Format: ::IMAGE_SAVED:: <path>
                    if "::IMAGE_SAVED::" in line:
                        try:
                            parts = line.split("::IMAGE_SAVED::")
                            if len(parts) >= 2:
                                image_path = parts[1].strip()
                                self.image_saved_signal.emit(image_path)
                        except Exception:
                            pass
                            
            if final_result:
                self.finished_signal.emit(final_result)
            else:
                self.finished_signal.emit({"status": "error", "message": "No result returned"})
        except Exception as e:
            self.log_signal.emit(f"Error in thread: {str(e)}")
            self.finished_signal.emit({"status": "error", "message": str(e), "logs": str(e)})

class PluginsTab(QWidget):
    """Tab for configuring plugins."""
    
    config_changed = pyqtSignal()
    
    def __init__(self, config_data: dict):
        super().__init__()
        self.config_data = config_data
        self.plugin_manager = PluginManager()
        self.current_plugin_widgets = {}
        self.last_plugin_name = None
        
        # Review state
        self.review_images = []
        self.review_index = 0
        self.blacklisted_indices = set()
        
        self.init_ui()
        
    def on_plugin_selection_changed(self, index):
        """Handle plugin selection change."""
        plugin_id = self.plugin_combo.itemData(index)
        if plugin_id:
            self.load_plugin_config_ui(plugin_id)
            # Reset review state on plugin change
            self.review_images = []
            self.review_index = 0
            self.blacklisted_indices = set()
            self.update_review_ui()

        
    def init_ui(self):
        # Enable keyboard focus for review navigation
        self.setFocusPolicy(Qt.FocusPolicy.StrongFocus)
        
        main_layout = QVBoxLayout()
        
        # Plugin Selection (Top)
        selection_group = QGroupBox("Select Plugin")
        selection_layout = QHBoxLayout()
        
        selection_layout.addWidget(QLabel("Active Plugin:"))
        self.plugin_combo = QComboBox()
        for plugin_id in self.plugin_manager.get_available_plugins():
            friendly_name = plugin_id.replace('_', ' ').title()
            self.plugin_combo.addItem(friendly_name, plugin_id)
        self.plugin_combo.currentIndexChanged.connect(self.on_plugin_selection_changed)
        selection_layout.addWidget(self.plugin_combo)
        
        selection_group.setLayout(selection_layout)
        main_layout.addWidget(selection_group)
        
        # Content Area (2 Columns)
        content_layout = QHBoxLayout()
        
        # --- Left Column: Configuration ---
        left_layout = QVBoxLayout()
        self.config_group = QGroupBox("Plugin Configuration")
        self.config_layout = QFormLayout()
        self.config_group.setLayout(self.config_layout)
        
        scroll = QScrollArea()
        scroll.setWidget(self.config_group)
        scroll.setWidgetResizable(True)
        left_layout.addWidget(scroll)
        
        # Review & Cleanup Controls
        review_group = QGroupBox("Review & Cleanup")
        review_layout = QVBoxLayout()
        
        scan_btn = QPushButton("Scan Images for Review")
        scan_btn.clicked.connect(self.scan_for_review)
        review_layout.addWidget(scan_btn)
        
        self.apply_blacklist_btn = QPushButton("Apply Blacklist (0 marked)")
        self.apply_blacklist_btn.clicked.connect(self.apply_blacklist)
        self.apply_blacklist_btn.setEnabled(False)
        self.apply_blacklist_btn.setStyleSheet("background-color: #552222;") 
        review_layout.addWidget(self.apply_blacklist_btn)
        
        review_group.setLayout(review_layout)
        left_layout.addWidget(review_group)
        
        content_layout.addLayout(left_layout, 1) # Stretch 1
        
        # --- Right Column: Preview, Logs, Actions ---
        right_layout = QVBoxLayout()
        
        # Live Preview (Top of right column, expanding)
        preview_group = QGroupBox("Live Preview")
        preview_layout = QVBoxLayout()
        preview_layout.setContentsMargins(0, 5, 0, 0)
        
        self.preview_label = QLabel("No image downloaded yet")
        self.preview_label.setAlignment(Qt.AlignmentFlag.AlignCenter)
        self.preview_label.setSizePolicy(QSizePolicy.Policy.Ignored, QSizePolicy.Policy.Ignored)
        self.preview_label.setStyleSheet("background-color: #222; border: 1px solid #444;")
        self.preview_label.setMinimumHeight(300) 
        preview_layout.addWidget(self.preview_label)
        
        preview_group.setLayout(preview_layout)
        right_layout.addWidget(preview_group, 2) # Stretch 2 (give more space)
        
        # Execution (Bottom of right column)
        run_group = QGroupBox("Execution")
        run_layout = QVBoxLayout()
        
        # Logs
        self.log_viewer = QTextEdit()
        self.log_viewer.setReadOnly(True)
        self.log_viewer.setPlaceholderText("Execution logs...")
        self.log_viewer.setMaximumHeight(150)
        run_layout.addWidget(self.log_viewer)
        
        # Buttons
        btn_layout = QHBoxLayout()
        run_btn = QPushButton("Run Plugin Now")
        run_btn.clicked.connect(self.run_current_plugin)
        btn_layout.addWidget(run_btn)
        
        reset_btn = QPushButton("Reset && Run")
        reset_btn.setStyleSheet("background-color: #552222;") # Subtle red tint
        reset_btn.clicked.connect(self.reset_current_plugin)
        btn_layout.addWidget(reset_btn)
        run_layout.addLayout(btn_layout)
        
        run_group.setLayout(run_layout)
        right_layout.addWidget(run_group, 0)
        
        content_layout.addLayout(right_layout, 1) # Stretch 1
        
        main_layout.addLayout(content_layout)
        
        # Initial load
        active_plugin = self.config_data.get('active_plugin')
        if active_plugin:
            index = self.plugin_combo.findData(active_plugin)
            if index >= 0:
                self.plugin_combo.setCurrentIndex(index)
        
        # Trigger load for current item (if any)
        if self.plugin_combo.count() > 0:
            self.on_plugin_selection_changed(self.plugin_combo.currentIndex())
            
        self.setLayout(main_layout)
        
        
    def _save_plugin_widgets_to_memory(self, plugin_name):
        """Save current widget values to internal config data."""
        if not plugin_name or plugin_name not in self.plugin_manager.get_available_plugins():
            return

        plugin_config = {}
        for field, widget in self.current_plugin_widgets.items():
            if isinstance(widget, QLineEdit):
                plugin_config[field] = widget.text()
            elif isinstance(widget, QComboBox):
                plugin_config[field] = widget.currentText()
            elif isinstance(widget, QCheckBox):
                plugin_config[field] = widget.isChecked()
            elif isinstance(widget, QSpinBox):
                plugin_config[field] = widget.value()
        
        # Ensure 'plugins' key exists
        if 'plugins' not in self.config_data:
            self.config_data['plugins'] = {}
            
        self.config_data['plugins'][plugin_name] = plugin_config

    def load_plugin_config_ui(self, plugin_name: str):
        """Load configuration widgets for the selected plugin."""
        # Save previous plugin configuration first
        if self.last_plugin_name and self.current_plugin_widgets:
             self._save_plugin_widgets_to_memory(self.last_plugin_name)
        
        self.last_plugin_name = plugin_name
        
        # Clear existing widgets
        while self.config_layout.count():
            item = self.config_layout.takeAt(0)
            widget = item.widget()
            if widget:
                widget.deleteLater()
        
        self.current_plugin_widgets = {}
        
        # Clear logs
        self.log_viewer.clear()
        
        try:
            schema = self.plugin_manager.get_plugin_schema(plugin_name)
            current_config = self.config_data.get('plugins', {}).get(plugin_name, {})
            
            for field, props in schema.items():
                label = QLabel(props.get('description', field))
                label.setWordWrap(True)
                font = label.font()
                font.setPointSize(9)
                label.setFont(font)
                
                widget = None
                
                field_type = props.get('type')
                default_value = props.get('default')
                value = current_config.get(field, default_value)
                
                if field_type == 'string':
                    suggestions = props.get('suggestions')
                    if suggestions:
                        widget = QComboBox()
                        widget.setEditable(True)
                        widget.addItems(suggestions)
                        if value:
                            widget.setCurrentText(str(value))
                        widget.editTextChanged.connect(lambda: self.config_changed.emit())
                        # Store reference
                        self.current_plugin_widgets[field] = widget
                    else:
                        widget = QLineEdit()
                        if value:
                            widget.setText(str(value))
                        widget.textChanged.connect(lambda: self.config_changed.emit())
                        
                        # Handle special widgets
                        widget_type = props.get('widget')
                        if widget_type == 'file_path':
                            container = QWidget()
                            h_layout = QHBoxLayout(container)
                            h_layout.setContentsMargins(0, 0, 0, 0)
                            h_layout.addWidget(widget)
                            btn = QPushButton("Browse...")
                            btn.clicked.connect(lambda checked, w=widget: self.browse_file(w))
                            h_layout.addWidget(btn)
                            widget = container
                            self.current_plugin_widgets[field] = container.findChild(QLineEdit) # Store the line edit
                        elif widget_type == 'directory_path':
                            container = QWidget()
                            h_layout = QHBoxLayout(container)
                            h_layout.setContentsMargins(0, 0, 0, 0)
                            h_layout.addWidget(widget)
                            btn = QPushButton("Browse...")
                            btn.clicked.connect(lambda checked, w=widget: self.browse_directory(w))
                            h_layout.addWidget(btn)
                            widget = container
                            self.current_plugin_widgets[field] = container.findChild(QLineEdit)
                        else:
                            self.current_plugin_widgets[field] = widget
                        
                elif field_type == 'boolean':
                    widget = QCheckBox()
                    if value:
                        widget.setChecked(True)
                    widget.toggled.connect(lambda: self.config_changed.emit())
                    self.current_plugin_widgets[field] = widget
                    
                elif field_type == 'integer':
                    widget = QSpinBox()
                    widget.setRange(0, 10000) # Reasonable range
                    if value is not None:
                        widget.setValue(int(value))
                    widget.valueChanged.connect(lambda: self.config_changed.emit())
                    self.current_plugin_widgets[field] = widget

                    
                elif field_type == 'integer':
                    widget = QSpinBox()
                    widget.setRange(0, 10000) # Reasonable range
                    if value is not None:
                        widget.setValue(int(value))
                    self.current_plugin_widgets[field] = widget
                
                if widget:
                    self.config_layout.addRow(label, widget)
                    
        except Exception as e:
            self.config_layout.addRow(QLabel(f"Error loading schema: {e}"))
            
        # Trigger auto-save to persist active_plugin change
        self.config_changed.emit()
            
    def browse_file(self, line_edit):
        path, _ = QFileDialog.getOpenFileName(self, "Select File")
        if path:
            line_edit.setText(path)
            
    def browse_directory(self, line_edit):
        path = QFileDialog.getExistingDirectory(self, "Select Directory")
        if path:
            line_edit.setText(path)

    def get_config(self):
        """Get the current configuration from the UI."""
        plugin_name = self.plugin_combo.currentData()
        if not plugin_name:
            return {}
            
        plugin_config = {}
        for field, widget in self.current_plugin_widgets.items():
            if isinstance(widget, QLineEdit):
                plugin_config[field] = widget.text()
            elif isinstance(widget, QComboBox):
                plugin_config[field] = widget.currentText()
            elif isinstance(widget, QCheckBox):
                plugin_config[field] = widget.isChecked()
            elif isinstance(widget, QSpinBox):
                plugin_config[field] = widget.value()
                
        # We return the whole plugins structure
        # Preserve other plugins' config if present
        all_plugins_config = self.config_data.get('plugins', {})
        all_plugins_config[plugin_name] = plugin_config
        
        # NOTE: ConfigManagerWidget.get_config_from_ui needs to be updated to capture 'active_plugin'
        # The return value here is just the 'plugins' dict.
        # We need a way to return the active plugin name too. 
        # But for now, let's just assume we return the plugins dict.
        # The ConfigManagerWidget will need to access the combobox directly or we need to change return type.
        
        return all_plugins_config

    def get_active_plugin(self):
        """Return the currently selected plugin name."""
        return self.plugin_combo.currentData()

    def update_config_data(self, new_config_data):
        """Update the internal config data and refresh UI."""
        self.config_data = new_config_data
        
        active_plugin = self.config_data.get('active_plugin')
        if active_plugin:
            index = self.plugin_combo.findData(active_plugin)
            if index >= 0:
                blocker = self.plugin_combo.blockSignals(True)
                self.plugin_combo.setCurrentIndex(index)
                self.plugin_combo.blockSignals(blocker)
                # Always reload UI with new config data for the active plugin
                self.load_plugin_config_ui(active_plugin)
        elif self.plugin_combo.count() > 0:
            # Refresh if no active plugin specified
            self.plugin_combo.setCurrentIndex(0)
            self.on_plugin_selection_changed(0)

    def run_current_plugin(self):
        """Execute the currently selected plugin."""
        self._execute_plugin_internal(reset=False)

    def reset_current_plugin(self):
        """Reset (clear data) and execute the plugin."""
        plugin_name = self.plugin_combo.currentText()
        if not plugin_name:
            return

        # Confirmation
        reply = QMessageBox.warning(
            self, 
            "Confirm Reset",
            f"This will DELETE ALL FILES in the download directory for '{plugin_name}'.\n\nAre you sure you want to continue?",
            QMessageBox.StandardButton.Yes | QMessageBox.StandardButton.No,
            QMessageBox.StandardButton.No
        )
        
        if reply == QMessageBox.StandardButton.Yes:
            self._execute_plugin_internal(reset=True)

    def _execute_plugin_internal(self, reset=False):
        """Internal method to execute plugin with options."""
        plugin_name = self.plugin_combo.currentData()
        if not plugin_name:
            return
            
        # Get transient config from UI
        current_config = {}
        for field, widget in self.current_plugin_widgets.items():
            if isinstance(widget, QLineEdit):
                current_config[field] = widget.text()
            elif isinstance(widget, QComboBox):
                current_config[field] = widget.currentText()
            elif isinstance(widget, QCheckBox):
                current_config[field] = widget.isChecked()
            elif isinstance(widget, QSpinBox):
                current_config[field] = widget.value()
                
        # Force run for manual execution
        current_config['force'] = True
        
        # Apply reset flag if requested
        if reset:
            current_config['reset'] = True
        
        self.log_viewer.clear()
        action = "Resetting and executing" if reset else "Starting execution of"
        self.log_viewer.append(f"{action} {plugin_name}...")
        
        # Run plugin through manager
        self.runner = PluginRunner(self.plugin_manager, plugin_name, current_config)
        self.runner.log_signal.connect(self.log_viewer.append)
        self.runner.image_saved_signal.connect(self.display_preview_image)
        self.runner.finished_signal.connect(self.display_result)
        
        # Connect to parent's progress bar if available
        # We need to find the ConfigManagerWidget parent
        parent = self.parent()
        found_progress = False
        while parent:
            if hasattr(parent, 'update_progress'):
                self.runner.progress_signal.connect(parent.update_progress)
                # Reset progress bar on start
                parent.update_progress(0, "Starting...")
                # Hide on finish
                self.runner.finished_signal.connect(lambda r: parent.update_progress(-1))
                found_progress = True
                break
            parent = parent.parent()
            
        if not found_progress:
            print("Warning: Could not find parent ConfigManagerWidget to update progress bar")
            
        self.runner.start()

    
    # --- Review & Blacklist Logic ---

    def scan_for_review(self):
        """Scan the current plugin's download directory for images."""
        current_plugin_name = self.get_active_plugin()
        current_config = self.get_config().get(current_plugin_name, {})
        download_dir_str = current_config.get('download_dir', '') # might come from schema default if missing in config? 
        # Actually get_config returns what is in UI. Default is populated in UI.
        
        if not download_dir_str:
            # Fallback to schema default if not in UI yet?
            schema = self.plugin_manager.get_plugin_schema(current_plugin_name)
            download_dir_str = schema.get('download_dir', {}).get('default', '')
            
        download_dir = Path(download_dir_str)
        
        if not download_dir.exists():
            QMessageBox.warning(self, "Directory Not Found", f"Directory does not exist:\n{download_dir}")
            return
            
        self.review_images = sorted(
            [f for f in download_dir.glob("*") if f.suffix.lower() in ('.jpg', '.jpeg', '.png', '.webp')],
            key=lambda f: f.stat().st_mtime, reverse=True
        )
        
        if not self.review_images:
            self.preview_label.setText("No images found in download directory.")
            self.review_index = 0
        else:
            self.review_index = 0
            self.show_review_image()
            
        self.blacklisted_indices = set()
        self.update_review_ui()
        # Grab focus for keyboard navigation
        self.setFocus()
        
    def update_review_ui(self):
        """Update review UI state."""
        count = len(self.blacklisted_indices)
        self.apply_blacklist_btn.setText(f"Apply Blacklist ({count} marked)")
        self.apply_blacklist_btn.setEnabled(count > 0)
        
    def show_review_image(self):
        """Display the current review image with overlays."""
        if not self.review_images or self.review_index >= len(self.review_images):
            return
            
        img_path = self.review_images[self.review_index]
        
        # Load pixmap using existing resize logic (we can reuse display_preview_image or partial logic)
        # But we need to draw over it.
        pixmap = QPixmap(str(img_path))
        if pixmap.isNull():
             self.preview_label.setText(f"Failed to load: {img_path.name}")
             return
             
        # Scale first
        target_size = self.preview_label.size()
        if target_size.width() < 10: target_size = self.preview_label.sizeHint()
        
        scaled_pixmap = pixmap.scaled(
            target_size,
            Qt.AspectRatioMode.KeepAspectRatio,
            Qt.TransformationMode.SmoothTransformation
        )
        
        # Draw overlay if blacklisted
        if self.review_index in self.blacklisted_indices:
            painter = QPainter(scaled_pixmap)
            painter.setRenderHint(QPainter.RenderHint.Antialiasing)
            
            # Red X or Border
            painter.setPen(QPen(QColor(255, 0, 0), 10))
            painter.drawRect(0, 0, scaled_pixmap.width(), scaled_pixmap.height())
            
            # Cross
            painter.drawLine(0, 0, scaled_pixmap.width(), scaled_pixmap.height())
            painter.drawLine(scaled_pixmap.width(), 0, 0, scaled_pixmap.height())
            
            painter.end()
            
        self.preview_label.setPixmap(scaled_pixmap)
        
        # Update text or status?
        self.log_viewer.setText(f"Reviewing {self.review_index + 1}/{len(self.review_images)}: {img_path.name}\n[Left/Right] Navigate  [Space] Mark to Delete")
        
    def keyPressEvent(self, event):
        """Handle keyboard navigation for review mode."""
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
            # Toggle blacklist
            if self.review_index in self.blacklisted_indices:
                self.blacklisted_indices.remove(self.review_index)
            else:
                self.blacklisted_indices.add(self.review_index)
            self.show_review_image()
            self.update_review_ui()
        else:
            super().keyPressEvent(event)
            
    def apply_blacklist(self):
        """Execute the blacklist process."""
        if not self.blacklisted_indices:
            return
            
        # Collect paths
        targets = [str(self.review_images[i]) for i in self.blacklisted_indices]
        
        plugin_name = self.plugin_combo.currentData()
        current_config = self.get_config().get(plugin_name, {})
        current_config['action'] = 'process_blacklist'
        current_config['targets'] = targets
        
        # Execute
        self.log_viewer.clear()
        self.log_viewer.append(f"Applying blacklist to {len(targets)} files...")
        
        self.runner = PluginRunner(self.plugin_manager, plugin_name, current_config)
        self.runner.log_signal.connect(self.log_viewer.append)
        self.runner.finished_signal.connect(self.on_blacklist_complete)
        self.runner.start()
        
    def on_blacklist_complete(self, result):
        """Handle completion of blacklist action."""
        self.display_result(result)
        # Rescan to update list
        self.scan_for_review()
        
    def display_result(self, result):
        self.log_viewer.append("\n--- Execution Finished ---")
        
        # Display logs if available
        if 'logs' in result:
             self.log_viewer.append("\n[LOGS]")
             self.log_viewer.append(result['logs'])
             
        self.log_viewer.append("\n[RESULT]")
        self.log_viewer.append(json.dumps(result, indent=2))
        
        # If success, update UI or allow saving?
        # User might want to save if it worked well.

    def display_preview_image(self, image_path):
        """Display the downloaded image in the preview area."""
        try:
            pixmap = QPixmap(image_path)
            if not pixmap.isNull():
                # Scale to fit current label size, keep aspect ratio
                # Use a slightly smaller size than label to avoid scrollbars/jitter if policy is strict
                # But with expanding policy it should be fine.
                target_size = self.preview_label.size()
                if target_size.width() < 10 or target_size.height() < 10:
                    target_size = self.preview_label.sizeHint() # Fallback

                scaled_pixmap = pixmap.scaled(
                    target_size,
                    Qt.AspectRatioMode.KeepAspectRatio,
                    Qt.TransformationMode.SmoothTransformation
                )
                self.preview_label.setPixmap(scaled_pixmap)
                self.preview_label.setText("")  # Clear text
            else:
                self.preview_label.setText(f"Failed to load image: {Path(image_path).name}")
        except Exception as e:
            self.preview_label.setText(f"Error previewing image: {e}")
