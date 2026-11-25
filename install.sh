#!/usr/bin/env bash
set -euo pipefail

repository="Max-0e/lazykcp"

fail() {
  printf 'lazykcp installer: %s\n' "$*" >&2
  exit 1
}

for command_name in curl tar awk; do
  command -v "$command_name" >/dev/null 2>&1 || fail "required command not found: $command_name"
done

system=$(uname -s)
case "$system" in
  Linux) os="linux" ;;
  Darwin) os="darwin" ;;
  *) fail "unsupported operating system: $system (supported: Linux and macOS)" ;;
esac

machine=$(uname -m)
case "$machine" in
  x86_64 | amd64) arch="amd64" ;;
  aarch64 | arm64) arch="arm64" ;;
  *) fail "unsupported architecture: $machine (supported: amd64 and arm64)" ;;
esac

if command -v sha256sum >/dev/null 2>&1; then
  sha256() {
    sha256sum "$1" | awk '{ print $1 }'
  }
elif command -v shasum >/dev/null 2>&1; then
  sha256() {
    shasum -a 256 "$1" | awk '{ print $1 }'
  }
else
  fail "required command not found: sha256sum or shasum"
fi

archive="lazykcp_${os}_${arch}.tar.gz"
version=${LAZYKCP_VERSION:-}
if [[ -n "$version" ]]; then
  if [[ ! "$version" =~ ^v?[0-9A-Za-z][0-9A-Za-z._-]*$ ]]; then
    fail "invalid LAZYKCP_VERSION: $version"
  fi
  version=${version#v}
  release_url="https://github.com/${repository}/releases/download/v${version}"
else
  release_url="https://github.com/${repository}/releases/latest/download"
fi

install_dir=${INSTALL_DIR:-}
if [[ -z "$install_dir" ]]; then
  [[ -n "${HOME:-}" ]] || fail "HOME is not set; set INSTALL_DIR to choose an install directory"
  install_dir="$HOME/.local/bin"
fi

temporary_dir=$(mktemp -d) || fail "could not create a temporary directory"
trap 'rm -rf "$temporary_dir"' EXIT

archive_path="$temporary_dir/$archive"
checksums_path="$temporary_dir/checksums.txt"
curl --fail --location --silent --show-error "$release_url/$archive" --output "$archive_path" ||
  fail "could not download $archive from $release_url"
curl --fail --location --silent --show-error "$release_url/checksums.txt" --output "$checksums_path" ||
  fail "could not download release checksums"

expected_hash=$(awk -v file="$archive" '$2 == file || $2 == ("*" file) { print $1; exit }' "$checksums_path")
[[ -n "$expected_hash" ]] || fail "checksum for $archive was not found in the release"
actual_hash=$(sha256 "$archive_path")
[[ "$actual_hash" == "$expected_hash" ]] || fail "checksum verification failed for $archive"

tar -xzf "$archive_path" -C "$temporary_dir" lazykcp ||
  fail "could not extract lazykcp from $archive"
[[ -x "$temporary_dir/lazykcp" ]] || fail "release archive does not contain an executable lazykcp binary"

mkdir -p "$install_dir" || fail "could not create install directory: $install_dir"
cp "$temporary_dir/lazykcp" "$install_dir/lazykcp" || fail "could not install to $install_dir"
chmod 0755 "$install_dir/lazykcp" || fail "could not make $install_dir/lazykcp executable"

printf 'Installed lazykcp to %s/lazykcp\n' "$install_dir"
case ":${PATH:-}:" in
  *":${install_dir}:"*) ;;
  *) printf 'Add %s to your PATH to run lazykcp from any directory.\n' "$install_dir" ;;
esac
