package installtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeminiInstallerAndFailurePreservation(t *testing.T) {
	f := newFixture(t)
	f.script, _ = filepath.Abs("install-gemini.sh")
	f.env = append(f.env, "BRIDGE_GEMINI_INSTALL_DIR="+f.install)
	entry := "node-v22.1.0-darwin-arm64"
	if err := os.MkdirAll(filepath.Join(f.root, entry, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(f.root, entry, "bin/node"), `#!/bin/sh
set -eu
case "$1" in
 */npm-cli.js)
  [ "${MOCK_FAIL:-}" != npm ] || exit 12
  shift
  while [ "$#" -gt 0 ]; do
   if [ "$1" = --prefix ]; then shift; prefix=$1; fi
   shift
  done
  mkdir -p "$prefix/node_modules/@google/gemini-cli/bundle"
  : > "$prefix/node_modules/@google/gemini-cli/bundle/gemini.js";;
 *) printf '0.58.0\n';;
esac
`)
	archive(t, f.root, "node.tar.gz", entry)
	put(t, filepath.Join(f.root, "NODE-SUMS"), digest(t, filepath.Join(f.root, "node.tar.gz"))+"  "+entry+".tar.gz\n")
	put(t, filepath.Join(f.root, "bin/curl"), `#!/bin/sh
set -eu
url= dest=
while [ "$#" -gt 0 ]; do
 case "$1" in https://*) url=$1;; -o) shift; dest=$1;; esac
 shift
done
[ "${MOCK_FAIL:-}" != download ] || exit 22
case "$url" in
 */SHASUMS256.txt) cp "$FIXTURE/NODE-SUMS" "$dest";;
 *.tar.gz) cp "$FIXTURE/node.tar.gz" "$dest"; if [ "${MOCK_FAIL:-}" = checksum ]; then printf bad >> "$dest"; fi;;
 *) exit 23;;
esac
`)
	for _, pipe := range []bool{false, true} {
		if out, err := f.run(t, pipe, nil); err != nil {
			t.Fatalf("%v %s", err, out)
		}
	}
	launcher := filepath.Join(f.install, "bin/gemini")
	before, err := os.ReadFile(launcher)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(before), "exec ") {
		t.Fatal("missing launcher")
	}
	for _, failure := range []string{"download", "checksum", "npm"} {
		if _, err = f.run(t, false, []string{"MOCK_FAIL=" + failure}); err == nil {
			t.Fatal("failure accepted", failure)
		}
		after, _ := os.ReadFile(launcher)
		if string(after) != string(before) {
			t.Fatal("existing launcher changed", failure)
		}
	}
}
