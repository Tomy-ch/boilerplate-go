#!/usr/bin/env sh
# Send the windows that have closed but never reached their feedback issue.
#
# This runs at SessionStart rather than SessionEnd, and the choice is deliberate. A window that
# has closed is complete and can be sent whenever; a session that is ending is the one moment the
# user wants the process gone, and a network call there hangs on the way out where nobody sees it
# fail. Deferring to the next start is also what the index already models — an entry with no
# feedback issue means "not sent yet", and the next start is the next chance. `/clear` closes a
# window and fires SessionStart in the same breath, so in practice a boundary sends almost at once.
#
# It sends only CLOSED windows. The one still open is incomplete, and half a window is worse than
# a late one.
#
# Everything here degrades to doing nothing. A missing runner, an unauthenticated `gh`, no network
# — each leaves the index untouched and the window pending, which is the state the next run reads.
# It must never block or fail a session, so it exits 0 unconditionally and runs
# the actual send in the background.
#
# Transcripts live under the user's home, which the tool-runner container cannot see; the path is
# therefore computed here by `transcripts_dir` and passed explicitly.

set -eu

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH='' cd -- "${SCRIPT_DIR}/../.." && pwd)
RUNNER="${REPO_ROOT}/scripts/node_modules/.bin/tsx"
ENTRY="${REPO_ROOT}/scripts/closed-loop/send"
LOCK_DIR="${REPO_ROOT}/tmp/closed-loop/send.lock"

# Only one send at a time, for the same reason marks.sh locks its pointer: the index is
# read-modify-write, so two runs both see every pending window and both create an issue for it.
# Observed, with the reading done locally: 2 windows became 21 issues.
#
# `mkdir` is the primitive because it is atomic on every filesystem this runs on and needs no
# `flock`, which macOS does not ship.
#
# The lock is released on the normal path rather than only from a trap, because the trap does not
# fire on the one path that matters. `--hook` starts this as `( ... & )`, and in that doubly
# backgrounded form the EXIT trap is never reached — measured: the same function leaks its lock
# under `( f & )` and releases it under `( f ) &`. A leak there is permanent and silent, since
# every later run finds the directory and returns without sending anything.
#
# A stale lock is therefore taken over rather than trusted. Something that has held it for longer
# than any real send could take is not running any more, whatever happened to it. That is the
# right way round: a missed send is retried at the next start, while a stale lock stops every one
# of them and says nothing.
#
# A second, live run does not wait: it exits, because the run already in flight will send the same
# windows. Queueing would only pile up duplicates of that same work behind a call that must not
# block a session.
LOCK_STALE_SEC=1800

# `find -mmin` rather than `stat`, whose flags differ between BSD and GNU.
lock_is_stale() {
  [ -d "${LOCK_DIR}" ] || return 1
  [ -n "$(find "${LOCK_DIR}" -maxdepth 0 -mmin "+$((LOCK_STALE_SEC / 60))" 2>/dev/null)" ]
}

acquire_lock() {
  mkdir -p "$(dirname -- "${LOCK_DIR}")" 2>/dev/null || return 1
  mkdir "${LOCK_DIR}" 2>/dev/null && return 0
  lock_is_stale || return 1
  rmdir "${LOCK_DIR}" 2>/dev/null || return 1
  mkdir "${LOCK_DIR}" 2>/dev/null
}

release_lock() {
  rmdir "${LOCK_DIR}" 2>/dev/null || :
}

locked_run() {
  acquire_lock || return 0
  trap 'release_lock' INT TERM
  # `set -e` の下で `run` が落ちると以降が走らないため、`if` で終了コードを受け止めてから
  # 解放する。失敗しても握ったままにしないのがここの要件。
  if run; then status=0; else status=$?; fi
  release_lock
  return "${status}"
}

usage() {
  cat <<'USAGE'
Usage:
  send.sh            Send closed, unsent windows (blocking; prints the result)
  send.sh --hook     Same, detached and silent (SessionStart)
  send.sh --dry-run  Print what would be sent, send nothing

Requires scripts/node_modules/.bin/tsx and an authenticated gh. Missing either is not an error:
the windows stay pending and the next run picks them up.
USAGE
}

# Claude Code names a project directory after the checkout path, with every character that cannot
# appear in the name replaced: `/Users/x/repo` -> `-Users-x-repo`, and `/Users/x/repo/.claude/wt`
# -> `-Users-x-repo--claude-wt`. Replacing only the separator silently misses any checkout whose
# path contains a dot, which is where a worktree parked under a dot-directory lives — and a missed
# directory reads as "no transcripts", not as an error. Resolving the name from this checkout
# rather than from a fixed one is what gives a worktree its own directory.
transcripts_dir() {
  printf '%s/.claude/projects/%s' "${HOME}" "$(printf '%s' "${REPO_ROOT}" | tr -c 'A-Za-z0-9-' '-')"
}

run() {
  dir=$(transcripts_dir)
  if [ -d "${dir}" ]; then
    "${RUNNER}" "${ENTRY}" --transcripts "${dir}" "$@"
  else
    "${RUNNER}" "${ENTRY}" "$@"
  fi
}

case "${1:-}" in
  -h | --help)
    usage
    ;;
  --hook)
    # Detached: SessionStart must not wait on the network. Failures are invisible by design —
    # the index keeps the window pending, and the next start tries again.
    [ -x "${RUNNER}" ] || exit 0
    command -v gh >/dev/null 2>&1 || exit 0
    (locked_run >/dev/null 2>&1 &) || :
    exit 0
    ;;
  --dry-run)
    [ -x "${RUNNER}" ] || { echo "tsx が見つかりません: ${RUNNER}" >&2; exit 1; }
    run --dry-run
    ;;
  '')
    [ -x "${RUNNER}" ] || { echo "tsx が見つかりません: ${RUNNER}" >&2; exit 1; }
    locked_run
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac
