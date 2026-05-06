#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: uninstall.sh [options]

Options:
  --install-dir DIR    Install directory (default: $HOME/.local/bin)
  --remove-state       Delete Loki local state after typed confirmation
  --remove-store PATH  Delete explicit Loki store path after typed confirmation
  --force              Skip typed confirmations for destructive flags
  -h, --help           Show help
USAGE
}

die() {
  echo "uninstall.sh: $*" >&2
  exit 1
}

install_dir="${LOKI_INSTALL_DIR:-$HOME/.local/bin}"
remove_state=0
remove_store=""
force=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --install-dir)
      [[ $# -ge 2 ]] || die "--install-dir requires a value"
      install_dir="$2"
      shift 2
      ;;
    --remove-state)
      remove_state=1
      shift
      ;;
    --remove-store)
      [[ $# -ge 2 ]] || die "--remove-store requires a value"
      remove_store="$2"
      shift 2
      ;;
    --force)
      force=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
done

resolve_path() {
  local path="$1"
  [[ -n "$path" ]] || return 0
  if [[ "$path" = /* ]]; then
    printf '%s\n' "$path"
  else
    printf '%s\n' "$(pwd)/$path"
  fi
}

state_dir() {
  case "$(uname -s)" in
    Darwin) printf '%s\n' "$HOME/Library/Application Support/loki-profile-manager" ;;
    Linux) printf '%s\n' "${XDG_STATE_HOME:-$HOME/.local/state}/loki-profile-manager" ;;
    *) die "unsupported OS: $(uname -s)" ;;
  esac
}

confirm_delete() {
  local phrase="$1" target="$2" answer
  [[ "$force" -eq 1 ]] && return 0
  printf 'Type %s to delete %s: ' "$phrase" "$target" >&2
  IFS= read -r answer
  [[ "$answer" == "$phrase" ]] || die "confirmation failed for $target"
}

install_dir=$(resolve_path "$install_dir")
[[ -z "$remove_store" ]] || remove_store=$(resolve_path "$remove_store")

echo "== uninstall Loki =="
echo "Install dir: $install_dir"

rm -f "$install_dir/loki" "$install_dir/.loki-install.json"

if [[ "$remove_state" -eq 1 ]]; then
  local_state=$(state_dir)
  confirm_delete "DELETE LOKI STATE" "$local_state"
  rm -rf "$local_state"
  echo "Removed local state: $local_state"
else
  echo "Preserved local state."
fi

if [[ -n "$remove_store" ]]; then
  confirm_delete "DELETE LOKI STORE" "$remove_store"
  rm -rf "$remove_store"
  echo "Removed store: $remove_store"
else
  echo "Preserved synced stores and managed targets."
fi

echo "Loki uninstalled"
