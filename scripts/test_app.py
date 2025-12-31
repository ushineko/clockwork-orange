import sys

from PyQt6.QtCore import Qt, QTimer
from PyQt6.QtWidgets import QApplication, QLabel, QMainWindow


def main():
    print("[TEST] Starting Test App...")

    try:
        app = QApplication(sys.argv)
        print("[TEST] QApplication initialized.")

        window = QMainWindow()
        window.setWindowTitle("PyInstaller Test")
        label = QLabel("Hello from PyInstaller!", window)
        label.setAlignment(Qt.AlignmentFlag.AlignCenter)
        window.setCentralWidget(label)
        window.resize(400, 300)

        window.show()
        print("[TEST] Window shown.")

        # Auto-close after 2 seconds to verify it runs without hanging
        print("[TEST] Scheduling auto-close in 2 seconds...")
        QTimer.singleShot(2000, app.quit)

        exit_code = app.exec()
        print(f"[TEST] Exiting with code {exit_code}")
        sys.exit(exit_code)

    except Exception as e:
        print(f"[ERROR] Exception occurred: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
