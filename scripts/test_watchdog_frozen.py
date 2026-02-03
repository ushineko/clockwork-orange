#!/usr/bin/env python
"""
Isolated test script to verify watchdog modules are properly frozen.

This script checks that watchdog and all its dependencies are loaded from
the frozen bundle (_MEIPASS) and NOT from the local Python interpreter.

Run this as a frozen executable to verify PyInstaller bundling works correctly.
"""
import sys
import os


def get_module_source(module):
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


def is_from_frozen_bundle(path):
    """Check if a path is from the PyInstaller frozen bundle."""
    if not path or path.startswith('<'):
        return None  # Unknown/built-in

    # PyInstaller extracts to a temp dir with _MEI in the name
    meipass = getattr(sys, '_MEIPASS', None)
    if meipass:
        return path.startswith(meipass)

    # Fallback: check for common frozen indicators
    return '_MEI' in path or 'AppData\\Local\\Temp' in path


def is_from_site_packages(path):
    """Check if a path is from site-packages (local Python install)."""
    if not path or path.startswith('<'):
        return False
    return 'site-packages' in path.lower() or 'lib\\python' in path.lower()


def main():
    print("=" * 70)
    print("WATCHDOG FROZEN MODULE VERIFICATION TEST")
    print("=" * 70)
    print()

    # System info
    print("SYSTEM INFO:")
    print(f"  Python version: {sys.version}")
    print(f"  Platform: {sys.platform}")
    print(f"  sys.frozen: {getattr(sys, 'frozen', False)}")
    print(f"  sys._MEIPASS: {getattr(sys, '_MEIPASS', 'NOT SET')}")
    print(f"  sys.executable: {sys.executable}")
    print()

    # Check if running frozen
    is_frozen = getattr(sys, 'frozen', False)
    meipass = getattr(sys, '_MEIPASS', None)

    if not is_frozen:
        print("WARNING: Not running as frozen executable!")
        print("         Results will show local Python paths.")
        print()

    # Modules to test - watchdog and all its submodules
    modules_to_test = [
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
        modules_to_test.extend([
            'watchdog.observers.winapi',
            'watchdog.observers.read_directory_changes',
        ])

    print("MODULE IMPORT VERIFICATION:")
    print("-" * 70)

    results = {}
    all_passed = True
    any_from_site_packages = False

    for mod_name in modules_to_test:
        try:
            module = __import__(mod_name, fromlist=[''])
            source = get_module_source(module)
            from_bundle = is_from_frozen_bundle(source)
            from_site = is_from_site_packages(source)

            # Determine status
            if is_frozen:
                if from_bundle:
                    status = "OK (from bundle)"
                elif from_site:
                    status = "FAIL (from site-packages!)"
                    all_passed = False
                    any_from_site_packages = True
                elif source.startswith('<'):
                    status = "OK (built-in)"
                else:
                    status = "WARN (unknown location)"
                    all_passed = False
            else:
                # Not frozen - just report location
                status = "IMPORTED (not frozen)"

            results[mod_name] = {
                'success': True,
                'source': source,
                'status': status,
                'from_bundle': from_bundle,
                'from_site': from_site,
            }

            print(f"  [{status[:4]}] {mod_name}")
            print(f"         Path: {source}")

        except ImportError as e:
            results[mod_name] = {
                'success': False,
                'error': str(e),
                'status': 'FAIL (import error)',
            }
            all_passed = False
            print(f"  [FAIL] {mod_name}")
            print(f"         Error: {e}")

        print()

    # Test actual functionality
    print("-" * 70)
    print("FUNCTIONALITY TEST:")
    print("-" * 70)

    try:
        from watchdog.observers import Observer
        from watchdog.events import FileSystemEventHandler

        # Get the actual Observer class being used
        observer = Observer()
        observer_class = type(observer).__name__
        observer_module = type(observer).__module__

        print(f"  Observer class: {observer_class}")
        print(f"  Observer module: {observer_module}")

        # Check Observer source
        observer_mod = sys.modules.get(observer_module)
        if observer_mod:
            obs_source = get_module_source(observer_mod)
            obs_from_bundle = is_from_frozen_bundle(obs_source)
            obs_from_site = is_from_site_packages(obs_source)

            if is_frozen and obs_from_site:
                print(f"  [FAIL] Observer loaded from site-packages!")
                print(f"         Path: {obs_source}")
                all_passed = False
                any_from_site_packages = True
            elif is_frozen and obs_from_bundle:
                print(f"  [OK] Observer loaded from bundle")
                print(f"         Path: {obs_source}")
            else:
                print(f"  Observer path: {obs_source}")

        # Try to create a handler
        handler = FileSystemEventHandler()
        print(f"  [OK] Created FileSystemEventHandler instance")

    except Exception as e:
        print(f"  [FAIL] Functionality test failed: {e}")
        all_passed = False

    print()
    print("=" * 70)
    print("SUMMARY:")
    print("=" * 70)

    if not is_frozen:
        print("  STATUS: NOT FROZEN (test ran in interpreter)")
        print("  To properly test, build and run as frozen executable.")
        exit_code = 0
    elif all_passed:
        print("  STATUS: PASSED")
        print("  All watchdog modules are loading from the frozen bundle.")
        exit_code = 0
    elif any_from_site_packages:
        print("  STATUS: FAILED - MODULES FROM SITE-PACKAGES")
        print("  Some modules are being loaded from the local Python installation")
        print("  instead of the frozen bundle. This means the frozen executable")
        print("  will fail on systems without watchdog installed.")
        exit_code = 1
    else:
        print("  STATUS: FAILED - IMPORT ERRORS")
        print("  Some watchdog modules failed to import.")
        exit_code = 1

    print()
    print(f"Exit code: {exit_code}")

    # Force flush before exit
    sys.stdout.flush()
    sys.stderr.flush()

    # Use os._exit to ensure clean exit
    os._exit(exit_code)


if __name__ == '__main__':
    main()
