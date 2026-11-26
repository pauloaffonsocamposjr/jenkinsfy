#!/usr/bin/env bash
# Tests for scripts/changelog.sh, run by `make changelog-check`.
set -euo pipefail

SCRIPT=$(cd "$(dirname "$0")" && pwd)/changelog.sh
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
export CHANGELOG=$tmp/CHANGELOG.md
fails=0

expect() { # expect DESCRIPTION EXPECTED ACTUAL
	if [[ $2 == "$3" ]]; then
		echo "  ok    $1"
	else
		echo "  FAIL  $1"; diff <(printf '%s\n' "$2") <(printf '%s\n' "$3") | sed 's/^/        /'
		fails=$((fails + 1))
	fi
}
expect_fail() { # expect_fail DESCRIPTION CMD...
	local d=$1; shift
	if "$@" >/dev/null 2>&1; then echo "  FAIL  $d (succeeded)"; fails=$((fails + 1)); else echo "  ok    $d"; fi
}

cat >"$CHANGELOG" <<'MD'
# Changelog

## [Unreleased]

### Added

- First feature.

[Unreleased]: https://github.com/o/r/compare/abc123...HEAD
MD

expect "check passes on a fresh changelog" "$CHANGELOG: ok" "$("$SCRIPT" check)"
expect_fail "notes for a missing version" "$SCRIPT" notes 0.1.0
expect_fail "cut rejects a non-semver version" "$SCRIPT" cut 1.0

RELEASE_DATE=2026-01-02 "$SCRIPT" cut v0.1.0 >/dev/null
expect "first cut" "# Changelog

## [Unreleased]

## [0.1.0] - 2026-01-02

### Added

- First feature.

[Unreleased]: https://github.com/o/r/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/o/r/releases/tag/v0.1.0" "$(cat "$CHANGELOG")"
expect "notes of the first release" "### Added

- First feature." "$("$SCRIPT" notes 0.1.0)"
expect_fail "cut with nothing unreleased" "$SCRIPT" cut 0.2.0
expect_fail "cut an existing version" "$SCRIPT" cut 0.1.0

sed -i 's/^## \[Unreleased\]$/## [Unreleased]\n\n### Fixed\n\n- A bug./' "$CHANGELOG"
RELEASE_DATE=2026-02-03 "$SCRIPT" cut 0.1.1-rc.1 >/dev/null
expect "second cut links to the previous release" "[Unreleased]: https://github.com/o/r/compare/v0.1.1-rc.1...HEAD
[0.1.1-rc.1]: https://github.com/o/r/compare/v0.1.0...v0.1.1-rc.1
[0.1.0]: https://github.com/o/r/releases/tag/v0.1.0" "$(grep '^\[' "$CHANGELOG")"
expect "notes stop at the next release" "### Fixed

- A bug." "$("$SCRIPT" notes v0.1.1-rc.1)"
expect "check passes after releases" "$CHANGELOG: ok" "$("$SCRIPT" check)"

sed -i 's/^## \[0.1.0\] - 2026-01-02$/## [0.1.0]/' "$CHANGELOG"
expect_fail "check rejects a heading without a date" "$SCRIPT" check

[[ $fails == 0 ]] || { echo "changelog tests: $fails failed"; exit 1; }
echo "changelog tests: all passed"
