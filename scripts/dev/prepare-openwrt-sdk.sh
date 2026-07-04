#!/usr/bin/env bash
set -euo pipefail

version="${OPENWRT_VERSION:-25.12.5}"
mirror="${OPENWRT_MIRROR:-https://downloads.openwrt.org}"
root="${TAPX_OPENWRT_SDK_ROOT:-$HOME/tapx-openwrt-sdk}"

targets=("$@")
if ((${#targets[@]} == 0)); then
  targets=(x86-64)
fi

validate_target() {
  case "$1" in
    x86-64) ;;
    *)
      echo "unsupported OpenWrt target '$1' in the current x86-64-only phase" >&2
      echo "add the target intentionally when non-x86 packaging work starts" >&2
      return 1
      ;;
  esac
}

target_path() {
  case "$1" in
    x86-64) echo "x86/64" ;;
    *) echo "unknown OpenWrt target $1" >&2; return 1 ;;
  esac
}

sdk_prefix() {
  case "$1" in
    x86-64) echo "openwrt-sdk-${version}-x86-64_" ;;
    *) echo "unknown OpenWrt target $1" >&2; return 1 ;;
  esac
}

download_sdk() {
  local target="$1"
  local path prefix base sha_file line sum file archive
  path="$(target_path "$target")"
  prefix="$(sdk_prefix "$target")"
  base="${mirror}/releases/${version}/targets/${path}"

  mkdir -p "${root}/downloads"
  sha_file="${root}/downloads/sha256sums-${version}-${target}"

  echo "fetch sha256sums: ${base}/sha256sums"
  if [[ "${TAPX_OPENWRT_OFFLINE:-0}" == "1" && -s "$sha_file" ]]; then
    echo "offline mode, reuse cached sha256sums: ${sha_file}"
  elif ! curl -4 -fL --retry 5 --connect-timeout 20 --max-time 180 \
    -o "$sha_file.tmp" "${base}/sha256sums"; then
    rm -f "$sha_file.tmp"
    if [[ -s "$sha_file" ]]; then
      echo "reuse cached sha256sums: ${sha_file}" >&2
    else
      return 1
    fi
  else
    mv "$sha_file.tmp" "$sha_file"
  fi

  line="$(awk -v prefix="$prefix" '{ file=$2; sub(/^\*/, "", file); if (file ~ "^" prefix && file ~ /Linux-x86_64\.tar\.(zst|xz)$/) { print; exit } }' "$sha_file")"
  if [[ -z "$line" ]]; then
    echo "sdk not found for target ${target} in ${base}/sha256sums" >&2
    return 1
  fi

  sum="${line%% *}"
  file="${line##* }"
  file="${file#\*}"
  archive="${root}/downloads/${file}"

  if [[ ! -f "$archive" ]]; then
    echo "download sdk: ${base}/${file}"
    curl -4 -fL --retry 5 --connect-timeout 20 --max-time 0 \
      -o "$archive" "${base}/${file}"
  else
    echo "reuse archive: ${archive}"
  fi

  (cd "${root}/downloads" && printf '%s  %s\n' "$sum" "$file" | sha256sum -c -)

  if [[ ! -d "${root}/${file%.tar.zst}" && ! -d "${root}/${file%.tar.xz}" ]]; then
    echo "extract sdk: ${archive}"
    tar -C "$root" -xf "$archive"
  else
    echo "sdk already extracted: ${file}"
  fi
}

for target in "${targets[@]}"; do
  validate_target "$target"
  download_sdk "$target"
done

echo
echo "toolchain bins:"
find "$root" -path "*/staging_dir/toolchain-*/bin" -type d -print | sort
