//go:build linux

package cupti

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/ebitengine/purego"
	"golang.org/x/sys/unix"
)

// TestIsPrivileged covers root and differing IDs plus both capability words.
func TestIsPrivileged(t *testing.T) {
	ids := [6]int{1000, 1000, 1000, 1000, 1000, 1000}
	check := func(ids [6]int, caps [2]unix.CapUserData) bool {
		return isPrivileged(ids[0], ids[1], ids[2], ids[3], ids[4], ids[5], caps)
	}
	if check(ids, [2]unix.CapUserData{}) {
		t.Fatal("ordinary user without capabilities classified as privileged")
	}
	for i, name := range []string{"real UID", "effective UID", "saved UID", "real GID", "effective GID", "saved GID"} {
		t.Run(name, func(t *testing.T) {
			changed := ids
			changed[i] = 1001
			if !check(changed, [2]unix.CapUserData{}) {
				t.Fatal("changed credentials must require trusted libraries")
			}
			if i < 3 {
				changed = ids
				changed[i] = 0
				if !check(changed, [2]unix.CapUserData{}) {
					t.Fatal("root credentials must require trusted libraries")
				}
			}
		})
	}
	if !check([6]int{}, [2]unix.CapUserData{}) {
		t.Fatal("root with no capabilities must require trusted libraries")
	}
	for _, caps := range []unix.CapUserData{{Effective: 1}, {Permitted: 1}, {Inheritable: 1}} {
		for word := range 2 {
			data := [2]unix.CapUserData{}
			data[word] = caps
			if !check(ids, data) {
				t.Errorf("capability word %d, %+v must require trusted libraries", word, caps)
			}
		}
	}
}

// TestLibraryPathsRejectRelativeDirectories checks that only ordinary loads may
// use relative directories or the dynamic linker's default search path.
func TestLibraryPathsRejectRelativeDirectories(t *testing.T) {
	initial := initialLibraryPathSet
	initialLibraryPathSet = false
	t.Cleanup(func() { initialLibraryPathSet = initial })
	for _, dir := range []string{"", ".", "relative/cupti"} {
		if _, err := libraryPaths(dir, true); err == nil || !strings.Contains(err.Error(), "absolute") {
			t.Errorf("libraryPaths(%q, true) = %v; want absolute-directory rejection", dir, err)
		}
		if _, err := libraryPaths(dir, false); err != nil {
			t.Errorf("ordinary loading of %q rejected: %v", dir, err)
		}
	}
}

