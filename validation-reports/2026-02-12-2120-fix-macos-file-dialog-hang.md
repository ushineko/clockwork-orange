## Validation Report: Fix macOS File Dialog Hang
**Date**: 2026-02-12 21:20
**Commit**: (pending)
**Status**: PASSED

### Phase 3: Tests
- Self-test: All 12 checks passed (non-frozen)
- No import breakage from changes
- Status: PASSED

### Phase 4: Code Quality
- Dead code: None
- Duplication: Single helper method `_file_dialog_options()` used by both dialog calls
- Encapsulation: Platform check isolated in one method
- Status: PASSED

### Phase 5: Security Review
- No new attack surface; change only affects dialog presentation
- Status: PASSED

### Root Cause
On macOS, `QFileDialog.getExistingDirectory()` and `getOpenFileName()` use the native `NSOpenPanel` by default. When the Python process is not a registered macOS GUI application (running from source, not from a `.app` bundle), `NSOpenPanel` can fail to acquire foreground focus. The dialog opens behind the main window, invisible to the user. Since it's modal, the main thread blocks waiting for user input that can never arrive, spinning CPU indefinitely.

### Fix
Use `QFileDialog.Option.DontUseNativeDialog` on macOS (`sys.platform == "darwin"`) to use Qt's built-in cross-platform dialog instead. This avoids the native `NSOpenPanel` focus issue entirely. On Windows and Linux, the native dialogs continue to be used.

### Files Changed
| File | Change | Description |
|------|--------|-------------|
| gui/plugins_tab.py | Modified | Add `_file_dialog_options()` helper; pass options to `getOpenFileName` and `getExistingDirectory` |

### Overall
- All gates passed: YES
