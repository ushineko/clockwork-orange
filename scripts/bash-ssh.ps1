<#
.SYNOPSIS
    Runs a command in bash environment with the active ssh-agent connected.
.DESCRIPTION
    Discovers the active ssh-agent socket in /tmp/ssh-*, verifies it has identities,
    and then executes the provided command within that environment.
.EXAMPLE
    .\scripts\bash-ssh.ps1 git pull
#>

$ErrorActionPreference = "Stop"

function Get-SSHAgent {
    $sockets = bash -c "find /tmp/ssh-* -name 'agent.*' -type s 2>/dev/null"
    if ($null -eq $sockets) { return $null }
    
    $sockets = $sockets -split "`n" | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
    $agentWithKeys = $null
    $emptyAgent = $null

    foreach ($sock in $sockets) {
        bash -c "SSH_AUTH_SOCK='$sock' ssh-add -l > /dev/null 2>&1"
        $exitCode = $LASTEXITCODE
        
        # Calculate PID (best effort guessing PID = socket_suffix + 1, though not always reliability needed if we just pass SOCK)
        $filename = Split-Path $sock -Leaf
        $socket_pid = $filename.Split('.')[-1]
        try {
            $pid_val = [int]$socket_pid + 1
        }
        catch {
            $pid_val = 0
        }
        $agentInfo = @{ Socket = $sock; Pid = $pid_val; HasKeys = ($exitCode -eq 0) }

        if ($exitCode -eq 0) {
            # Found agent with keys, this is preferred
            return $agentInfo
        }
        elseif ($exitCode -eq 1) {
            # Found working agent but empty
            if ($null -eq $emptyAgent) {
                $emptyAgent = $agentInfo
            }
        }
    }
    
    # Return empty agent if that's all we found
    return $emptyAgent
}

function Start-SSHAgent {
    Write-Host "No active ssh-agent found. Starting a new one..." -ForegroundColor Cyan
    $output = bash -c "ssh-agent -s"
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to start ssh-agent."
    }
    
    # Output looks like:
    # SSH_AUTH_SOCK=/tmp/ssh-Cw.../agent.123; export SSH_AUTH_SOCK;
    # SSH_AGENT_PID=124; export SSH_AGENT_PID;
    # echo Agent pid 124;
    
    $matchSock = $output | Select-String "SSH_AUTH_SOCK=([^;]+)"
    $matchPid = $output | Select-String "SSH_AGENT_PID=([^;]+)"
    
    if ($matchSock -and $matchPid) {
        return @{
            Socket  = $matchSock.Matches.Groups[1].Value
            Pid     = $matchPid.Matches.Groups[1].Value
            HasKeys = $false
        }
    }
    throw "Could not parse ssh-agent output."
}

$agent = Get-SSHAgent

if ($null -eq $agent) {
    try {
        $agent = Start-SSHAgent
    }
    catch {
        Write-Error $_.Exception.Message
        exit 1
    }
}

$env_str = "SSH_AUTH_SOCK=$($agent.Socket) SSH_AGENT_PID=$($agent.Pid)"
# Write-Host "Using Agent: $($agent.Socket)"

$command = $args -join " "
if ([string]::IsNullOrWhiteSpace($command)) {
    Write-Error "Please provide a command to run."
    exit 1
}

# Check if we need keys and if we have them
if (-not $agent.HasKeys) {
    # Check if the command is trying to add keys (ssh-add)
    # Simple check: does the command start with "ssh-add" or contains " ssh-add"
    $isAddCommand = ($args[0] -eq "ssh-add")
    
    if (-not $isAddCommand) {
        Write-Error "The active ssh-agent ($($agent.Socket)) has no identities."
        Write-Error "Please run the following command first to add your SSH key:"
        Write-Error "    .\scripts\bash-ssh.ps1 ssh-add <path-to-key>"
        exit 1
    }
}

# Escape double quotes in the command for the bash string
$escaped_command = $command -replace '"', '\"'

$bash_cmd = "$env_str $escaped_command"

# Run it
bash -c $bash_cmd