// TestSystemSearchDirsExcludeUserLocations verifies that environment-controlled
// user locations affect ordinary discovery but not system candidates.
func TestSystemSearchDirsExcludeUserLocations(t *testing.T) {
	want := systemSearchDirs()
	home := t.TempDir()
	userLibs := filepath.Join(home, ".local/lib/python3.99/site-packages/nvidia/cuda_cupti/lib")
	if err := os.MkdirAll(userLibs, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if got := systemSearchDirs(); !slices.Equal(got, want) {
		t.Fatalf("system discovery changed with user environment: %v; want %v", got, want)
	}
	if !slices.Contains(searchDirs(), userLibs) {
		t.Fatal("ordinary discovery omitted the user Python package directory")
	}
	if err := loadFromDirs(nil); err == nil {
		t.Fatal("empty candidate list must not report a successful load")
	}
}

// rootTempDir creates a fixture directory with trusted ancestors and automatic
// cleanup, skipping when root privileges or trusted ancestors are unavailable.
func rootTempDir(t *testing.T) string {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("requires root to create and change ownership of trusted fixtures; run sudo go test -count=1 -v ./internal/ffi/cupti")
	}
	for _, dir := range []string{"/", "/root"} {
		info, err := os.Lstat(dir)
		if err != nil || !info.IsDir() || info.Sys().(*syscall.Stat_t).Uid != 0 || info.Mode().Perm()&0o022 != 0 {
			t.Skipf("requires a root-owned, non-writable trusted ancestor: %s", dir)
		}
	}
	dir, err := os.MkdirTemp("/root", "utlz-cupti-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// TestTrustedLibraryPathRoot checks rejection of unsafe ownership, permissions,
// file types, symlink targets, and dynamic linker tokens.
func TestTrustedLibraryPathRoot(t *testing.T) {
	root := rootTempDir(t)
	cases := []struct {
		name   string
		modify func(string) error
	}{
		{"group writable file", func(p string) error { return os.Chmod(p, 0o664) }},
		{"world writable file", func(p string) error { return os.Chmod(p, 0o646) }},
		{"writable ancestor", func(p string) error { return os.Chmod(filepath.Dir(p), 0o777) }},
		{"user owned file", func(p string) error { return os.Chown(p, 65534, -1) }},
		{"user owned ancestor", func(p string) error { return os.Chown(filepath.Dir(p), 65534, -1) }},
		{"nonregular file", func(p string) error {
			if err := os.Remove(p); err != nil {
				return err
			}
			return os.Mkdir(p, 0o755)
		}},
		{"symlink to user target", func(p string) error {
			target := p + ".user"
			if err := os.Rename(p, target); err != nil {
				return err
			}
			if err := os.Chown(target, 65534, -1); err != nil {
				return err
			}
			return os.Symlink(target, p)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, err := os.MkdirTemp(root, "case-")
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, hostLibName)
			if err := os.WriteFile(path, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			if got, err := trustedLibraryPath(path); err != nil || got != path {
				t.Fatalf("safe path = %q, %v", got, err)
			}
			if err := tc.modify(path); err != nil {
				t.Fatal(err)
			}
			if _, err := trustedLibraryPath(path); err == nil {
				t.Fatal("unsafe path accepted")
			}
		})
	}
	path := filepath.Join(root, "$ORIGIN.so")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := trustedLibraryPath(path); err == nil || !strings.Contains(err.Error(), "token") {
		t.Fatalf("dynamic linker token accepted: %v", err)
	}
}

const constructor = `
#include <stdio.h>
#include <stdlib.h>
static void mark(char value) {
    const char *path = getenv("UTLZ_CUPTI_TEST_MARKER");
    FILE *file = path ? fopen(path, "a") : NULL;
    if (file) { fputc(value, file); fclose(file); }
}
`

// buildLibrary compiles a shared fixture with constructor marker support,
// skipping the test when gcc is unavailable.
func buildLibrary(t *testing.T, dir, name, source string, linkArgs ...string) {
	t.Helper()
	gcc, err := exec.LookPath("gcc")
	if err != nil {
		t.Skip("gcc is required for harmless ELF constructor fixtures")
	}
	args := append([]string{"-shared", "-fPIC", "-o", filepath.Join(dir, name), "-x", "c", "-"}, linkArgs...)
	cmd := exec.Command(gcc, args...)
	cmd.Stdin = strings.NewReader(constructor + source)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", name, err, output)
	}
}

// buildPair creates host and target fixtures with distinct constructor markers
// and the host symbol required by the loader's probe.
func buildPair(t *testing.T, dir string) {
	t.Helper()
	buildLibrary(t, dir, hostLibName, `
__attribute__((constructor)) static void init(void) { mark('H'); }
void NVPW_Device_RawCounterConfig_Create(void) { }
`)
	buildLibrary(t, dir, targetLibName, `
__attribute__((constructor)) static void init(void) { mark('T'); }
`)
}

// buildRawExecLauncher compiles a launcher that preserves empty and duplicate
// LD_LIBRARY_PATH entries for startup environment regression tests.
func buildRawExecLauncher(t *testing.T) string {
	t.Helper()
	gcc, err := exec.LookPath("gcc")
	if err != nil {
		t.Skip("gcc is required for raw execve environment fixtures")
	}
	path := filepath.Join(t.TempDir(), "raw-exec")
	cmd := exec.Command(gcc, "-o", path, "-x", "c", "-")
	cmd.Stdin = strings.NewReader(`
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
extern char **environ;
int main(int argc, char **argv) {
    if (argc != 4) return 111;
    size_t count = 0;
    while (environ[count]) count++;
    char **env = calloc(count + 3, sizeof(*env));
    if (!env) return 111;
    memcpy(env, environ, count * sizeof(*env));
    env[count++] = "LD_LIBRARY_PATH=";
    if (argv[3][0]) {
        size_t size = strlen(argv[3]) + sizeof("LD_LIBRARY_PATH=");
        env[count] = malloc(size);
        if (!env[count]) return 111;
        snprintf(env[count], size, "LD_LIBRARY_PATH=%s", argv[3]);
    }
    char *args[] = {argv[1], argv[2], NULL};
    execve(argv[1], args, env);
    perror("execve");
    return 111;
}
`)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build raw execve launcher: %v\n%s", err, output)
	}
	return path
}

// runLoader isolates a load attempt in a subprocess and checks constructor markers.
// An optional raw launcher preserves environment entries that os/exec deduplicates.
func runLoader(t *testing.T, dir, mode, startupPath string, rawLauncher ...string) string {
	t.Helper()
	marker := filepath.Join(t.TempDir(), "constructor-marker")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=^TestCUPTILoaderProcess$")
	if len(rawLauncher) != 0 {
		// os/exec deduplicates Env; C preserves the exact duplicate vector.
		cmd = exec.Command(rawLauncher[0], executable, "-test.run=^TestCUPTILoaderProcess$", startupPath)
	}
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "LD_LIBRARY_PATH=") && !strings.HasPrefix(value, "UTLZ_CUPTI_TEST_") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.Env = append(cmd.Env, "UTLZ_CUPTI_TEST_MODE="+mode, "UTLZ_CUPTI_TEST_DIR="+dir,
		"UTLZ_CUPTI_TEST_MARKER="+marker)
	if len(rawLauncher) != 0 {
		cmd.Env = append(cmd.Env, "UTLZ_CUPTI_TEST_FIRST_EMPTY=1")
	} else if startupPath != "" {
		cmd.Env = append(cmd.Env, "LD_LIBRARY_PATH="+startupPath)
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("loader %s: %v\n%s", mode, err, output)
	}
	data, err := os.ReadFile(marker)
	if strings.HasPrefix(mode, "reject") {
		if !os.IsNotExist(err) {
			t.Fatalf("rejected load ran a constructor: marker %q, error %v", data, err)
		}
	} else if err != nil || !strings.Contains(string(data), "H") || !strings.Contains(string(data), "T") {
		t.Fatalf("constructors did not run: marker %q, error %v", data, err)
	}
	return string(data)
}

