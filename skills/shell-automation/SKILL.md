---
name: shell-automation
description: Cross-platform shell and PowerShell execution guide for Norma's Bash tool and CORTEX Custom Tools across Linux/Docker (bash) and Windows (PowerShell).
license: MIT
---

# Cross-Platform Shell & PowerShell Automation Skill

Norma's `Bash` tool automatically selects the host shell based on the operating system (`bash -c` on Linux/macOS/Docker, and `powershell -NoProfile -NonInteractive -Command` on native Windows). Check `# hostEnvironment` in `<system-reminder>` and follow the matching rules below:

## 1. When Host OS is Windows (`powershell`)
- **No POSIX Heredocs**: PowerShell does NOT support `cat << 'EOF'` or `python - << 'PY'`. Use single-quoted PowerShell here-strings (`@'\n...\n'@ | Set-Content file.txt`) or `python -c "..."`.
- **Statement Chaining**: Use `;` instead of `&&` before PowerShell built-ins (`if`, `foreach`, `$var = ...`, `Set-Location`).
- **Prefer Native Cmdlets or Python**: Use `Get-ChildItem`, `Select-String`, `Get-Content`, `Invoke-RestMethod`, or invoke `python -c` for portable data processing.

## 2. When Host OS is Linux / macOS / Docker (`bash`)
- **Non-Interactive Flags**: Always pass non-interactive flags (`--no-pager`, `-y`, `-f`) so commands never hang waiting for stdin.
- **Script Execution**: For complex multi-step logic, prefer `python3 -c "..."` or writing a temporary script file over deeply nested quoting.
