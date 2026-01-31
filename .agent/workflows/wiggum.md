---
description: Iteratively run, capture logs, and debug a program until a success condition is met.
---

1.  **Preparation**:
    *   **Identify Goal**: What is the **Success Condition**? (e.g., "Battery level 70% shows in logs", "Error X is gone").
    *   **Ensure Observability**:
        *   Does the program output logs to stdout/stderr?
        *   Does it support a `--debug` flag? Use it.
        *   **CRITICAL**: If the program is NOT amenable to log capturing (silent, no debug mode), you **MUST** modify the code first to add print/logging statements. Do not skip this.
    *   **System Python**: Always use the system python (`/usr/bin/python3`) for running scripts and tests, to avoid environment mismatches.
    *   **Artifact Restriction**: Do **NOT** generate artifacts (especially images, e.g., "computer monitor with text") unless explicitly requested by the user. Focus on code and logs.

2.  **Clean Slate**:
    *   **Kill Existing Instances**: Before every run, ensure to kill any background instances or GUI processes of the target tool (e.g., `pkill -f my_script.py`) to ensure you are capturing the *new* run and not an old one.

3.  **Execution Loop**:
    *   **Run**: Execute the program.
        *   If it's interactive or long-running, use `timeout` or run in background with redirection: `python3 script.py --debug > debug.log 2>&1`.
    *   **Capture**: Read the output (stdout or log file).
    *   **Analyze**:
        *   Did the **Success Condition** occur?
        *   Are there new errors?
    *   **Decision**:
        *   **Success**: If yes, Proceed to **Finalization**.
            *   Be sure to remove all temporary logs and files created by this process so you don't clutter up the project.
        *   **Failure**:
            *   Analyze the logs to understand *why*.
            *   **Adjust**: Apply a fix to the code OR add *more* specific logging if the root cause is still hidden.
            *   **Loop**: Go back to **Clean Slate** and repeat.

4.  **Finalization**:
    *   **Update Sub-project Docs & Version** (Reference: `/update_docs_and_version`):
        *   **Identify Sub-Project**: Locate main source and README.
        *   **Bump Version**: Increment version in source code and README.
        *   **Update README**: Add new features/fixes to the sub-project's README.
        *   **Update Inline Help** (If applicable): Ensure any internal help text, "About" dialog content (e.g., see `audio-source-switcher`), or `--help` output reflects new changes and the updated version.
    *   **Ensure Uninstaller** (Reference: `/uninstaller`):
        *   Check if `uninstall.sh` exists.
        *   If not, create one that removes desktop files, config entries, etc.
        *   If yes, update it to include any new artifacts.
    *   **Update Global Documentation** (Reference: `/update_documentation`):
        *   Identify all scripts in the repo.
        *   Update the root `README.md` with a table/list of all scripts and their descriptions.
        *   **About/Help Dialog**: If the app has an About/Help dialog that displays the README, ensure it stays in sync with documentation changes.
    *   **Update AUR** (If applicable):
        *   If this project has an AUR package:
            1.  **Update pkgver**: First, update the `pkgver` line in PKGBUILD to the current version:
                ```bash
                echo "$(cat .tag | tr -d 'v').r$(git rev-list --count HEAD).g$(git rev-parse --short HEAD)"
                ```
            2.  **Run update script**: `./scripts/update_aur.sh "Update to vX.Y.Z"`.
        *   This copies PKGBUILD, generates `.SRCINFO`, and pushes to AUR.
    *   **Notify User**: Confirm success and documentation updates.