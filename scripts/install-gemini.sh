#!/bin/sh
# Optional Gemini CLI runtime. The default ChatGPT installation stays unchanged.
exec /bin/bash -s -- "$@" <<'RISU_GEMINI_BASH'
set -eEuo pipefail
IFS=$'\n\t'
umask 077
readonly gemini_version=0.58.0
install_dir=${BRIDGE_GEMINI_INSTALL_DIR:-"${BRIDGE_INSTALL_DIR:-$HOME/.local/share/risu-subscription-bridge}/gemini-cli"}
stage=''
launcher=''
published=0
cleanup() {
  local status=$?
  trap - EXIT ERR HUP INT TERM
  [[ -z "$launcher" ]] || rm -f -- "$launcher"
  if [[ -n "$stage" && "$published" = 0 ]]; then rm -rf -- "$stage"; fi
  exit "$status"
}
trap cleanup EXIT
trap 'printf "[ERROR] Gemini 설치에 실패했습니다. 기존 설치는 유지됩니다.\n" >&2' ERR
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
fetch() { curl --fail --silent --show-error --location --retry 2 --connect-timeout 15 --max-time 600 --proto '=https' --proto-redir '=https' "$1" -o "$2"; }
main() {
  local arch platform node_archive checksum actual release
  if [[ "${1:-}" = --help || "${1:-}" = -h ]]; then
    printf '사용법: sh scripts/install-gemini.sh\n브리지 전용 Node.js와 Gemini CLI를 설치합니다. 로그인은 브리지 설정 페이지에서 진행하세요.\n'; return
  fi
  [[ $# = 0 ]] || { printf '알 수 없는 옵션입니다.\n' >&2; return 1; }
  [[ "$install_dir" = /* ]] || { printf '설치 경로는 절대 경로여야 합니다.\n' >&2; return 1; }
  [[ "$(uname -s)" = Darwin ]] || { printf '현재 설치 스크립트는 macOS용입니다.\n' >&2; return 1; }
  case "$(uname -m)" in arm64) arch=arm64;; x86_64) arch=x64;; *) printf '지원하지 않는 CPU입니다.\n' >&2; return 1;; esac
  platform="darwin-$arch"
  mkdir -p "$install_dir/releases" "$install_dir/bin"
  stage=$(mktemp -d "$install_dir/releases/gemini-$gemini_version.XXXXXX")
  printf '[INFO] 브리지 전용 Node.js 22 LTS를 다운로드합니다.\n' >&2
  fetch 'https://nodejs.org/dist/latest-v22.x/SHASUMS256.txt' "$stage/SHASUMS256.txt"
  node_archive=$(awk -v platform="$platform" '$2 ~ ("^node-v22\\.[0-9]+\\.[0-9]+-" platform "\\.tar\\.gz$") {print $2}' "$stage/SHASUMS256.txt")
  [[ -n "$node_archive" && "$node_archive" != *$'\n'* ]] || { printf 'Node.js 배포 파일을 확인할 수 없습니다.\n' >&2; return 1; }
  checksum=$(awk -v name="$node_archive" '$2==name {print $1}' "$stage/SHASUMS256.txt")
  [[ "$checksum" =~ ^[0-9a-f]{64}$ ]]
  fetch "https://nodejs.org/dist/latest-v22.x/$node_archive" "$stage/node.tar.gz"
  actual=$(shasum -a 256 "$stage/node.tar.gz" | awk '{print $1}')
  [[ "$actual" = "$checksum" ]] || { printf 'Node.js checksum 불일치\n' >&2; return 1; }
  mkdir "$stage/node"
  tar -xzf "$stage/node.tar.gz" --strip-components=1 -C "$stage/node"
  printf '[INFO] Gemini CLI %s를 설치합니다.\n' "$gemini_version" >&2
  PATH="$stage/node/bin:$PATH" "$stage/node/bin/node" "$stage/node/lib/node_modules/npm/bin/npm-cli.js" install \
    --prefix "$stage/cli" --cache "$stage/npm-cache" --registry https://registry.npmjs.org \
    --ignore-scripts --omit=optional --no-audit --no-fund "@google/gemini-cli@$gemini_version"
  mkdir "$stage/check-home"
  GEMINI_CLI_HOME="$stage/check-home" "$stage/node/bin/node" "$stage/cli/node_modules/@google/gemini-cli/bundle/gemini.js" --version
  rm -rf -- "$stage/node.tar.gz" "$stage/npm-cache" "$stage/check-home"
  release=$(basename "$stage")
  launcher=$(mktemp "$install_dir/bin/.gemini.XXXXXX")
  cat > "$launcher" <<'SH'
#!/bin/sh
set -eu
install_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
SH
  printf 'release=%s\n' "$release" >> "$launcher"
  cat >> "$launcher" <<'SH'
export PATH="$install_dir/releases/$release/node/bin:$PATH"
exec "$install_dir/releases/$release/node/bin/node" "$install_dir/releases/$release/cli/node_modules/@google/gemini-cli/bundle/gemini.js" "$@"
SH
  chmod 700 "$launcher"
  published=1
  mv -f -- "$launcher" "$install_dir/bin/gemini"
  launcher=''
  printf '[INFO] 설치 완료: %s/bin/gemini\n브리지 설정 페이지에서 Gemini를 선택하고 Google 로그인 버튼을 누르세요.\n' "$install_dir" >&2
}
main "$@"
RISU_GEMINI_BASH
