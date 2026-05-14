package platform

import (
	"os"
	"path/filepath"
	"runtime"
)

type Config struct {
	OS            string
	HostPath      string
	SocketNetwork string
	SocketAddress string
	StatePath     string
	GroupsPath    string
}

func DefaultConfig() Config {
	cfg := Config{OS: runtime.GOOS}
	cfg.HostPath = hostsFilePath(cfg.OS)
	cfg.SocketNetwork, cfg.SocketAddress = socketAddress(cfg.OS)
	cfg.StatePath, cfg.GroupsPath = stateAndGroupsPath(cfg.OS)
	return cfg
}

func hostsFilePath(goos string) string {
	switch goos {
	case "windows":
		sysRoot := os.Getenv("SystemRoot")
		if sysRoot == "" {
			sysRoot = `C:\Windows`
		}
		return filepath.Join(sysRoot, "System32", "drivers", "etc", "hosts")
	case "darwin", "linux":
		return "/etc/hosts"
	default:
		return "/etc/hosts"
	}
}

func socketAddress(goos string) (string, string) {
	switch goos {
	case "windows":
		return "tcp", "127.0.0.1:47321"
	case "darwin", "linux":
		return "unix", "/var/run/dopelock.sock"
	default:
		return "unix", "/tmp/dopelock.sock"
	}
}

func stateAndGroupsPath(goos string) (string, string) {
	switch goos {
	case "windows":
		base := filepath.Join(os.Getenv("ProgramData"), "dopelock")
		if os.Getenv("ProgramData") == "" {
			base = `C:\ProgramData\dopelock`
		}
		return filepath.Join(base, "state.json"), filepath.Join(base, "groups.json")
	case "darwin", "linux":
		return "/var/lib/dopelock/state.json", "/var/lib/dopelock/groups.json"
	default:
		return "/tmp/dopelock/state.json", "/tmp/dopelock/groups.json"
	}
}
