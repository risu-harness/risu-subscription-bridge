#!/bin/sh
# Keep `curl ... | sh` compatible: sh reads the complete body before starting Bash.
# Based on BASH3 Boilerplate conventions: https://github.com/kvz/bash3boilerplate
# MIT License, Copyright (c) 2013 Kevin van Zonneveld and contributors.
exec /bin/bash -s -- "$@" <<'RISU_INSTALL_BASH'
set -o errexit
set -o errtrace
set -o nounset
set -o pipefail
IFS=$'\n\t'
umask 077

readonly __release='v0.2.0'
__codex_version=''
__stage=''
__launcher=''
__cache_temp=''
__publishing=0
__phase='초기화'
__install_only=${BRIDGE_INSTALL_ONLY:-0}
__bridge_args=()

log() {
  local level="${1}" priority="${2}" color=''
  shift 2
  if (( priority > ${LOG_LEVEL:-6} )); then return 0; fi
  if [[ -t 2 && -z "${NO_COLOR:-}" && "${TERM:-dumb}" != 'dumb' ]]; then
    color=$'\033[36m'
    if [[ "${level}" = 'ERROR' ]]; then color=$'\033[31m'; fi
  fi
  printf '%s[%s] %s' "${color}" "${level}" "$*" >&2
  if [[ -n "${color}" ]]; then printf '\033[0m' >&2; fi
  printf '\n' >&2
}
fail() { log ERROR 3 "$*"; exit 1; }
on_error() {
  local status="${1}" line="${2}"
  # Never log BASH_COMMAND, environment, or arguments: they may contain secrets.
  log ERROR 3 "${__phase} 실패 (행 ${line}, 종료 코드 ${status})."
  exit "${status}"
}
cleanup() {
  local status="${1}"
  trap - EXIT ERR HUP INT TERM
  set +o errexit
  if [[ -n "${__launcher}" ]]; then rm -f -- "${__launcher}"; fi
  if [[ -n "${__cache_temp}" ]]; then rm -f -- "${__cache_temp}"; fi
  # After publication begins, retain the validated release even if a signal lands
  # between the atomic launcher rename and the next shell statement.
  if [[ -n "${__stage}" && "${__publishing}" = 0 ]]; then rm -rf -- "${__stage}"; fi
  exit "${status}"
}
on_signal() {
  log ERROR 3 "${__phase} 중 설치가 중단되었습니다."
  exit "${1}"
}
trap 'on_error "$?" "${LINENO}"' ERR
trap 'cleanup "$?"' EXIT
trap 'on_signal 129' HUP
trap 'on_signal 130' INT
trap 'on_signal 143' TERM

usage() {
  cat <<'HELP'
Risu Subscription Bridge 설치
  curl -fsSL https://raw.githubusercontent.com/risu-harness/risu-subscription-bridge/main/install.sh | sh
  sh install.sh [--install-only] [--restart]

  --install-only  설치만 하고 실행하지 않음
  --restart       실행 중인 브리지를 종료 후 재시작 (진행 중 응답 중단)
  --version       설치할 브리지 릴리스 표시
  --help, -h      도움말

환경 변수:
  BRIDGE_INSTALL_DIR       설치할 절대 경로
  BRIDGE_INSTALL_ONLY=1    설치만 하기
  BRIDGE_FORCE_DOWNLOAD=1  Codex 캐시 대신 다시 다운로드
  BRIDGE_OPEN_BROWSER=0   설정 페이지 자동 열기 비활성화
  BRIDGE_ACTION           reuse / stop / restart
  LOG_LEVEL               3=오류, 4=경고, 6=정보(기본), 7=상세
  NO_COLOR=1              로그 색상 비활성화
HELP
}
parse_options() {
  while (( $# > 0 )); do
    case "${1}" in
      --help|-h) usage; exit 0 ;;
      --version) printf '%s\n' "${__release}"; exit 0 ;;
      --install-only) __install_only=1 ;;
      --restart) __bridge_args+=(--restart) ;;
      *) fail "알 수 없는 옵션입니다. --help를 확인하세요." ;;
    esac
    shift
  done
}
require_commands() {
  local cmd
  for cmd in uname mkdir mktemp curl plutil shasum awk tar cp mv chmod rm basename dirname cat; do
    command -v "${cmd}" >/dev/null 2>&1 || fail "필수 명령을 찾을 수 없습니다: ${cmd}"
  done
}
fetch() {
  curl --fail --silent --show-error --location --retry 3 \
    --connect-timeout 15 --max-time 600 --proto '=https' --proto-redir '=https' \
    --tlsv1.2 "${1}" -o "${2}"
}
checksum() { shasum -a 256 "${1}" | awk '{print $1}'; }

