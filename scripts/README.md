# Environment Setup and Scripts

This directory contains PowerShell wrapper scripts to facilitate interoperability between the Windows environment and Unix-style tools (bash, git, ssh) available on the system.

## The Problem
We are working on Windows (PowerShell) but relying on Unix tools running in a bash environment. Authentication for git/ssh is handled by an `ssh-agent` running under bash. To use these tools from PowerShell, we need to locate the `SSH_AUTH_SOCK` and `SSH_AGENT_PID` and inject them into the `bash` execution environment.

## The Solution
We use wrapper scripts that:
1.  Scan `/tmp/` for `ssh-agent` sockets (`agent.<pid>`).
2.  Test each socket by running `ssh-add -l`.
3.  Upon finding a valid agent, execute the desired command within `bash` with the correct environment variables set.

## Scripts
### `bash-ssh.ps1`
**Usage:** `.\scripts\bash-ssh.ps1 <command>`
Generic wrapper. Finds the active SSH agent and runs the provided command in bash.

Example:
```powershell
.\scripts\bash-ssh.ps1 ssh-add -l
.\scripts\bash-ssh.ps1 ssh user@host
```

### `git-ssh.ps1`
**Usage:** `.\scripts\git-ssh.ps1 <git-args>`
Specific wrapper for git operations.

Example:
```powershell
.\scripts\git-ssh.ps1 clone git@github.com:user/repo.git
.\scripts\git-ssh.ps1 pull
```

## Current Environment Status
During the initial checks, the following values were discovered (example):
- **SSH_AUTH_SOCK**: `/tmp/ssh-ETp3GFxiZ4Lq/agent.1166`
- **SSH_AGENT_PID**: `1167`
