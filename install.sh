#!/usr/bin/env bash
set -euo pipefail

repository="${TAPX_REPO:-VAMPIRE0924/TapX}"
requested_version="${TAPX_VERSION:-latest}"

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "TapX one-click installation supports Linux only." >&2
  exit 1
fi
if [[ ! "$repository" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]]; then
  echo "TAPX_REPO has an invalid format." >&2
  exit 1
fi
if [[ "$requested_version" != "latest" && ! "${requested_version#v}" =~ ^[A-Za-z0-9._-]+$ ]]; then
  echo "TAPX_VERSION has an invalid format." >&2
  exit 1
fi
command -v python3 >/dev/null 2>&1 || {
  echo "python3 is required to verify release metadata." >&2
  exit 1
}

download() {
  local url="$1" output="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL --retry 3 --connect-timeout 15 -o "$output" "$url"
  elif command -v wget >/dev/null 2>&1; then
    wget -q -O "$output" "$url"
  else
    echo "curl or wget is required." >&2
    return 1
  fi
}

temporary_directory="$(mktemp -d /tmp/tapx-public-installer.XXXXXX)"
cleanup() {
  rm -rf -- "$temporary_directory"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

if [[ "$requested_version" == "latest" ]]; then
  metadata_url="https://api.github.com/repos/${repository}/releases/latest"
else
  metadata_url="https://api.github.com/repos/${repository}/releases/tags/v${requested_version#v}"
fi
download "$metadata_url" "$temporary_directory/release.json"

mapfile -t installer_metadata < <(python3 - "$temporary_directory/release.json" "$repository" "$requested_version" <<'PY'
import json
import re
import sys
from pathlib import Path

metadata_file, repository, requested = sys.argv[1:]
release = json.loads(Path(metadata_file).read_text(encoding="utf-8"))
tag = str(release.get("tag_name", "")).strip()
version = tag[1:] if tag.startswith("v") else tag
expected_version = requested[1:] if requested.startswith("v") else requested
if release.get("draft") or not version:
    raise SystemExit("release metadata is not a published version")
if requested != "latest" and version != expected_version:
    raise SystemExit("release version does not match TAPX_VERSION")
assets = [asset for asset in release.get("assets", []) if asset.get("name") == "install.sh"]
if len(assets) != 1:
    raise SystemExit("release does not contain exactly one install.sh asset")
asset = assets[0]
url = str(asset.get("browser_download_url", "")).strip()
expected_url = f"https://github.com/{repository}/releases/download/{tag}/install.sh"
if url != expected_url:
    raise SystemExit("release returned an unexpected installer URL")
digest = str(asset.get("digest", "")).strip().lower()
if not re.fullmatch(r"sha256:[0-9a-f]{64}", digest):
    raise SystemExit("release installer has no valid GitHub SHA-256 digest")
print(url)
print(digest.removeprefix("sha256:"))
PY
)

if ((${#installer_metadata[@]} != 2)); then
  echo "TapX release metadata is incomplete." >&2
  exit 1
fi

download "${installer_metadata[0]}" "$temporary_directory/install.sh"
actual_digest="$(sha256sum "$temporary_directory/install.sh" | awk '{print $1}')"
if [[ "$actual_digest" != "${installer_metadata[1]}" ]]; then
  echo "TapX installer checksum verification failed." >&2
  exit 1
fi

exec bash "$temporary_directory/install.sh" "$@"
