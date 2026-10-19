//go:build linux

package cupti

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/systalyze/utilyze/internal/format"

	"github.com/ebitengine/purego"
)

const (
	hostLibName   = "libnvperf_host.so"
	targetLibName = "libnvperf_target.so"

	// some libnvperf_host libraries (i.e. distributed with nv-hostengine/dcgm) are stub libraries
	// we need to probe for the actual symbol we'll use in the native sampler library
	probeSymbol = "NVPW_Device_RawCounterConfig_Create"
)

var requiredLibs = []string{hostLibName, targetLibName}

var loadOnce sync.Once
var loadErr error

func EnsureLoaded() error {
	loadOnce.Do(func() {
		loadErr = load()
	})
	return loadErr
}

func cacheDir() (string, error) {
	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(userCacheDir, "utlz"), nil
}

func load() error {
	restricted, err := requiresTrustedLibraries()
	if err != nil {
		return err
	}
	if restricted {
		if err := loadFromDirs(systemSearchDirs()); err != nil {
			return fmt.Errorf("privileged CUPTI loading requires an administrator-installed library pair in a system directory (for example /usr/local/lib/utilyze/cupti); user packages, caches, and downloads are disabled: %w", err)
		}
		return nil
	}

	// try loading from LD_LIBRARY_PATH first
	if err := loadFromDir(""); err == nil {
		return nil
	}

	dirs := searchDirs()
	if len(dirs) > 0 {
		if err := loadFromDirs(dirs); err == nil {
			return nil
		}
	}

	cacheDir, err := cacheDir()
	if err != nil {
		return fmt.Errorf("could not get user cache directory for loading/extracting CUPTI: %w", err)
	}

	cuptiCacheDirs := globPaths(filepath.Join(cacheDir, "cupti-*"))
	if len(cuptiCacheDirs) > 0 {
		if err := loadFromDirs(cuptiCacheDirs); err == nil {
			return nil
		}
	}

	release, version, err := findLatestRelease()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to determine latest CUPTI release. You can try manually installing the latest CUPTI release from PyPI:")
		fmt.Fprintf(os.Stderr, "  https://pypi.org/project/%s/\n", cuptPypiDistribution)
		fmt.Fprintln(os.Stderr, "Then verify LD_LIBRARY_PATH includes the directory containing the library.")
		return fmt.Errorf("failed to load CUPTI: %w", err)
	}

	fmt.Fprintf(os.Stderr, "NVIDIA CUPTI 12+ was not found.\n\n")
	fmt.Fprintf(os.Stderr, "Would you like to download the latest release %s (%s) to %s? [y/N] ", release.Filename, format.SI(float64(release.Size), 3), cacheDir)
	ok, err := promptDownload()
	if err != nil {
		return fmt.Errorf("failed to prompt for download: %w", err)
	}

	if !ok {
		return errors.New("failed to load CUPTI: missing and did not download")
	}

	fmt.Fprint(os.Stderr, "Downloading from PyPI...")
	cuptiCacheDir := filepath.Join(cacheDir, "cupti-"+version)
	if err := installWheel(release, cuptiCacheDir); err != nil {
		return fmt.Errorf("failed to download or install CUPTI: %w", err)
	}
	fmt.Fprintln(os.Stderr, " done.")

	if err := loadFromDir(cuptiCacheDir); err != nil {
		return fmt.Errorf("load CUPTI from %s: %w", cuptiCacheDir, err)
	}
	return nil
}

func loadFromDirs(dirs []string) error {
	if len(dirs) == 0 {
		return errors.New("no CUPTI library directories found")
	}
	var err error
	for _, dir := range dirs {
		loadErr := loadFromDir(dir)
		if loadErr == nil {
			return nil
		}
		err = errors.Join(err, loadErr)
	}
	return err
}

func loadFromDir(dir string) (err error) {
	restricted, err := requiresTrustedLibraries()
	if err != nil {
		return err
	}
	return loadFromDirWithPolicy(dir, restricted)
}

func loadFromDirWithPolicy(dir string, restricted bool) (err error) {
	// Resolve and validate the entire pair before opening either library: the
	// first Dlopen can execute constructors or load the second as a dependency.
	paths, err := libraryPaths(dir, restricted)
	if err != nil {
		return err
	}
	handles := make([]uintptr, 0, len(requiredLibs))
	defer func() {
		if err != nil {
			for _, handle := range handles {
				purego.Dlclose(handle)
			}
		}
	}()

	for _, path := range paths {
		var handle uintptr
		handle, err = purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			return fmt.Errorf("could not load %s: %w", path, err)
		}
		handles = append(handles, handle)
	}

	// only the host lib needs to be probed
	_, err = purego.Dlsym(handles[0], probeSymbol)
	if err != nil {
		return fmt.Errorf("could not find %s in %s: %w", probeSymbol, filepath.Join(dir, hostLibName), err)
	}
	return nil
}

func searchDirs() []string {
	dirs := systemSearchDirs()
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		dirs = append(dirs, globPaths(filepath.Join(home, ".local/lib/python*/site-packages/nvidia/cuda_cupti/lib"))...)
	}
	return dirs
}

func systemSearchDirs() []string {
	return globPaths(
		"/usr/local/lib/utilyze/cupti",
		"/usr/local/lib",
		"/usr/local/lib64",
		"/usr/lib",
		"/usr/lib64",
		"/usr/lib/*-linux-gnu",
		"/lib",
		"/lib64",
		"/lib/*-linux-gnu",
		"/usr/local/cuda*/targets/*/lib",
		"/usr/local/cuda*/extras/CUPTI/lib64",
		"/opt/nvidia/nsight-compute/*/target/linux-desktop-glibc_2_11_3-x64",
		"/usr/lib/python*/dist-packages/nvidia/cuda_cupti/lib",
		"/usr/local/lib/python*/dist-packages/nvidia/cuda_cupti/lib",
		"/usr/lib/python*/site-packages/nvidia/cuda_cupti/lib",
		"/usr/local/lib/python*/site-packages/nvidia/cuda_cupti/lib",
	)
}

func globPaths(patterns ...string) []string {
	var dirs []string
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		dirs = append(dirs, matches...)
	}
	return dirs
}