// TestCUPTILoaderProcess is the subprocess entry point for loading policy tests.
// It clears LD_LIBRARY_PATH after startup to exercise the retained linker state.
func TestCUPTILoaderProcess(t *testing.T) {
	mode := os.Getenv("UTLZ_CUPTI_TEST_MODE")
	if mode == "" {
		return
	}
	dir := os.Getenv("UTLZ_CUPTI_TEST_DIR")
	startupPath, startupSet := os.LookupEnv("LD_LIBRARY_PATH")
	if os.Getenv("UTLZ_CUPTI_TEST_FIRST_EMPTY") != "" && (!startupSet || startupPath != "") {
		t.Fatal("raw launcher did not expose the first empty LD_LIBRARY_PATH to Go")
	}
	if err := os.Unsetenv("LD_LIBRARY_PATH"); err != nil {
		t.Fatal(err)
	}
	if mode == "legacy" {
		// Historical control: opening an unchecked path executes its constructor.
		for _, lib := range requiredLibs {
			if _, err := purego.Dlopen(filepath.Join(dir, lib), purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	var err error
	if mode == "reject-policy" {
		err = loadFromDirWithPolicy(dir, true)
	} else if mode == "ordinary" {
		restricted, policyErr := requiresTrustedLibraries()
		if policyErr != nil {
			t.Fatal(policyErr)
		}
		if restricted {
			err = loadFromDirWithPolicy(dir, false)
		} else {
			err = loadFromDir(dir)
		}
	} else {
		err = loadFromDir(dir)
	}
	if strings.HasPrefix(mode, "reject") {
		if err == nil {
			t.Fatal("privileged load unexpectedly succeeded")
		}
		if startupSet && !strings.Contains(err.Error(), "LD_LIBRARY_PATH") {
			t.Fatalf("startup linker path was not rejected: %v", err)
		}
	} else if err != nil {
		t.Fatal(err)
	}
}

// TestOrdinaryCUPTILoad verifies that ordinary policy accepts a writable fixture
// directory while restricted policy rejects it before constructors execute.
func TestOrdinaryCUPTILoad(t *testing.T) {
	dir := t.TempDir()
	buildPair(t, dir)
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	runLoader(t, dir, "ordinary", "")
	runLoader(t, dir, "reject-policy", "")
}

// TestPrivilegedCUPTILoadRoot checks trusted symlink loading and verifies that an
// unsafe second library prevents either library's constructor from executing.
func TestPrivilegedCUPTILoadRoot(t *testing.T) {
	root := rootTempDir(t)
	dir := filepath.Join(root, "installation")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	buildPair(t, dir)
	alias := filepath.Join(root, "cuda")
	if err := os.Symlink(dir, alias); err != nil {
		t.Fatal(err)
	}
	if got, err := trustedLibraryPath(filepath.Join(alias, hostLibName)); err != nil || got != filepath.Join(dir, hostLibName) {
		t.Fatalf("trusted directory symlink = %q, %v; want canonical installation path", got, err)
	}
	runLoader(t, alias, "trusted", "")
	if err := os.Chmod(filepath.Join(dir, targetLibName), 0o666); err != nil {
		t.Fatal(err)
	}
	runLoader(t, dir, "legacy", "")
	// The host is valid: rejecting the second library must precede its constructor.
	runLoader(t, dir, "reject", "")
}

// TestStartupLibraryPathDependencyRoot verifies that empty, nonempty, and duplicate
// startup LD_LIBRARY_PATH entries cannot redirect privileged dependency loading.
func TestStartupLibraryPathDependencyRoot(t *testing.T) {
	dir, dependencies := rootTempDir(t), t.TempDir()
	buildPair(t, dir)
	launcher := buildRawExecLauncher(t)
	runLoader(t, dir, "reject", "", launcher)
	buildLibrary(t, dependencies, "libfixture_dependency.so", `
__attribute__((constructor)) static void init(void) { mark('D'); }
void fixture_dependency(void) { }
`)
	buildLibrary(t, dir, hostLibName, `
extern void fixture_dependency(void);
__attribute__((constructor)) static void init(void) { fixture_dependency(); mark('H'); }
void NVPW_Device_RawCounterConfig_Create(void) { }
`, "-L"+dependencies, "-lfixture_dependency")
	if marker := runLoader(t, dir, "legacy", dependencies); !strings.Contains(marker, "D") {
		t.Fatal("control did not load dependency through startup LD_LIBRARY_PATH")
	}
	runLoader(t, dir, "reject", dependencies)
	if marker := runLoader(t, dir, "legacy", dependencies, launcher); !strings.Contains(marker, "D") {
		t.Fatal("duplicate environment control did not load dependency through the last LD_LIBRARY_PATH")
	}
	runLoader(t, dir, "reject", dependencies, launcher)
}
