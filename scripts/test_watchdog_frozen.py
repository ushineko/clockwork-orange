#!/usr/bin/env python
"""
Isolated test script to verify watchdog modules are properly frozen.

IMPORTANT: This script writes ALL output to a log file next to the executable.
If the exe crashes, check watchdog_test.log for diagnostics.

All imports are done INSIDE functions with full tracebacks to catch failures.
"""
import sys
import os
import traceback

# Log file path - next to executable
LOG_FILE = None


def setup_logging():
    """Set up logging to file next to executable."""
    global LOG_FILE
    try:
        if getattr(sys, 'frozen', False):
            exe_dir = os.path.dirname(sys.executable)
        else:
            exe_dir = os.path.dirname(os.path.abspath(__file__))
        LOG_FILE = os.path.join(exe_dir, 'watchdog_test.log')
        with open(LOG_FILE, 'w') as f:
            f.write("=" * 70 + "\n")
            f.write("WATCHDOG FROZEN MODULE TEST LOG\n")
            f.write("=" * 70 + "\n\n")
        return True
    except Exception as e:
        print(f"Warning: Could not set up log file: {e}")
        return False


def log(msg):
    """Write to both stdout and log file."""
    print(msg)
    if LOG_FILE:
        try:
            with open(LOG_FILE, 'a') as f:
                f.write(msg + '\n')
        except:
            pass


def log_exception(context):
    """Log full exception with traceback."""
    exc_info = traceback.format_exc()
    log(f"EXCEPTION in {context}:")
    log(exc_info)


def get_module_path(module):
    """Get the source location of an imported module."""
    try:
        if hasattr(module, '__file__') and module.__file__:
            return module.__file__
        elif hasattr(module, '__path__'):
            return str(list(module.__path__))
        elif hasattr(module, '__spec__') and module.__spec__:
            return str(module.__spec__.origin)
        else:
            return "<built-in or unknown>"
    except Exception as e:
        return f"<error: {e}>"


def is_from_frozen_bundle(path, meipass):
    """Check if a path is from the PyInstaller frozen bundle."""
    if not path or path.startswith('<'):
        return None
    if meipass:
        return path.startswith(meipass)
    return '_MEI' in path


def is_from_site_packages(path):
    """Check if a path is from site-packages."""
    if not path or path.startswith('<'):
        return False
    return 'site-packages' in path.lower()


def test_single_import(module_name, meipass):
    """Test importing a single module with full diagnostics."""
    log(f"\n  Testing: {module_name}")
    try:
        # Try the import
        module = __import__(module_name, fromlist=[''])
        path = get_module_path(module)
        log(f"    Path: {path}")

        # Check source
        from_bundle = is_from_frozen_bundle(path, meipass)
        from_site = is_from_site_packages(path)

        if from_bundle:
            log(f"    Status: OK (from bundle)")
            return True, "bundle"
        elif from_site:
            log(f"    Status: FAIL (from site-packages!)")
            return False, "site-packages"
        elif path.startswith('<'):
            log(f"    Status: OK (built-in)")
            return True, "builtin"
        else:
            log(f"    Status: UNKNOWN SOURCE")
            return True, "unknown"

    except ImportError as e:
        log(f"    ImportError: {e}")
        log_exception(f"importing {module_name}")
        return False, "import_error"
    except Exception as e:
        log(f"    Exception: {e}")
        log_exception(f"importing {module_name}")
        return False, "exception"


