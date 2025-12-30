<#
.SYNOPSIS
    Runs git in bash environment with the active ssh-agent.
.DESCRIPTION
    Wrapper around bash-ssh.ps1 to execute git commands.
    Useful for git operations that require SSH keys (clone, push, pull).
.EXAMPLE
    .\scripts\git-ssh.ps1 pull origin master
#>

$ScriptDir = Split-Path $MyInvocation.MyCommand.Path
# Pass all arguments to the bash-ssh wrapper prepended with 'git'
& "$ScriptDir\bash-ssh.ps1" git @args
