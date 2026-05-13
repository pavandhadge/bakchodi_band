package main

import (
	"os"
	"path/filepath"
	"runtime"
)

// getHostsFilePath returns the correct OS-specific path to the hosts file.
func getHostsFilePath(CURRENT_OS string) string {
	switch CURRENT_OS  {
	case "windows":
		// Safely construct the Windows path 
		sysRoot := os.Getenv("SystemRoot")
		if sysRoot == "" {
			sysRoot = `C:\Windows` // Fallback
		}
		return filepath.Join(sysRoot, "System32", "drivers", "etc", "hosts")
		
	case "darwin", "linux":
		// Both macOS and Linux use the same Unix path
		return "/etc/hosts"
		
	default:
		return "/etc/hosts"
	}
}


// getSocketAddress returns (networkType, address)
// For Unix-like: ("unix", "/var/run/dopelock.sock")
// For Windows:   ("pipe", `\\.\pipe\dopelock`)
func getSocketAddress(CURRENT_OS string) (string, string) {
	switch CURRENT_OS {
	case "windows":
		// Windows uses Named Pipes for IPC
		return "pipe", `\\.\pipe\dopelock`
	case "darwin", "linux":
		// Unix-like systems use Unix Domain Sockets
		return "unix", "/var/run/dopelock.sock"
	default:
		// Fallback for other Unices
		return "unix", "/tmp/dopelock.sock"
	}
}

func  getStateAndGroupJSONPath(CURRENT_OS string)(string,string){
	switch CURRENT_OS {
	case "windows":
		// Windows uses Named Pipes for IPC
		return "C:\\ProgramData\\dopelock\\state.json","C:\\ProgramData\\dopelock\\groups.json"
	case "darwin", "linux":
		// Unix-like systems use Unix Domain Sockets
		return "/var/lib/deeplock/state.json", "/var/lib/deeplock/groups.json"
	default:
		// Fallback for other Unices
		return "/tmp/dopelock.json","/tmp/groups.json"
	}
}
const (
	DEFAULT_TIMELIMIT = 45
)

var (
	CURRENT_OS = runtime.GOOS
	HOST_PATH  = getHostsFilePath(CURRENT_OS)
	SOCKET_NETWORK , SOCKET_PATH = getSocketAddress(CURRENT_OS)
	STATE_JSON,GROUPS_JSON = getStateAndGroupJSONPath(CURRENT_OS)
	
)