def main():
    # Set up logging FIRST
    setup_logging()

    log("=" * 70)
    log("WATCHDOG FROZEN MODULE VERIFICATION TEST")
    log("=" * 70)
    log("")

    # System info
    log("SYSTEM INFO:")
    log(f"  Python version: {sys.version}")
    log(f"  Platform: {sys.platform}")

    is_frozen = getattr(sys, 'frozen', False)
    meipass = getattr(sys, '_MEIPASS', None)

    log(f"  sys.frozen: {is_frozen}")
    log(f"  sys._MEIPASS: {meipass or 'NOT SET'}")
    log(f"  sys.executable: {sys.executable}")
    log(f"  sys.path:")
    for p in sys.path:
        log(f"    - {p}")
    log("")

    if not is_frozen:
        log("WARNING: Not running as frozen executable!")
        log("         Results will show local Python paths.")
        log("")

    # Modules to test
    watchdog_modules = [
        'watchdog',
        'watchdog.events',
        'watchdog.observers',
        'watchdog.observers.api',
        'watchdog.observers.polling',
        'watchdog.utils',
        'watchdog.utils.bricks',
        'watchdog.utils.delayed_queue',
        'watchdog.utils.dirsnapshot',
        'watchdog.utils.event_debouncer',
        'watchdog.utils.patterns',
        'watchdog.utils.platform',
    ]

    # Windows-specific modules
    if sys.platform == 'win32':
        watchdog_modules.extend([
            'watchdog.observers.winapi',
            'watchdog.observers.read_directory_changes',
        ])

    log("=" * 70)
    log("MODULE IMPORT TESTS")
    log("=" * 70)

    results = {}
    all_ok = True
    any_site_packages = False

    for mod_name in watchdog_modules:
        success, source = test_single_import(mod_name, meipass)
        results[mod_name] = (success, source)
        if not success:
            all_ok = False
        if source == "site-packages":
            any_site_packages = True

    # Functionality test
    log("")
    log("=" * 70)
    log("FUNCTIONALITY TEST")
    log("=" * 70)
    log("")

    try:
        log("Attempting to import Observer and FileSystemEventHandler...")
        from watchdog.observers import Observer
        from watchdog.events import FileSystemEventHandler

        log("  Import successful")

        # Check Observer class
        observer = Observer()
        observer_class = type(observer).__name__
        observer_module = type(observer).__module__

        log(f"  Observer class: {observer_class}")
        log(f"  Observer module: {observer_module}")

        # Check module path
        obs_mod = sys.modules.get(observer_module)
        if obs_mod:
            obs_path = get_module_path(obs_mod)
            log(f"  Observer module path: {obs_path}")

            if is_from_site_packages(obs_path):
                log("  FAIL: Observer loaded from site-packages!")
                all_ok = False
                any_site_packages = True
            elif is_frozen and is_from_frozen_bundle(obs_path, meipass):
                log("  OK: Observer loaded from bundle")

        # Create handler
        handler = FileSystemEventHandler()
        log(f"  Created FileSystemEventHandler: OK")
        # Use handler to avoid unused variable warning
        log(f"  Handler class: {type(handler).__name__}")

    except Exception as e:
        log(f"  FAILED: {e}")
        log_exception("functionality test")
        all_ok = False

    # Summary
    log("")
    log("=" * 70)
    log("SUMMARY")
    log("=" * 70)
    log("")

    failed_modules = [m for m, (s, _) in results.items() if not s]
    site_modules = [m for m, (_, src) in results.items() if src == "site-packages"]

    if failed_modules:
        log(f"FAILED IMPORTS ({len(failed_modules)}):")
        for m in failed_modules:
            log(f"  - {m}")
        log("")

    if site_modules:
        log(f"LOADED FROM SITE-PACKAGES ({len(site_modules)}):")
        for m in site_modules:
            log(f"  - {m}")
        log("")

    if not is_frozen:
        log("STATUS: NOT FROZEN (test ran in interpreter)")
        log("To properly test, build and run as frozen executable.")
        exit_code = 0
    elif all_ok and not any_site_packages:
        log("STATUS: PASSED")
        log("All watchdog modules are loading from the frozen bundle.")
        exit_code = 0
    elif any_site_packages:
        log("STATUS: FAILED - MODULES FROM SITE-PACKAGES")
        log("Some modules loaded from local Python, not the bundle.")
        exit_code = 1
    else:
        log("STATUS: FAILED - IMPORT ERRORS")
        log("Some watchdog modules failed to import.")
        exit_code = 1

    log("")
    log(f"Exit code: {exit_code}")
    log(f"Log file: {LOG_FILE}")

    # Flush
    sys.stdout.flush()
    sys.stderr.flush()

    os._exit(exit_code)


if __name__ == '__main__':
    try:
        main()
    except Exception as e:
        # Last resort error handling
        setup_logging()
        log(f"FATAL ERROR: {e}")
        log_exception("main")
        sys.stdout.flush()
        os._exit(1)
