#!/usr/bin/env python
"""
Test module to verify all required imports work correctly.

This test can be run both in development (python) and frozen (PyInstaller) contexts.
It validates that all platform-specific and dynamic imports are available.
"""
import sys
import unittest


class TestFrozenImports(unittest.TestCase):
    """Test that all required modules can be imported."""

    def test_standard_library(self):
        """Test standard library imports."""
        import ctypes
        import sqlite3
        import ssl
        self.assertTrue(ctypes)
        self.assertTrue(sqlite3)
        self.assertTrue(ssl)

    def test_pillow(self):
        """Test Pillow (PIL) import."""
        import PIL
        from PIL import Image
        self.assertTrue(PIL)
        self.assertTrue(Image)

    def test_requests(self):
        """Test requests import."""
        import requests
        self.assertTrue(requests)

    def test_yaml(self):
        """Test PyYAML import."""
        import yaml
        self.assertTrue(yaml)

    def test_watchdog_base(self):
        """Test watchdog base package import."""
        import watchdog
        self.assertTrue(watchdog)

    def test_watchdog_events(self):
        """Test watchdog.events import."""
        from watchdog.events import FileSystemEventHandler
        self.assertTrue(FileSystemEventHandler)

    def test_watchdog_observers(self):
        """Test watchdog.observers import."""
        from watchdog.observers import Observer
        self.assertTrue(Observer)

    def test_watchdog_platform_specific(self):
        """Test platform-specific watchdog observer is available."""
        from watchdog.observers import Observer
        # Create an instance to verify the platform-specific backend loads
        observer = Observer()
        self.assertTrue(observer)
        # Check the class name to verify correct backend
        class_name = observer.__class__.__name__
        if sys.platform == 'win32':
            self.assertIn('Windows', class_name)
        print(f"Observer class: {class_name}")

    def test_watchdog_winapi(self):
        """Test Windows API watchdog module (only on Windows).

        This is the critical module that read_directory_changes depends on.
        If this import fails, file watching will break on bare Windows installs.
        """
        if sys.platform != 'win32':
            self.skipTest("Windows-specific test")

        # This is the underlying module that read_directory_changes imports from
        # PyInstaller won't auto-detect this dependency
        from watchdog.observers import winapi
        self.assertTrue(winapi)

    def test_watchdog_read_directory_changes(self):
        """Test Windows-specific watchdog module (only on Windows)."""
        if sys.platform != 'win32':
            self.skipTest("Windows-specific test")

        # This is the module that PyInstaller often misses
        from watchdog.observers import read_directory_changes
        self.assertTrue(read_directory_changes)
        self.assertTrue(read_directory_changes.WindowsApiObserver)


class TestFrozenEnvironment(unittest.TestCase):
    """Test frozen executable environment."""

    def test_is_frozen(self):
        """Report whether running in frozen context."""
        is_frozen = getattr(sys, 'frozen', False)
        print(f"Frozen: {is_frozen}")
        print(f"Python: {sys.version}")
        print(f"Platform: {sys.platform}")
        # This test always passes - it's informational
        self.assertTrue(True)


if __name__ == '__main__':
    # Run with verbose output
    unittest.main(verbosity=2)
