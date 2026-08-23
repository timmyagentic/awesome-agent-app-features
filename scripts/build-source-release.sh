#!/bin/sh

set -eu

if [ "$#" -ne 2 ]; then
  echo "usage: $0 <annotated-semver-tag> <output-directory>" >&2
  exit 2
fi

release_tag=$1
output_argument=$2

if ! printf '%s\n' "$release_tag" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "release tag must be exact vX.Y.Z SemVer" >&2
  exit 2
fi

repository_root=$(git rev-parse --show-toplevel)
cd "$repository_root"

if [ "$(git cat-file -t "refs/tags/$release_tag" 2>/dev/null || true)" != "tag" ]; then
  echo "release tag must exist and be annotated: $release_tag" >&2
  exit 2
fi
if [ "$(git rev-parse "$release_tag^{commit}")" != "$(git rev-parse HEAD)" ]; then
  echo "release tag must point at the checked-out commit" >&2
  exit 2
fi

mkdir -p "$output_argument"
output_directory=$(cd "$output_argument" && pwd)
version=${release_tag#v}
archive_name="awesome-agent-app-features-$version.tar.gz"
archive_path="$output_directory/$archive_name"
checksums_path="$output_directory/checksums.txt"

if [ -e "$archive_path" ] || [ -e "$checksums_path" ]; then
  echo "release output already exists" >&2
  exit 2
fi

archive_temporary=$(mktemp "$output_directory/.source-archive.XXXXXX")
checksums_temporary=$(mktemp "$output_directory/.checksums.XXXXXX")
cleanup() {
  rm -f -- "$archive_temporary" "$checksums_temporary"
}
trap cleanup EXIT HUP INT TERM

LC_ALL=C git archive \
  --format=tar \
  --prefix="awesome-agent-app-features-$version/" \
  "$release_tag^{commit}" | gzip -n -9 >"$archive_temporary"

mv "$archive_temporary" "$archive_path"
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$output_directory" && sha256sum "$archive_name") >"$checksums_temporary"
elif command -v shasum >/dev/null 2>&1; then
  (cd "$output_directory" && shasum -a 256 "$archive_name") >"$checksums_temporary"
else
  echo "sha256sum or shasum is required" >&2
  exit 2
fi
mv "$checksums_temporary" "$checksums_path"

trap - EXIT HUP INT TERM
