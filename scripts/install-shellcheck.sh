#!/usr/bin/env bash
# Install the pinned shellcheck into .tools/ and print its path.
# The archive's SHA-256 is checked against the pins below, which match the
# digests GitHub publishes for the v0.11.0 release assets. Needs curl, tar and
# sha256sum, but not root, so it works in CI images and Jenkins docker agents.
set -euo pipefail

VERSION=v0.11.0
declare -A SHA256=(
	[x86_64]=b7af85e41cc99489dcc21d66c6d5f3685138f06d34651e6d34b42ec6d54fe6f6
	[aarch64]=68a8133197a50beb8803f8d42f9908d1af1c5540d4bb05fdfca8c1fa47decefc
)

root=$(cd "$(dirname "$0")/.." && pwd)
dest=$root/.tools/shellcheck-$VERSION
bin=$dest/shellcheck
if [[ -x $bin ]]; then
	echo "$bin"
	exit 0
fi

arch=$(uname -m)
case $arch in
amd64) arch=x86_64 ;;
arm64) arch=aarch64 ;;
esac
want=${SHA256[$arch]:-}
[[ -n $want ]] || { echo "install-shellcheck: unsupported architecture $arch" >&2; exit 1; }

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
archive=shellcheck-$VERSION.linux.$arch.tar.gz
curl -sSfL -o "$tmp/$archive" "https://github.com/koalaman/shellcheck/releases/download/$VERSION/$archive"
echo "$want  $tmp/$archive" | sha256sum -c --quiet - >&2 ||
	{ echo "install-shellcheck: checksum mismatch for $archive" >&2; exit 1; }
tar -xzf "$tmp/$archive" -C "$tmp"
mkdir -p "$dest"
install -m 0755 "$tmp/shellcheck-$VERSION/shellcheck" "$bin"
echo "$bin"
