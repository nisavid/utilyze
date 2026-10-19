//go:build linux

package cupti

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// The dynamic linker retains its startup search path. Clearing the environment
// later does not prevent a dependency from being loaded through that path.
// Check presence, not value: Go and the dynamic linker can select different
// occurrences when execve supplies duplicate environment keys.
var _, initialLibraryPathSet = os.LookupEnv("LD_LIBRARY_PATH")

func requiresTrustedLibraries() (bool, error) {
	hdr := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	data := [2]unix.CapUserData{}
	if err := unix.Capget(&hdr, &data[0]); err != nil {
		return false, fmt.Errorf("could not inspect privileges before loading CUPTI: %w", err)
	}
	uid, euid, suid := unix.Getresuid()
	gid, egid, sgid := unix.Getresgid()
	return isPrivileged(uid, euid, suid, gid, egid, sgid, data), nil
}

func isPrivileged(uid, euid, suid, gid, egid, sgid int, data [2]unix.CapUserData) bool {
	if uid == 0 || euid == 0 || suid == 0 || uid != euid || uid != suid || gid != egid || gid != sgid {
		return true
	}
	for _, caps := range data {
		// Permitted capabilities can be made effective by loaded native code;
		// inheritable capabilities can survive a subsequent exec.
		if caps.Effective|caps.Permitted|caps.Inheritable != 0 {
			return true
		}
	}
	return false
}

func libraryPaths(dir string, restricted bool) ([]string, error) {
	if restricted {
		if initialLibraryPathSet {
			return nil, errors.New("privileged CUPTI loading requires starting utlz without LD_LIBRARY_PATH")
		}
		if !filepath.IsAbs(dir) {
			return nil, errors.New("privileged CUPTI loading requires an absolute system library directory")
		}
	}
	paths := make([]string, 0, len(requiredLibs))
	for _, lib := range requiredLibs {
		path := filepath.Join(dir, lib)
		if restricted {
			var err error
			path, err = trustedLibraryPath(path)
			if err != nil {
				return nil, err
			}
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func trustedLibraryPath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("CUPTI library path must be absolute: %s", path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve CUPTI library %s: %w", path, err)
	}
	// dlopen expands dynamic string tokens even in an absolute filename. Never
	// validate a literal filename and then ask the linker to expand it.
	if strings.Contains(resolved, "$") {
		return "", fmt.Errorf("CUPTI library path contains a dynamic linker token: %s", resolved)
	}
	for component := resolved; ; component = filepath.Dir(component) {
		info, err := os.Lstat(component)
		if err != nil {
			return "", fmt.Errorf("inspect CUPTI path %s: %w", component, err)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != 0 || info.Mode().Perm()&0o022 != 0 {
			return "", fmt.Errorf("CUPTI path must be root-owned and not group- or world-writable: %s", component)
		}
		if component == resolved {
			if !info.Mode().IsRegular() {
				return "", fmt.Errorf("CUPTI library must be a regular file: %s", component)
			}
		} else if !info.IsDir() {
			return "", fmt.Errorf("CUPTI ancestor must be a directory: %s", component)
		}
		if component == "/" {
			break
		}
	}
	// Load this canonical name, not the original alias. Every component of the
	// returned path is protected from replacement by an unprivileged user.
	// Administrator-installed code and the system linker's configuration remain
	// trusted, including the dependencies declared by those libraries.
	return resolved, nil
}
