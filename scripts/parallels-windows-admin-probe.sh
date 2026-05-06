#!/usr/bin/env bash
set -euo pipefail

vm_name="${1:-Windows 11}"
vm_user="${PRLCTL_VM_USER:-testing}"
if [[ -z "${PRLCTL_VM_PASSWORD:-}" ]]; then
  echo "PRLCTL_VM_PASSWORD is required; export it in your shell or inject it from a secret manager." >&2
  exit 2
fi

ps_script='\
$p = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
"who=" + [Security.Principal.WindowsIdentity]::GetCurrent().Name
"admin=" + $p.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
whoami /groups | Select-String -Pattern "Administrators|Mandatory" | ForEach-Object { $_.Line.Trim() }
'
encoded=$(printf '%s' "$ps_script" | iconv -f UTF-8 -t UTF-16LE | base64 | tr -d '\n')

prlctl exec "$vm_name" \
  --user "$vm_user" \
  --password "$PRLCTL_VM_PASSWORD" \
  powershell.exe -NoProfile -ExecutionPolicy Bypass -EncodedCommand "$encoded"
