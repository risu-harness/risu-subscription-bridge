#!/bin/sh
# macOS bootstrap. No sudo, Homebrew, global npm install, or shell profile edits.
set -eu
umask 077
fail() { printf '\n설치 실패: %s\n' "$*" >&2; exit 1; }
[ "$(uname -s)" = Darwin ] || fail '현재 설치판은 macOS만 지원합니다.'
case "$(uname -m)" in
  arm64) arch=arm64; node_sha=61130f394c1630d211dd50aecc4353d379480f36d3ac913cd85dbba1aed585c6 ;;
  x86_64) arch=x64; node_sha=58e99022c2ff89395576cc7fd4d98cea24bb68081475d5f88b801ee8729fb026 ;;
  *) fail '지원하지 않는 CPU입니다.' ;;
esac
install_dir=${BRIDGE_INSTALL_DIR:-"$HOME/.local/share/risu-subscription-bridge"}
case "$install_dir" in /*) ;; *) fail 'BRIDGE_INSTALL_DIR은 절대 경로여야 합니다.' ;; esac
mkdir -p "$install_dir/releases" "$install_dir/bin" "$install_dir/data" "$install_dir/downloads"
stage=$(mktemp -d "$install_dir/releases/install.XXXXXX")
printf 'Risu Subscription Bridge 설치\n위치: %s\n' "$install_dir"

# A local script uses its checkout; a piped script fetches the public repository.
source_dir=${BRIDGE_SOURCE_DIR:-}
if [ -z "$source_dir" ] && [ -f "$0" ]; then
  candidate=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
  [ ! -f "$candidate/src/main.mjs" ] || source_dir=$candidate
fi
if [ -n "$source_dir" ]; then
  [ -f "$source_dir/src/main.mjs" ] || fail '소스 폴더에 src/main.mjs가 없습니다.'
  for entry in src scripts test package.json README.md install.sh; do
    cp -R "$source_dir/$entry" "$stage/"
  done
else
  repo=${BRIDGE_REPO:-risu-harness/risu-subscription-bridge}
  ref=${BRIDGE_REF:-main}
  printf 'GitHub에서 소스 다운로드 중…\n'
  curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 "https://api.github.com/repos/$repo/tarball/$ref" -o "$stage/source.tar.gz" || fail 'GitHub 소스 다운로드 실패'
  tar -xzf "$stage/source.tar.gz" --strip-components=1 -C "$stage" || fail '소스 압축 해제 실패'
fi
[ -f "$stage/src/main.mjs" ] || fail '소스가 올바르지 않습니다.'

node_bin=${BRIDGE_NODE_BIN:-}
if [ -z "$node_bin" ]; then node_bin=$(command -v node || true); fi
if [ "${BRIDGE_FORCE_DOWNLOAD:-0}" = 1 ]; then node_bin=; fi
if [ -z "$node_bin" ] || ! "$node_bin" -e 'process.exit(Number(process.versions.node.split(".")[0]) >= 22 ? 0 : 1)' >/dev/null 2>&1; then
  printf 'Node.js 22.23.2 다운로드 및 SHA-256 확인 중…\n'
  archive="$install_dir/downloads/node-v22.23.2-darwin-$arch.tar.gz"
  curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 "https://nodejs.org/dist/v22.23.2/node-v22.23.2-darwin-$arch.tar.gz" -o "$archive"
  actual=$(shasum -a 256 "$archive" | awk '{print $1}')
  [ "$actual" = "$node_sha" ] || fail 'Node.js checksum 불일치. 설치를 중단합니다.'
  mkdir -p "$install_dir/node-22.23.2"
  tar -xzf "$archive" --strip-components=1 -C "$install_dir/node-22.23.2"
  node_bin="$install_dir/node-22.23.2/bin/node"
fi
node_bin=$("$node_bin" -p 'process.execPath')
export PATH="$(dirname "$node_bin"):$PATH"

codex_bin=${BRIDGE_CODEX_BIN:-}
if [ -z "$codex_bin" ] && [ "${BRIDGE_FORCE_DOWNLOAD:-0}" != 1 ]; then
  if [ -x /Applications/ChatGPT.app/Contents/Resources/codex ]; then codex_bin=/Applications/ChatGPT.app/Contents/Resources/codex
  else codex_bin=$(command -v codex || true); fi
fi
# Pin the tested protocol version; do not silently run an older incompatible CLI.
if [ -z "$codex_bin" ] || ! "$codex_bin" --version 2>/dev/null | grep -q '^codex-cli 0\.153\.0$'; then
  printf '공식 npm 패키지 Codex 0.153.0 설치 중…\n'
  npm_cli="$(dirname "$node_bin")/../lib/node_modules/npm/bin/npm-cli.js"
  [ -f "$npm_cli" ] || fail '선택된 Node의 npm을 찾을 수 없습니다. BRIDGE_FORCE_DOWNLOAD=1로 다시 실행하세요.'
  "$node_bin" "$npm_cli" install --prefix "$install_dir/codex-0.153.0" --cache "$install_dir/npm-cache" --no-audit --no-fund --ignore-scripts --registry=https://registry.npmjs.org @openai/codex@0.153.0
  codex_bin="$install_dir/codex-0.153.0/node_modules/.bin/codex"
fi
"$codex_bin" --version || fail 'Codex 실행 실패'

# Store absolute paths as JSON, never interpolate them as shell source.
"$node_bin" --input-type=module - "$install_dir" "$stage" "$node_bin" "$codex_bin" <<'JS'
import {writeFile,chmod,rename} from 'node:fs/promises';
import {join} from 'node:path';
const [dir,source,node,codex]=process.argv.slice(2);
const configTemp=join(dir,`install.${process.pid}.json`);
await writeFile(configTemp,JSON.stringify({source,node,codex},null,2),{mode:0o600});
await rename(configTemp,join(dir,'install.json'));
const quote=s=>"'"+s.replaceAll("'","'\\''")+"'";
const launcherTemp=join(dir,'bin',`risu-bridge.${process.pid}`);
await writeFile(launcherTemp,'#!/bin/sh\nexec '+quote(node)+' '+quote(join(source,'scripts','launch.mjs'))+' '+quote(dir)+' "$@"\n',{mode:0o700});
await chmod(launcherTemp,0o700);
await rename(launcherTemp,join(dir,'bin','risu-bridge'));
JS
printf '\n준비 완료. 다음에도 같은 curl 명령을 사용하세요. 실행 중이면 기존 브리지를 다시 엽니다.\n'
if [ "${BRIDGE_INSTALL_ONLY:-0}" = 1 ]; then exit 0; fi
exec sh "$install_dir/bin/risu-bridge"
