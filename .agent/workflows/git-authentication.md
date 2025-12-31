---
description: How to perform authenticated Git operations (push, pull, clone)
---
The user's environment requires a specific wrapper script to access the SSH agent for Git authentication.

**Context:**
- Standard `git push/pull` commands will fail because they cannot see the SSH agent.
- A wrapper script `scripts/git-ssh.ps1` (which calls `scripts/bash-ssh.ps1`) correctly bridges the PowerShell environment to the WSL/Bash SSH agent.

**Instructions:**
When you need to perform any Git operation that interacts with the remote (push, pull, fetch, clone):
1.  Do NOT use the standard `git` command directly.
2.  Use the `.\scripts\git-ssh.ps1` wrapper script.
3.  Pass arguments exactly as you would to git.

**Examples:**
-   **Wrong:** `git push`
-   **Correct:** `.\scripts\git-ssh.ps1 push`

-   **Wrong:** `git pull origin master`
-   **Correct:** `.\scripts\git-ssh.ps1 pull origin master`

-   **Wrong:** `git fetch --all`
-   **Correct:** `.\scripts\git-ssh.ps1 fetch --all`

**Exceptions:**
- Local operations like `git status`, `git add`, `git commit`, `git diff` can still be run with standard `git` if preferred, but using the wrapper is also safe and consistent.
