
try:
    from PyQt6.QtWidgets import QApplication
    from PyQt6.QtCore import QTimer
    import sys
    print("PyQt6 module found.")
except ImportError as e:
    print(f"Failed to import PyQt6: {e}")
    sys.exit(1)

try:
    # Attempt to initialize QApplication to check for DLL/Linker errors
    # Pass 'headless' or minimal args
    app = QApplication(sys.argv)
    print("QApplication initialized successfully.")
    
    # Just to be sure, maybe schedule a quit
    QTimer.singleShot(0, app.quit)
    sys.exit(0)
except Exception as e:
    print(f"Failed to initialize QApplication: {e}")
    sys.exit(1)
