package installtest

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func put(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
}
func archive(t *testing.T, dir, name, entry string) {
	t.Helper()
	c := exec.Command("tar", "-czf", filepath.Join(dir, name), "-C", dir, entry)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("tar: %v %s", err, out)
	}
}
func digest(t *testing.T, path string) string {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

type fixture struct {
	root, install, script string
	env                   []string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	// The install destination exercises quoting, including characters meaningful to shells.
	install := filepath.Join(root, "space ' dollar $ path")
	put(t, filepath.Join(root, "risu-bridge"), "#!/bin/sh\nprintf 'bridge %s\\n' \"$*\"\n")
	put(t, filepath.Join(root, "codex-aarch64-apple-darwin"), "#!/bin/sh\nprintf 'codex-cli 0.153.0\\n'\n")
	archive(t, root, "bridge.tar.gz", "risu-bridge")
	archive(t, root, "codex.tar.gz", "codex-aarch64-apple-darwin")
	put(t, filepath.Join(root, "SHA256SUMS"), digest(t, filepath.Join(root, "bridge.tar.gz"))+"  risu-bridge-darwin-arm64.tar.gz\n")
	put(t, filepath.Join(bin, "uname"), "#!/bin/sh\ncase \"$1\" in -s) echo Darwin;; -m) echo arm64;; esac\n")
	put(t, filepath.Join(bin, "curl"), `#!/bin/sh
set -eu
url= dest=
while [ "$#" -gt 0 ]; do
 case "$1" in https://*) url=$1;; -o) shift; dest=$1;; esac
 shift
done
if [ "${MOCK_FAIL:-}" = download ]; then exit 22; fi
if [ "${MOCK_FAIL:-}" = signal ]; then kill -TERM "$PPID"; exit 143; fi
case "$url" in
 */SHA256SUMS) source=SHA256SUMS;;
 */codex-aarch64-apple-darwin.tar.gz) source=codex.tar.gz;;
 */risu-bridge-darwin-arm64.tar.gz) source=bridge.tar.gz;;
 *) exit 23;;
esac
cp "$FIXTURE/$source" "$dest"
if [ "${MOCK_FAIL:-}" = checksum ]; then printf corrupt >> "$dest"; fi
`)
	realSHA, err := exec.LookPath("shasum")
	if err != nil {
		t.Fatal(err)
	}
	// Only the test fixture's Codex digest maps to the pinned upstream digest.
	// Corrupt bytes still fail, so cache-repair and checksum tests exercise the real branches.
	put(t, filepath.Join(bin, "shasum"), `#!/bin/sh
set -eu
if [ "${MOCK_FAIL:-}" = hash ]; then exit 7; fi
out=$("$REAL_SHA" "$@")
case "$out" in
 "$FIXTURE_CODEX_SHA "*) printf '%s  %s\n' 8cdecd0b8ebe23f20eb373010fd91e9517977e840b68941ef6a646b409cb32e1 "$3";;
 *) printf '%s\n' "$out";;
esac
`)
	script, err := filepath.Abs("../install.sh")
	if err != nil {
		t.Fatal(err)
	}
	env := []string{}
	for _, e := range os.Environ() {
		k, _, _ := strings.Cut(e, "=")
		if !strings.HasPrefix(k, "BRIDGE_") && !strings.HasPrefix(k, "MOCK_") && k != "PATH" && k != "LOG_LEVEL" && k != "NO_COLOR" {
			env = append(env, e)
		}
	}
	env = append(env, "PATH="+bin+":"+os.Getenv("PATH"), "FIXTURE="+root, "REAL_SHA="+realSHA, "FIXTURE_CODEX_SHA="+digest(t, filepath.Join(root, "codex.tar.gz")), "BRIDGE_INSTALL_DIR="+install, "NO_COLOR=1")
	return fixture{root, install, script, env}
}
func (f fixture) run(t *testing.T, piped bool, extra []string, args ...string) (string, error) {
	t.Helper()
	var cmd *exec.Cmd
	if piped {
		cmd = exec.Command("/bin/sh", append([]string{"-s", "--"}, args...)...)
		body, err := os.ReadFile(f.script)
		if err != nil {
			t.Fatal(err)
		}
		cmd.Stdin = strings.NewReader(string(body))
	} else {
		cmd = exec.Command("/bin/sh", append([]string{f.script}, args...)...)
	}
	cmd.Env = append(append([]string{}, f.env...), extra...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
func TestInstallerFilePipeAndPreservation(t *testing.T) {
	f := newFixture(t)
	out, err := f.run(t, false, nil, "--install-only")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	sentinel := filepath.Join(f.install, "data", "bridge-key")
	put(t, sentinel, "existing-key")
	out, err = f.run(t, true, nil, "--restart")
	if err != nil || !strings.Contains(out, "bridge --restart") {
		t.Fatalf("%v\n%s", err, out)
	}
	if b, _ := os.ReadFile(sentinel); string(b) != "existing-key" {
		t.Fatal("existing data changed")
	}
	out, err = f.run(t, true, nil)
	if err != nil || !strings.Contains(out, "bridge ") {
		t.Fatalf("default launch: %v\n%s", err, out)
	}
	// Corrupt cache must recover with a verified download.
	put(t, filepath.Join(f.install, "bin", "codex-aarch64-apple-darwin.tar.gz"), "corrupt")
	out, err = f.run(t, true, nil, "--install-only")
	if err != nil || !strings.Contains(out, "손상된") {
		t.Fatalf("%v\n%s", err, out)
	}
	temps, _ := filepath.Glob(filepath.Join(f.install, "bin", ".*"))
	if len(temps) != 0 {
		t.Fatalf("temporary files leaked: %v", temps)
	}
}
func TestInstallerFailureKeepsLauncher(t *testing.T) {
	for _, kind := range []string{"download", "checksum", "hash", "signal"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			if out, err := f.run(t, false, nil, "--install-only"); err != nil {
				t.Fatalf("%v %s", err, out)
			}
			launcher := filepath.Join(f.install, "bin", "risu-bridge")
			before, err := os.ReadFile(launcher)
			if err != nil {
				t.Fatal(err)
			}
			stages, _ := filepath.Glob(filepath.Join(f.install, "releases", "go.*"))
			out, err := f.run(t, true, []string{"MOCK_FAIL=" + kind}, "--install-only")
			if err == nil {
				t.Fatalf("failure accepted: %s", out)
			}
			after, _ := os.ReadFile(launcher)
			if string(before) != string(after) {
				t.Fatal("working launcher changed")
			}
			remaining, _ := filepath.Glob(filepath.Join(f.install, "releases", "go.*"))
			if len(remaining) != len(stages) {
				t.Fatalf("failed stage leaked: %v", remaining)
			}
		})
	}
}
func TestInstallerValidationBeforeMutation(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"--unknown"}, {"--adapter"}} {
		f := newFixture(t)
		out, err := f.run(t, true, nil, args...)
		if args[0] == "--help" {
			if err != nil {
				t.Fatal(out)
			}
		} else if err == nil {
			t.Fatal("invalid args accepted")
		}
		if _, err = os.Stat(f.install); !os.IsNotExist(err) {
			t.Fatal("validation created install directory")
		}
	}
}

