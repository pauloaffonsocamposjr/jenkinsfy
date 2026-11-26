#!/usr/bin/env bash
# Prepare a release: move the Unreleased changelog entries into VERSION,
# commit, and create an annotated tag. Nothing is pushed.
#
#   scripts/release.sh 0.2.0
#
# Then push the commit and the tag; the CI release job builds the binaries
# and publishes a GitHub release with the changelog section as its notes.
set -euo pipefail

cd "$(dirname "$0")/.."
v=${1:?usage: scripts/release.sh X.Y.Z[-pre]}
v=${v#v}
tag="v$v"

[[ -z $(git status --porcelain) ]] || { echo "release: working tree is not clean" >&2; exit 1; }
git rev-parse -q --verify "refs/tags/$tag" >/dev/null && { echo "release: tag $tag already exists" >&2; exit 1; }
branch=$(git branch --show-current)
if [[ $branch != main ]]; then
	echo "release: note: releasing from '$branch', not 'main'" >&2
fi

scripts/changelog.sh cut "$v"
scripts/changelog.sh check >/dev/null
git add CHANGELOG.md
git commit -q -m "Release $tag"
# The first line is the tag's subject, which GitHub shows as its title.
# Verbatim cleanup keeps the notes' "### Added" headings, which git would
# otherwise strip as comments.
{
	echo "jenkinsfy $tag"
	echo
	scripts/changelog.sh notes "$v"
} | git tag -a "$tag" --cleanup=verbatim -F -

echo "Created commit and tag $tag. Publish with:"
echo "  git push origin $branch $tag"
