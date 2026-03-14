package detect

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kamilrybacki/botl/internal/container"
)

// HostPackages detects installed package directories on the host
// and returns them as read-only mount specifications.
func HostPackages() []container.Mount {
	var mounts []container.Mount

	mounts = append(mounts, detectNode()...)
	mounts = append(mounts, detectPython()...)
	mounts = append(mounts, detectGo()...)
	mounts = append(mounts, detectRust()...)

	return mounts
}

func detectNode() []container.Mount {
	out, err := exec.Command("npm", "root", "-g").Output()
	if err != nil {
		return nil
	}
	dir := strings.TrimSpace(string(out))
	if !isDir(dir) {
		return nil
	}
	return []container.Mount{{Source: dir, Target: dir}}
}

func detectPython() []container.Mount {
	out, err := exec.Command("python3", "-c",
		"import site; print('\\n'.join(site.getsitepackages()))").Output()
	if err != nil {
		return nil
	}
	var mounts []container.Mount
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		dir := strings.TrimSpace(line)
		if dir != "" && isDir(dir) {
			mounts = append(mounts, container.Mount{Source: dir, Target: dir})
		}
	}
	return mounts
}

func detectGo() []container.Mount {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		gopath = filepath.Join(home, "go")
	}
	modCache := filepath.Join(gopath, "pkg", "mod")
	if !isDir(modCache) {
		return nil
	}
	return []container.Mount{{Source: modCache, Target: modCache}}
}

func detectRust() []container.Mount {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	registry := filepath.Join(home, ".cargo", "registry")
	if !isDir(registry) {
		return nil
	}
	return []container.Mount{{Source: registry, Target: registry}}
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
