#!/bin/sh
# Prebuilt Go bridge + official native Codex. No language runtime or compiler.
set -eu
umask 077
fail() { printf '\n설치 실패: %s\n' "$*" >&2; exit 1; }
[ "$(uname -s)" = Darwin ] || fail '현재 설치판은 macOS만 지원합니다.'
case "$(uname -m)" in
 arm64) arch=arm64; target=aarch64-apple-darwin; codex_sha=8cdecd0b8ebe23f20eb373010fd91e9517977e840b68941ef6a646b409cb32e1 ;;
 x86_64) arch=amd64; target=x86_64-apple-darwin; codex_sha=668309c7d7cc1ebee5f9b4485739f92fd5556b0805bd9f49a3479302b5f68adc ;;
 *) fail '지원하지 않는 CPU입니다.' ;;
esac
release=v0.2.0
install_dir=${BRIDGE_INSTALL_DIR:-"$HOME/.local/share/risu-subscription-bridge"}
case "$install_dir" in /*) ;; *) fail 'BRIDGE_INSTALL_DIR은 절대 경로여야 합니다.' ;; esac
mkdir -p "$install_dir/releases" "$install_dir/bin" "$install_dir/data"
stage=$(mktemp -d "$install_dir/releases/go.XXXXXX")
success=0
trap 'if [ "$success" = 0 ]; then rm -rf "$stage"; fi' EXIT
printf 'Risu Subscription Bridge · Go 설치\n위치: %s\n' "$install_dir"
fetch() { curl --fail --silent --show-error --location --retry 3 --proto '=https' --tlsv1.2 "$1" -o "$2"; }
# Local developer smoke tests may supply an already built binary. End users only download.
if [ -n "${BRIDGE_SOURCE_BIN:-}" ]; then
 cp "$BRIDGE_SOURCE_BIN" "$stage/risu-bridge"
else
 url="https://github.com/risu-harness/risu-subscription-bridge/releases/download/$release"
 archive="risu-bridge-darwin-$arch.tar.gz"
 fetch "$url/$archive" "$stage/$archive" || fail '브리지 다운로드 실패'
 fetch "$url/SHA256SUMS" "$stage/SHA256SUMS" || fail '체크섬 다운로드 실패'
 expected=$(awk -v name="$archive" '$2 == name {print $1}' "$stage/SHA256SUMS")
 [ ${#expected} = 64 ] || fail '브리지 체크섬이 올바르지 않습니다.'
 actual=$(shasum -a 256 "$stage/$archive" | awk '{print $1}')
 [ "$actual" = "$expected" ] || fail '브리지 checksum 불일치'
 tar -xzf "$stage/$archive" -C "$stage" risu-bridge || fail '브리지 압축 해제 실패'
fi
chmod 700 "$stage/risu-bridge"
"$stage/risu-bridge" --version || fail '브리지 실행 실패'
printf '공식 Codex 0.153.0 네이티브 실행 파일 준비 중…\n'
# Always use the pinned native artifact: a PATH codex might be an npm wrapper.
if [ -f "$install_dir/bin/codex-$target.tar.gz" ] && [ "${BRIDGE_FORCE_DOWNLOAD:-0}" != 1 ]; then
 cp "$install_dir/bin/codex-$target.tar.gz" "$stage/codex.tar.gz"
else
 fetch "https://github.com/openai/codex/releases/download/rust-v0.153.0/codex-$target.tar.gz" "$stage/codex.tar.gz" || fail 'Codex 다운로드 실패'
fi
actual=$(shasum -a 256 "$stage/codex.tar.gz" | awk '{print $1}')
[ "$actual" = "$codex_sha" ] || fail 'Codex checksum 불일치'
tar -xzf "$stage/codex.tar.gz" -C "$stage" "codex-$target" || fail 'Codex 압축 해제 실패'
mv "$stage/codex-$target" "$stage/codex"
chmod 700 "$stage/codex"
CODEX_HOME="$install_dir/data/codex" "$stage/codex" --version || fail 'Codex 실행 실패'
cp "$stage/codex.tar.gz" "$install_dir/bin/codex-$target.tar.gz"
# Only a mktemp-generated basename is inserted as shell code, never user paths.
release_dir=$(basename "$stage")
launcher="$install_dir/bin/.risu-bridge.$$"
cat > "$launcher" <<'SH'
#!/bin/sh
set -eu
install_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
export BRIDGE_DATA_DIR=${BRIDGE_DATA_DIR:-"$install_dir/data"}
SH
printf 'release_dir=%s\n' "$release_dir" >> "$launcher"
cat >> "$launcher" <<'SH'
export BRIDGE_CODEX_BIN="$install_dir/releases/$release_dir/codex"
exec "$install_dir/releases/$release_dir/risu-bridge" "$@"
SH
chmod 700 "$launcher"
mv -f "$launcher" "$install_dir/bin/risu-bridge"
success=1
printf '\n준비 완료. Node·Python·Go 설치가 필요하지 않습니다.\n기존 브리지가 실행 중이면 최신 Go 버전을 사용하려면 3번(재시작)을 선택하세요.\n'
if [ "${BRIDGE_INSTALL_ONLY:-0}" = 1 ]; then exit 0; fi
exec sh "$install_dir/bin/risu-bridge" "$@"