func TestBuildFailurePreservesArtifacts(t *testing.T) {
	f := newFixture(t)
	scripts := filepath.Join(f.root, "scripts")
	if err := os.Mkdir(scripts, 0700); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile("build-release.sh")
	if err != nil {
		t.Fatal(err)
	}
	build := filepath.Join(scripts, "build-release.sh")
	put(t, build, string(body))
	dist := filepath.Join(f.root, "dist")
	if err = os.Mkdir(dist, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"risu-bridge-darwin-arm64.tar.gz", "risu-bridge-darwin-amd64.tar.gz", "SHA256SUMS"} {
		put(t, filepath.Join(dist, name), "previous release")
	}
	put(t, filepath.Join(f.root, "bin", "go"), `#!/bin/sh
set -eu
if [ "$GOARCH" = amd64 ]; then exit 9; fi
while [ "$#" -gt 0 ]; do
 if [ "$1" = -o ]; then shift; printf binary > "$1"; exit 0; fi
 shift
done
exit 1
`)
	cmd := exec.Command("/bin/sh", build)
	cmd.Env = f.env
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("build failure accepted: %s", out)
	}
	for _, name := range []string{"risu-bridge-darwin-arm64.tar.gz", "risu-bridge-darwin-amd64.tar.gz", "SHA256SUMS"} {
		b, e := os.ReadFile(filepath.Join(dist, name))
		if e != nil || string(b) != "previous release" {
			t.Fatalf("artifact changed: %s", name)
		}
	}
	stages, _ := filepath.Glob(filepath.Join(dist, ".build.*"))
	if len(stages) != 0 {
		t.Fatalf("build stage leaked: %v", stages)
	}
}