install_bridge() {
  __phase='브리지 다운로드·검증'
  local url archive expected actual
  if [[ -n "${BRIDGE_SOURCE_BIN:-}" ]]; then
    [[ -f "${BRIDGE_SOURCE_BIN}" ]] || fail 'BRIDGE_SOURCE_BIN 파일을 찾을 수 없습니다.'
    cp -- "${BRIDGE_SOURCE_BIN}" "${__stage}/risu-bridge"
  else
    url="https://github.com/risu-harness/risu-subscription-bridge/releases/download/${__release}"
    archive="risu-bridge-darwin-${__arch}.tar.gz"
    fetch "${url}/${archive}" "${__stage}/${archive}"
    fetch "${url}/SHA256SUMS" "${__stage}/SHA256SUMS"
    expected=$(awk -v name="${archive}" '$2 == name {print $1}' "${__stage}/SHA256SUMS")
    [[ "${expected}" =~ ^[0-9a-f]{64}$ ]] || fail '브리지 체크섬이 올바르지 않습니다.'
    actual=$(checksum "${__stage}/${archive}")
    [[ "${actual}" = "${expected}" ]] || fail '브리지 checksum 불일치'
    tar -xzf "${__stage}/${archive}" -C "${__stage}" risu-bridge
  fi
  chmod 700 "${__stage}/risu-bridge"
  "${__stage}/risu-bridge" --version
}
resolve_codex() {
  __phase='최신 Codex 안정 릴리스 확인'
  local metadata="${__stage}/codex-release.json" tag index=0 name digest
  fetch 'https://api.github.com/repos/openai/codex/releases/latest' "${metadata}"
  tag=$(plutil -extract tag_name raw -o - "${metadata}")
  [[ "${tag}" =~ ^rust-v[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail 'Codex 안정 릴리스 태그가 올바르지 않습니다.'
  [[ "$(plutil -extract prerelease raw -o - "${metadata}")" = false ]] || fail 'Codex 사전 릴리스는 설치하지 않습니다.'
  [[ "$(plutil -extract draft raw -o - "${metadata}")" = false ]] || fail 'Codex 초안 릴리스는 설치하지 않습니다.'
  __codex_version=${tag#rust-v}
  __codex_sha=''
  while name=$(plutil -extract "assets.${index}.name" raw -o - "${metadata}" 2>/dev/null); do
    if [[ "${name}" = "codex-${__target}.tar.gz" ]]; then
      digest=$(plutil -extract "assets.${index}.digest" raw -o - "${metadata}")
      [[ "${digest}" =~ ^sha256:[0-9a-f]{64}$ ]] || fail 'Codex SHA-256 정보가 없습니다.'
      __codex_sha=${digest#sha256:}
      break
    fi
    index=$((index + 1))
  done
  [[ -n "${__codex_sha}" ]] || fail '현재 Mac용 Codex 릴리스 파일을 찾을 수 없습니다.'
}
install_codex() {
  local existing
  existing=$(type -P codex || true)
  if [[ -n "${existing}" ]]; then
    # Preserve the PATH entry (including Homebrew's symlink), not its versioned target.
    existing="$(cd -- "$(dirname -- "${existing}")" && pwd)/$(basename -- "${existing}")"
    if ! CODEX_HOME="${__install_dir}/data/codex" "${existing}" --version; then
      fail 'PATH의 Codex를 실행할 수 없습니다. 기존 Codex 설치를 복구한 뒤 다시 실행하세요.'
    fi
    printf '%s\n' "${existing}" > "${__stage}/codex-path"
    log INFO 6 "기존 Codex 사용: ${existing} (브리지 자동 업데이트 제외)"
    return
  fi
  resolve_codex
  __phase='Codex 다운로드·검증'
  local cache="${__install_dir}/bin/codex-${__codex_version}-${__target}.tar.gz"
  local actual=''
  log INFO 6 "공식 Codex ${__codex_version} 네이티브 실행 파일 준비 중…"
  if [[ -f "${cache}" && "${BRIDGE_FORCE_DOWNLOAD:-0}" != 1 ]]; then
    cp -- "${cache}" "${__stage}/codex.tar.gz"
    actual=$(checksum "${__stage}/codex.tar.gz")
    if [[ "${actual}" != "${__codex_sha}" ]]; then log WARN 4 '손상된 Codex 캐시를 다시 다운로드합니다.'; fi
  fi
  if [[ "${actual}" != "${__codex_sha}" ]]; then
    fetch "https://github.com/openai/codex/releases/download/rust-v${__codex_version}/codex-${__target}.tar.gz" "${__stage}/codex.tar.gz"
    actual=$(checksum "${__stage}/codex.tar.gz")
  fi
  [[ "${actual}" = "${__codex_sha}" ]] || fail 'Codex checksum 불일치'
  tar -xzf "${__stage}/codex.tar.gz" -C "${__stage}" "codex-${__target}"
  mv -- "${__stage}/codex-${__target}" "${__stage}/codex"
  chmod 700 "${__stage}/codex"
  CODEX_HOME="${__install_dir}/data/codex" "${__stage}/codex" --version
  __cache_temp=$(mktemp "${__install_dir}/bin/.codex-cache.XXXXXX")
  cp -- "${__stage}/codex.tar.gz" "${__cache_temp}"
  mv -f -- "${__cache_temp}" "${cache}"
  __cache_temp=''
}
publish_updater() {
  # Embed the checked-in implementation; startup never executes downloaded scripts.
  local updater="${__stage}/update-codex.sh"
  cat > "${updater}" <<'SH'
#!/bin/bash
set -eEuo pipefail
umask 077
install_dir=$1
release_dir=$2
__install_dir=$install_dir
__target=$3
__phase='Codex 자동 업데이트'
lock="$install_dir/bin/.codex-update-lock"
mkdir "$lock" 2>/dev/null || exit 0
__stage=''
cleanup_update() {
  [[ -z "$__stage" ]] || rm -rf -- "$__stage"
  rmdir "$lock"
}
trap cleanup_update EXIT
trap 'exit 130' INT
trap 'exit 143' TERM HUP
__stage=$(mktemp -d "$install_dir/releases/.codex-update.XXXXXX")
fail() { printf '[WARN] %s\n' "$*" >&2; exit 1; }
SH
  declare -f checksum resolve_codex >> "${updater}"
  cat >> "${updater}" <<'SH'
fetch() {
  local limit=180
  [[ "$1" != 'https://api.github.com/repos/openai/codex/releases/latest' ]] || limit=10
  curl --fail --silent --show-error --location --connect-timeout 5 --max-time "$limit" \
    --proto '=https' --proto-redir '=https' --tlsv1.2 "$1" -o "$2"
}
resolve_codex
current="$install_dir/releases/$release_dir"
if [[ -f "$current/codex-version" && "$(cat "$current/codex-version")" = "$__codex_version" ]]; then exit 0; fi
printf '[INFO] Codex %s 업데이트 중…\n' "$__codex_version" >&2
fetch "https://github.com/openai/codex/releases/download/rust-v${__codex_version}/codex-${__target}.tar.gz" "$__stage/codex.tar.gz"
[[ "$(checksum "$__stage/codex.tar.gz")" = "$__codex_sha" ]] || fail 'Codex checksum 불일치'
tar -xzf "$__stage/codex.tar.gz" -C "$__stage" "codex-${__target}"
chmod 700 "$__stage/codex-${__target}"
[[ "$(CODEX_HOME="$install_dir/data/codex" "$__stage/codex-${__target}" --version)" = "codex-cli $__codex_version" ]] || fail 'Codex 실행·버전 검증 실패'
printf '%s\n' "$__codex_version" > "$__stage/codex-version"
# Same filesystem rename: running processes retain their executable; new starts use the new binary.
mv -f -- "$__stage/codex-${__target}" "$current/codex"
mv -f -- "$__stage/codex-version" "$current/codex-version"
# Only installer-owned compressed copies; never touch credentials or external Codex installs.
rm -f -- "$current/codex.tar.gz"
for cached in "$install_dir"/bin/codex-*.tar.gz; do
  [[ ! -f "$cached" ]] || rm -f -- "$cached"
done
printf '[INFO] Codex 업데이트 완료. 이전 실행 파일과 다운로드 캐시를 정리했습니다.\n' >&2
SH
  chmod 700 "${updater}"
  printf '%s\n' "${__codex_version}" > "${__stage}/codex-version"
}
publish_launcher() {
  __phase='실행 명령 설치'
  local release_dir
  release_dir=$(basename "${__stage}")
  __launcher=$(mktemp "${__install_dir}/bin/.risu-bridge.XXXXXX")
  cat > "${__launcher}" <<'SH'
#!/bin/sh
set -eu
install_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
export BRIDGE_DATA_DIR=${BRIDGE_DATA_DIR:-"$install_dir/data"}
SH
  # mktemp basename only; never interpolate user-provided paths as shell code.
  printf 'release_dir=%s\ncodex_target=%s\n' "${release_dir}" "${__target}" >> "${__launcher}"
  cat >> "${__launcher}" <<'SH'
if [ -f "$install_dir/releases/$release_dir/codex-path" ]; then
  BRIDGE_CODEX_BIN=$(cat "$install_dir/releases/$release_dir/codex-path")
  if [ ! -x "$BRIDGE_CODEX_BIN" ]; then
    printf '[ERROR] 기존 Codex를 찾을 수 없습니다. Codex 설치를 복구하거나 브리지 설치 명령을 다시 실행하세요.\n' >&2
    exit 1
  fi
else
case "${1:-}" in
  --help|-h|--version|--stop) ;;
  *)
    if ! /bin/bash "$install_dir/releases/$release_dir/update-codex.sh" "$install_dir" "$release_dir" "$codex_target"; then
      printf '[WARN] Codex 업데이트에 실패해 기존 버전으로 실행합니다.\n' >&2
    fi ;;
esac
BRIDGE_CODEX_BIN="$install_dir/releases/$release_dir/codex"
fi
export BRIDGE_CODEX_BIN
exec "$install_dir/releases/$release_dir/risu-bridge" "$@"
SH
  chmod 700 "${__launcher}"
  __publishing=1
  mv -f -- "${__launcher}" "${__install_dir}/bin/risu-bridge"
  __launcher=''
}
main() {
  case "${LOG_LEVEL:-6}" in [3-7]) ;; *) printf 'LOG_LEVEL은 3~7이어야 합니다.\n' >&2; exit 1 ;; esac
  parse_options "$@"
  case "${__install_only}:${BRIDGE_FORCE_DOWNLOAD:-0}" in [01]:[01]) ;; *) fail '설치 환경 플래그는 0 또는 1이어야 합니다.' ;; esac
  case "${BRIDGE_ACTION:-}" in ''|reuse|stop|restart) ;; *) fail 'BRIDGE_ACTION: reuse, stop, restart' ;; esac
  require_commands
  [[ "$(uname -s)" = Darwin ]] || fail '현재 설치판은 macOS만 지원합니다.'
  case "$(uname -m)" in
    arm64) __arch=arm64; __target=aarch64-apple-darwin ;;
    x86_64) __arch=amd64; __target=x86_64-apple-darwin ;;
    *) fail '지원하지 않는 CPU입니다.' ;;
  esac
  __install_dir=${BRIDGE_INSTALL_DIR:-"${HOME:?HOME이 필요합니다.}/.local/share/risu-subscription-bridge"}
  [[ "${__install_dir}" = /* ]] || fail 'BRIDGE_INSTALL_DIR은 절대 경로여야 합니다.'
  __phase='설치 경로 준비'
  mkdir -p -- "${__install_dir}/releases" "${__install_dir}/bin" "${__install_dir}/data/codex"
  __stage=$(mktemp -d "${__install_dir}/releases/go.XXXXXX")
  log INFO 6 "Risu Subscription Bridge · Go 설치: ${__install_dir}"
  install_bridge
  install_codex
  if [[ ! -f "${__stage}/codex-path" ]]; then publish_updater; fi
  publish_launcher
  log INFO 6 '준비 완료. 기존 브리지가 실행 중이면 3번(재시작)을 선택하세요.'
  if [[ "${__install_only}" = 1 ]]; then return 0; fi
  __phase='브리지 실행'
  # Bash 3 + nounset treats an empty array as unset; expand only when nonempty.
  if (( ${#__bridge_args[@]} > 0 )); then
    exec /bin/sh "${__install_dir}/bin/risu-bridge" "${__bridge_args[@]}"
  else
    exec /bin/sh "${__install_dir}/bin/risu-bridge"
  fi
}
main "$@"
RISU_INSTALL_BASH
