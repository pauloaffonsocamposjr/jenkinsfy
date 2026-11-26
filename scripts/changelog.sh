#!/usr/bin/env bash
# Changelog helpers for CHANGELOG.md (https://keepachangelog.com/en/1.1.0/).
#
#   scripts/changelog.sh check            validate the changelog structure
#   scripts/changelog.sh notes VERSION    print the entries of one release
#   scripts/changelog.sh cut VERSION      move Unreleased entries into VERSION
#
# VERSION is a semantic version with or without the leading "v".
set -euo pipefail

FILE=${CHANGELOG:-CHANGELOG.md}
SEMVER='^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$'

die() { echo "changelog: $*" >&2; exit 1; }

normalize() { # strip a leading v and validate
	local v=${1#v}
	[[ $v =~ $SEMVER ]] || die "'$1' is not a semantic version (X.Y.Z[-pre])"
	printf '%s' "$v"
}

# section VERSION|Unreleased: print the body of a "## [name]" section.
section() {
	awk -v name="$1" '
		/^## \[/ { inside = (index($0, "## [" name "]") == 1); next }
		/^\[[^]]+\]: / { inside = 0 }
		inside { print }
	' "$FILE" | sed -e '/./,$!d' | sed -e ':a' -e '/^\n*$/{$d;N;ba' -e '}'
}

cmd_check() {
	[[ -f $FILE ]] || die "$FILE not found"
	grep -q '^## \[Unreleased\]$' "$FILE" || die "missing '## [Unreleased]' heading"
	local bad
	bad=$(grep '^## \[' "$FILE" | grep -v '^## \[Unreleased\]$' |
		grep -Ev '^## \[[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?\] - [0-9]{4}-[0-9]{2}-[0-9]{2}$' || true)
	[[ -z $bad ]] || die "malformed release headings (want '## [X.Y.Z] - YYYY-MM-DD'):"$'\n'"$bad"
	local dups
	dups=$(grep -o '^## \[[^]]*\]' "$FILE" | sort | uniq -d)
	[[ -z $dups ]] || die "duplicate headings: $dups"
	echo "$FILE: ok"
}

cmd_notes() {
	local v
	v=$(normalize "${1:?usage: notes VERSION}")
	grep -q "^## \[$v\]" "$FILE" || die "no '## [$v]' section in $FILE"
	local body
	body=$(section "$v")
	[[ -n $body ]] || die "section $v is empty"
	printf '%s\n' "$body"
}

cmd_cut() {
	local v date repo prev
	v=$(normalize "${1:?usage: cut VERSION}")
	date=${RELEASE_DATE:-$(date -u +%F)}
	grep -q "^## \[$v\]" "$FILE" && die "version $v already exists"
	[[ -n $(section Unreleased) ]] || die "nothing under [Unreleased] to release"
	repo=$(sed -n 's|^\[Unreleased\]: \(https://github.com/[^/]*/[^/]*\)/compare/.*|\1|p' "$FILE")
	[[ -n $repo ]] || die "missing '[Unreleased]: https://github.com/OWNER/REPO/compare/...' link"
	prev=$(grep -m1 -oE '^## \[[0-9][^]]*\]' "$FILE" | sed 's/^## \[\(.*\)\]$/\1/' || true)

	awk -v v="$v" -v date="$date" -v repo="$repo" -v prev="$prev" '
		$0 == "## [Unreleased]" { print; print ""; print "## [" v "] - " date; next }
		/^\[Unreleased\]: / {
			print "[Unreleased]: " repo "/compare/v" v "...HEAD"
			if (prev != "") print "[" v "]: " repo "/compare/v" prev "...v" v
			else print "[" v "]: " repo "/releases/tag/v" v
			next
		}
		{ print }
	' "$FILE" >"$FILE.tmp" && mv "$FILE.tmp" "$FILE"
	echo "$FILE: released $v ($date)"
}

case ${1:-} in
check) cmd_check ;;
notes) cmd_notes "${2:-}" ;;
cut) cmd_cut "${2:-}" ;;
*) sed -n '2,8p' "$0"; exit 1 ;;
esac
