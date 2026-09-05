#!/bin/sh
# b3bp-style Bash 3 strict mode; retain `sh scripts/build-release.sh` support.
exec /bin/bash -s -- "$0" "$@" <<'RISU_BUILD_BASH'
set -o errexit
set -o errtrace
set -o nounset
set -o pipefail
IFS=$'\n\t'
__file=${1}
shift
__stage=''
cleanup() {
  local status=${1}
  trap - EXIT ERR HUP INT TERM
  if [[ -n "${__stage}" ]]; then rm -rf -- "${__stage}" || true; fi
  exit "${status}"
}
trap 'printf "릴리스 빌드 실패 (행 %s).\n" "${LINENO}" >&2' ERR
trap 'cleanup "$?"' EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
main() {
  local version=${1:-0.2.2} arch cmd
  [[ $# -le 1 && "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { printf '사용법: sh scripts/build-release.sh [0.2.2]\n' >&2; exit 1; }
  for cmd in go tar shasum mktemp mkdir mv rm dirname; do
    command -v "${cmd}" >/dev/null || { printf '필수 명령 없음: %s\n' "${cmd}" >&2; exit 1; }
  done
  cd -- "$(dirname -- "${__file}")/.."
  mkdir -p dist
  __stage=$(mktemp -d dist/.build.XXXXXX)
  for arch in arm64 amd64; do
    mkdir -p "${__stage}/${arch}"
    CGO_ENABLED=0 GOOS=darwin GOARCH="${arch}" go build -buildvcs=false -trimpath -ldflags="-s -w -X main.version=${version}" -o "${__stage}/${arch}/risu-bridge" ./cmd/risu-bridge
    tar -czf "${__stage}/risu-bridge-darwin-${arch}.tar.gz" -C "${__stage}/${arch}" risu-bridge
  done
  (cd -- "${__stage}" && shasum -a 256 risu-bridge-darwin-*.tar.gz > SHA256SUMS)
  # Publish only after both architectures build successfully; checksum goes last.
  for arch in arm64 amd64; do mv -f -- "${__stage}/risu-bridge-darwin-${arch}.tar.gz" dist/; done
  mv -f -- "${__stage}/SHA256SUMS" dist/
}
main "$@"
RISU_BUILD_BASH
