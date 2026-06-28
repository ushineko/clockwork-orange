#!/usr/bin/env python
"""Tests for the service daemon's config-watcher debounce coalescing.

Verifies that a burst of rapid config-file writes (as happens at login) is
coalesced into a single trigger, while a settled single change still triggers
promptly. See specs/009-config-watcher-debounce.md.
"""
import importlib.util
import sys
import threading
import time
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent


def _load_main_module():
    """Load the hyphenated main script as an importable module."""
    if str(REPO_ROOT) not in sys.path:
        sys.path.insert(0, str(REPO_ROOT))
    spec = importlib.util.spec_from_file_location(
        "clockwork_orange_main", REPO_ROOT / "clockwork-orange.py"
    )
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


co = _load_main_module()


class TestConfigDebounce(unittest.TestCase):
    """Contract tests for _drain_config_change_burst coalescing behavior."""

    def test_single_change_returns_after_quiet_window(self):
        """A settled single change returns after ~debounce, not immediately."""
        event = threading.Event()
        event.set()  # caller already observed the first change

        start = time.monotonic()
        co._drain_config_change_burst(
            event, debounce_seconds=0.2, is_shutdown=lambda: False
        )
        elapsed = time.monotonic() - start

        # Returned after roughly the quiet window: not instant, not a long wait.
        self.assertGreaterEqual(elapsed, 0.18)
        self.assertLess(elapsed, 0.6)
        self.assertFalse(event.is_set())

    def test_burst_is_coalesced_into_single_return(self):
        """A burst of writes keeps resetting the window; return only after quiet.

        Encodes the contract that the function does not return mid-burst — it
        waits until the writes have settled for the full debounce window. That
        is what collapses a login-time flurry into one wallpaper switch.
        """
        event = threading.Event()
        event.set()  # initial change observed by caller

        last_write = {"t": None}

        def burst():
            # Five rapid writes spaced 0.05s apart (~0.25s total), then quiet.
            for _ in range(5):
                time.sleep(0.05)
                last_write["t"] = time.monotonic()
                event.set()

        writer = threading.Thread(target=burst)
        start = time.monotonic()
        writer.start()
        co._drain_config_change_burst(
            event, debounce_seconds=0.2, is_shutdown=lambda: False
        )
        elapsed = time.monotonic() - start
        writer.join()

        # Must not return before the burst ends; must return only after the
        # quiet window following the LAST write.
        self.assertIsNotNone(last_write["t"])
        quiet_since_last_write = time.monotonic() - last_write["t"]
        self.assertGreaterEqual(quiet_since_last_write, 0.18)
        # Total elapsed covers the burst (~0.25s) plus the quiet window (~0.2s).
        self.assertGreaterEqual(elapsed, 0.4)
        self.assertFalse(event.is_set())

    def test_returns_promptly_on_shutdown(self):
        """Shutdown short-circuits the debounce wait."""
        event = threading.Event()
        event.set()

        start = time.monotonic()
        # A long debounce window would block for seconds if shutdown were not
        # honored; is_shutdown=True must make it return without a full wait.
        co._drain_config_change_burst(
            event, debounce_seconds=30.0, is_shutdown=lambda: True
        )
        elapsed = time.monotonic() - start

        self.assertLess(elapsed, 1.0)


if __name__ == "__main__":
    unittest.main()
