#!/usr/bin/env sh
# Report the checkout's environment state to a session that is just starting: which checkout
# it is, whether the vendor tree exists, and whether a DB slot is held.
#
# It must never run `make slot-acquire`, `slot-free`, `slot-release`, or a database
# reinitialization. Acquiring a slot recreates that slot's databases, and this runs on every
# startup, resume and compaction. ADR-0009 draws the boundary.
#
# Two modes. With no arguments it prints the status line for a human. With --hook it emits the
# SessionStart additionalContext envelope; the payload on stdin is not read, because every
# input is a property of the checkout the script itself lives in.
#
# It always exits 0. A session must start whether or not this check can answer.

set -eu

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH='' cd -- "${SCRIPT_DIR}/../.." && pwd)
SLOT_FILE="${REPO_ROOT}/.gobp-db-slot"

usage() {
  cat <<'USAGE'
Usage:
  session-start-env.sh          Print the environment status line
  session-start-env.sh --hook   Emit the SessionStart hook JSON

Reports the checkout kind, the vendor tree, and the DB slot. Never changes any of them.
USAGE
}

# A linked worktree's git dir sits under the main checkout's; the main checkout's two are the
# same path. The answer also gates the slot notice: without a slot the make targets fall back
# to the shared `local` / `test` databases, which is the main checkout's normal mode and a
# collision between tenants in a worktree.
checkout_kind() {
  git_dir=$(git -C "${REPO_ROOT}" rev-parse --absolute-git-dir 2>/dev/null) || {
    printf 'unknown'
    return
  }
  common_dir=$(git -C "${REPO_ROOT}" rev-parse --path-format=absolute --git-common-dir 2>/dev/null) || {
    printf 'unknown'
    return
  }

  if [ "${git_dir}" = "${common_dir}" ]; then
    printf 'main'
  else
    printf 'worktree'
  fi
}

current_branch() {
  git -C "${REPO_ROOT}" rev-parse --abbrev-ref HEAD 2>/dev/null || printf 'unknown'
}

# Read one key out of the slot file without sourcing it: this runs unattended at every session
# start, so it reads the value rather than executing the line that holds it.
slot_value() {
  sed -n "s/^${1}=//p" "${SLOT_FILE}" 2>/dev/null | tail -n 1 | tr -d "\"'" || true
}

kind=$(checkout_kind)
branch=$(current_branch)

if [ -d "${REPO_ROOT}/vendor" ]; then
  vendor='present'
else
  vendor='missing'
fi

# The slot is held only when the file names one. Treating a half-written lease as held would
# report a slot no make target can resolve, and silence the notice below in the one case that
# most needs it.
slot_id=''
[ ! -f "${SLOT_FILE}" ] || slot_id=$(slot_value SLOT)

if [ -n "${slot_id}" ]; then
  slot="wt${slot_id}"
  api_port=$(slot_value API_HOST_PORT)
  [ -z "${api_port}" ] || slot="${slot} api:${api_port}"
elif [ -f "${SLOT_FILE}" ]; then
  slot='invalid'
else
  slot='none'
fi

status="[agent-env] checkout=${kind} branch=${branch} vendor=${vendor} db-slot=${slot}"

notices=''
add_notice() {
  notices="${notices}
- $1"
}

# Notify only: generating the tree here would rewrite files the session has not yet looked at.
# air builds with `--mod=vendor`, so the failure this warns about is `make serve` reporting
# success and then dying with `inconsistent vendoring`.
if [ "${vendor}" = 'missing' ]; then
  # shellcheck disable=SC2016 # バッククォートは Markdown のコードスパン。展開させない
  add_notice 'vendor/ がありません。`make serve` の air は `--mod=vendor` 固定のため、起動前に `go mod vendor` が必要です（このフックは実行しません）。'
fi

if [ -z "${slot_id}" ] && [ "${kind}" = 'worktree' ]; then
  # shellcheck disable=SC2016 # 同上
  add_notice 'DB スロットは未取得です。DB を使う作業（`make go-test` / `make db-init` / `make serve`）を始める直前に `make slot-acquire` を実行してください。スロット取得は DB を作り直すため、このフックは実行しません。'
fi

emit_hook() {
  # Without jq there is no way to build the envelope safely. Staying silent is the right
  # failure: the alternative is a malformed payload or an error notice on every session start.
  command -v jq >/dev/null 2>&1 || exit 0

  if [ -n "${notices}" ]; then
    context="${status}
確認が必要な点:${notices}"
  else
    context="${status}"
  fi

  jq -n --arg context "${context}" '{
    hookSpecificOutput: {
      hookEventName: "SessionStart",
      additionalContext: $context
    }
  }'
}

case "${1:-}" in
  --hook)
    emit_hook
    ;;
  -h | --help)
    usage
    ;;
  '')
    printf '%s\n' "${status}"
    [ -z "${notices}" ] || printf '確認が必要な点:%s\n' "${notices}"
    ;;
  *)
    usage
    exit 0
    ;;
esac
