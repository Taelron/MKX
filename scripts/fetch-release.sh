#!/usr/bin/env bash
#
# Downloads a published mkx release archive, verifies its checksum, and leaves
# the extracted binary in a directory the caller names.
#
# It does not install anything. `make install` owns where the binary goes and
# owns the PATH warnings; this script owns "get a trustworthy binary onto this
# machine", which is the part with a network in it.
#
# Usage: scripts/fetch-release.sh <version|latest> <outdir>
#
# The checksum step is not decoration. This writes a binary that a later step
# puts on PATH, so an archive that arrives corrupted or substituted must fail
# loudly here rather than become the thing the user runs. goreleaser publishes
# checksums.txt beside the archives for exactly this.

set -euo pipefail

VERSION="${1:?usage: fetch-release.sh <version|latest> <outdir>}"
OUTDIR="${2:?usage: fetch-release.sh <version|latest> <outdir>}"
REPO="${REPO:-Taelron/MKX}"
BINARY="${BINARY:-mkx}"

# goreleaser names archives {project}_{Os}_{Arch}. uname's spelling is not
# goreleaser's, so both axes are mapped rather than passed through — and an
# unmapped value is an error, not a guess at a URL that will 404 later with a
# less useful message.
os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
linux | darwin) ;;
*)
	echo "fetch-release: no published build for OS '$os' — build from source with 'make install-source'" >&2
	exit 1
	;;
esac

case "$(uname -m)" in
x86_64 | amd64) arch="amd64" ;;
aarch64 | arm64) arch="arm64" ;;
*)
	echo "fetch-release: no published build for architecture '$(uname -m)' — build from source with 'make install-source'" >&2
	exit 1
	;;
esac

archive="${BINARY}_${os}_${arch}.tar.gz"

if [ "$VERSION" = "latest" ]; then
	base="https://github.com/$REPO/releases/latest/download"
else
	base="https://github.com/$REPO/releases/download/$VERSION"
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

echo "fetching $archive from $REPO ($VERSION)" >&2

# --fail so an HTML 404 page is an error rather than a file that fails to
# extract two steps later; -L because the download endpoint redirects.
for f in "$archive" checksums.txt; do
	if ! curl -fsSL -o "$work/$f" "$base/$f"; then
		echo "fetch-release: could not download $f from $base" >&2
		echo "              check that release '$VERSION' exists and publishes $archive" >&2
		exit 1
	fi
done

# --ignore-missing: checksums.txt covers every platform's archive, and only one
# of them is here. Without it sha256sum fails on the archives it cannot find and
# the real result is lost in the noise.
(cd "$work" && sha256sum --ignore-missing --check --status checksums.txt) || {
	echo "fetch-release: checksum verification FAILED for $archive" >&2
	echo "              the download does not match the published checksum; not installing it" >&2
	exit 1
}
echo "checksum verified" >&2

tar -C "$work" -xzf "$work/$archive"
if [ ! -f "$work/$BINARY" ]; then
	echo "fetch-release: $archive did not contain a '$BINARY' binary" >&2
	exit 1
fi

mkdir -p "$OUTDIR"
mv "$work/$BINARY" "$OUTDIR/$BINARY"
chmod 0755 "$OUTDIR/$BINARY"

# Whether the release is behind the checkout the user is standing in.
#
# This is the case that made the target worth guarding: a repository can sit
# many merged commits ahead of its newest tag, and then installing "the latest
# release" hands back something older than the source in the current directory.
# It is a legitimate thing to want; it is never a legitimate thing to get by
# accident, because a binary on PATH does not say how old it is.
if command -v git >/dev/null 2>&1 && git rev-parse --git-dir >/dev/null 2>&1; then
	tag="$VERSION"
	if [ "$tag" = "latest" ]; then
		tag="$(git tag --sort=-creatordate | head -1 || true)"
	fi
	if [ -n "$tag" ] && git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
		if ! git merge-base --is-ancestor HEAD "refs/tags/$tag" 2>/dev/null; then
			behind="$(git rev-list --count "refs/tags/$tag"..HEAD 2>/dev/null || echo "?")"
			echo >&2
			echo "warning: this checkout is $behind commits ahead of $tag." >&2
			echo "         You are installing a binary OLDER than the source you are standing in." >&2
			echo "         'make install-source' builds and installs this checkout instead." >&2
			echo >&2
		fi
	fi
fi
