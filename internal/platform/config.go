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
	TokenPath     string
	DataDir       string
}

const (
	NetworkTCP  = "tcp"
	NetworkUnix = "unix"
)

func DefaultConfig() Config {
	cfg := Config{OS: runtime.GOOS}
	cfg.HostPath = hostsFilePath(cfg.OS)
	cfg.SocketNetwork, cfg.SocketAddress = socketAddress(cfg.OS)
	cfg.StatePath, cfg.GroupsPath, cfg.TokenPath, cfg.DataDir = statePaths(cfg.OS)
	return cfg
}

func (c Config) UsesUnixSocket() bool {
	return c.SocketNetwork == NetworkUnix
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
		return NetworkTCP, "127.0.0.1:47321"
	case "darwin", "linux":
		return NetworkUnix, "/var/run/bakchodi.sock"
	default:
		return NetworkUnix, "/tmp/bakchodi.sock"
	}
}

func statePaths(goos string) (string, string, string, string) {
	var base string
	switch goos {
	case "windows":
		base = filepath.Join(os.Getenv("ProgramData"), "bakchodi_band")
		if os.Getenv("ProgramData") == "" {
			base = `C:\ProgramData\bakchodi_band`
		}
	case "darwin", "linux":
		base = "/var/lib/bakchodi_band"
	default:
		base = "/tmp/bakchodi_band"
	}
	return filepath.Join(base, "state.json"),
		filepath.Join(base, "groups.json"),
		filepath.Join(base, "auth_token"),
		base
}
