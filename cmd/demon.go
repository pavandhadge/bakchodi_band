package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// isAdmin checks if the current process has root/admin privileges.
func isAdmin() bool {
	switch runtime.GOOS {
	case "windows":
		// Check if the current process is "elevated"
		var token windows.Token
		// Open current process token
		err := windows.OpenCurrentProcessToken(&token)
		if err != nil {
			return false
		}
		defer token.Close()

		// Get elevation info
		var isElevated uint32
		var outLen uint32
		err = windows.GetTokenInformation(token, windows.TokenElevation, (*byte)(unsafe.Pointer(&isElevated)), uint32(unsafe.Sizeof(isElevated)), &outLen)
		if err != nil {
			return false
		}
		return isElevated != 0

	case "linux", "darwin":
		// Standard Unix check for UID 0
		return os.Geteuid() == 0

	default:
		return false
	}
}

func runDaemon() {
	if !isAdmin() {
		if runtime.GOOS == "windows" {
			fmt.Println("FATAL: Please run this terminal as Administrator.")
		} else {
			fmt.Println("FATAL: This daemon must be run with sudo.")
		}
		os.Exit(1)
	}

	// 1. Clean up old socket files if the daemon crashed previously
	os.Remove(SOCKET_PATH)


	// 2. Create the Unix socket listener
	listener, err := net.Listen(SOCKET_NETWORK, SOCKET_PATH)
	if err != nil {
		fmt.Printf("Failed to listen on socket: %v\n", err)
		os.Exit(1)
	}
	defer listener.Close()

	// 3. Set file permissions so only root can write to the socket (Security!)
	// Note: os.Chmod doesn't work properly on Windows for socket files
	if runtime.GOOS != "windows" {
		os.Chmod(SOCKET_PATH, 0600)
	}

// 4. Define the HTTP route and what to do when a message arrives
	http.HandleFunc("/unlock", handleFullUnlock)
	http.HandleFunc("/unlock-group", handleGroupUnlock)
	http.HandleFunc("/unlock-url", handleURLUnlock)
	http.HandleFunc("/lock", handleFullLock)
	http.HandleFunc("/lock-group", handleGroupLock)
	http.HandleFunc("/lock-url", handleURLLock)
	http.HandleFunc("/add-group", handleAddGroup)
	http.HandleFunc("/add-url-to-group", handleAddURLToGroup)

	// 5. Start background ticker to check for expired unlocks
	go runExpiryChecker()

	// 6. Start the server (This blocks forever)
	fmt.Println("Daemon listening on", SOCKET_PATH)
	http.Serve(listener, nil)
}

func runExpiryChecker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		<-ticker.C
		cleanupExpiredUnlocks()
	}
}

func cleanupExpiredUnlocks() {
	state, err := loadState()
	if err != nil || len(state.ActiveUnlocks) == 0 {
		return
	}

	groups, _ := loadGroups()
	now := time.Now()
	changed := false

	var validUnlocks []UnlockState
	for _, unlock := range state.ActiveUnlocks {
		if now.Before(unlock.Expiry) {
			validUnlocks = append(validUnlocks, unlock)
		} else {
			changed = true
			fmt.Printf("🔒 Expired: %s (%s)\n", unlock.Target, unlock.Type)
		}
	}

	if changed {
		state.ActiveUnlocks = validUnlocks
		saveState(state)

		remainingBlocks := calculateBlocklist(groups, state.ActiveUnlocks)
		syncHostsFile(remainingBlocks)
		fmt.Println("✅ Expired unlocks cleaned up and sites re-blocked")
	}
}


func loadState() (StateJSON, error) {
	data, err := os.ReadFile(STATE_JSON)
	if err != nil {
		// If file doesn't exist yet, return empty state
		return StateJSON{ActiveUnlocks: []UnlockState{}}, nil 
	}
	var state StateJSON
	json.Unmarshal(data, &state)
	return state, nil
}

func saveState(state StateJSON) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(STATE_JSON, data, 0644)
}

func loadGroups() (GroupMap, error) {
	data, err := os.ReadFile(GROUPS_JSON) // The file where you define your groups
	if err != nil {
		return make(GroupMap), err
	}
	var groups GroupMap
	json.Unmarshal(data, &groups)
	return groups, nil
}

func calculateBlocklist(allGroups GroupMap, activeUnlocks []UnlockState) []string {
	// 1. Check for Global Unlock first
	for _, unlock := range activeUnlocks {
		if unlock.Type == "all" && time.Now().Before(unlock.Expiry) {
			return []string{} // Return empty list = block nothing
		}
	}

	// 2. Start by assuming EVERYTHING should be blocked
	toBlock := make(map[string]bool)
	for _, urls := range allGroups {
		for _, url := range urls {
			toBlock[url] = true
		}
	}

	// 3. Remove URLs that are actively unlocked
	for _, unlock := range activeUnlocks {
		if time.Now().After(unlock.Expiry) {
			continue // Skip expired timers
		}

		if unlock.Type == "group" {
			// Remove all URLs in this group from the blocklist
			if urls, exists := allGroups[unlock.Target]; exists {
				for _, url := range urls {
					delete(toBlock, url)
				}
			}
		} else if unlock.Type == "url" {
			// Remove specific URL from the blocklist
			delete(toBlock, unlock.Target)
		}
	}

	// 4. Convert the remaining map keys to a slice
	var finalList []string
	for url := range toBlock {
		finalList = append(finalList, url)
	}
	return finalList
}


// syncHostsFile handles the logic of finding markers and replacing content
func syncHostsFile(urlsToBlock []string) error {
	const (
		startMarker = "# --- DOPAMINE-LOCK-START ---"
		endMarker   = "# --- DOPAMINE-LOCK-END ---"
	)

	// Read the current file
	input, err := os.ReadFile(HOST_PATH)
	if err != nil {
		return err
	}

	lines := strings.Split(string(input), "\n")
	var newLines []string
	inDopamineBlock := false
	foundBlock := false

	// Filter out the old block
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == startMarker {
			inDopamineBlock = true
			foundBlock = true
			continue
		}
		if trimmed == endMarker {
			inDopamineBlock = false
			continue
		}
		if !inDopamineBlock {
			newLines = append(newLines, line)
		}
	}

	// Prepare the new block content
	var blockContent []string
	if len(urlsToBlock) > 0 {
		blockContent = append(blockContent, startMarker)
		for _, url := range urlsToBlock {
			blockContent = append(blockContent, "127.0.0.1 "+url)
			blockContent = append(blockContent, "127.0.0.1 www."+url)
		}
		blockContent = append(blockContent, endMarker)
	}

	// If we found an old block, replace it. If not, append to end.
	var finalOutput string
	if foundBlock {
		// Insert the new block where the old one used to be or just join
		// For simplicity in an MVP, we append the block to the end of preserved lines
		finalOutput = strings.Join(newLines, "\n") + "\n" + strings.Join(blockContent, "\n")
	} else {
		finalOutput = strings.Join(newLines, "\n") + "\n\n" + strings.Join(blockContent, "\n")
	}

	// Write back to file (Truncates automatically)
	return os.WriteFile(HOST_PATH, []byte(strings.TrimSpace(finalOutput)+"\n"), 0644)
}

func handleFullUnlock(w http.ResponseWriter, r *http.Request) {
	var req UnlockState
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 1. Update the State JSON
	expiryTime := time.Now().Add(time.Duration(DEFAULT_TIMELIMIT) * time.Minute)
	universalRule := UnlockState{
		Target: "all",
		Type:   "all",
		Expiry: expiryTime,
	}

	stateJson := StateJSON{ActiveUnlocks: []UnlockState{universalRule}}
	jsonData, err := json.MarshalIndent(stateJson, "", "  ")
	if err != nil {
		http.Error(w, "JSON Marshal failed", http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(STATE_JSON, jsonData, 0644); err != nil {
		http.Error(w, "Failed to save state", http.StatusInternalServerError)
		return
	}

	// 2. Clear the blocks from the Hosts file
	// Since this is a "Full Unlock", we pass an empty list of URLs to block
	err = syncHostsFile([]string{})
	if err != nil {
		http.Error(w, "Failed to update hosts file", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("Global unlock activated until %s\n", expiryTime.Format("15:04:05"))))
}

func handleGroupUnlock(w http.ResponseWriter, r *http.Request) {
	var req UnlockState
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 1. Load existing state and groups
	state, _ := loadState()
	groups, _ := loadGroups()

	// 2. Create the new rule
	expiryTime := time.Now().Add(time.Duration(DEFAULT_TIMELIMIT) * time.Minute)
	newUnlock := UnlockState{
		Target: req.Target, // e.g., "social"
		Type:   "group",
		Expiry: expiryTime,
	}

	// 3. Update State (Replace if exists, append if new)
	updated := false
	for i, existing := range state.ActiveUnlocks {
		if existing.Target == req.Target && existing.Type == "group" {
			state.ActiveUnlocks[i] = newUnlock
			updated = true
			break
		}
	}
	if !updated {
		state.ActiveUnlocks = append(state.ActiveUnlocks, newUnlock)
	}

	// 4. Persist updated state to disk
	if err := saveState(state); err != nil {
		http.Error(w, "Failed to save state", http.StatusInternalServerError)
		return
	}

	// 5. Calculate what remains blocked and Sync Hosts File
	remainingBlocks := calculateBlocklist(groups, state.ActiveUnlocks)
	err := syncHostsFile(remainingBlocks)
	if err != nil {
		http.Error(w, "Failed to update hosts file", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("Group '%s' unlocked until %s\n", req.Target, expiryTime.Format("15:04:05"))))
}

func handleURLUnlock(w http.ResponseWriter, r *http.Request) {
	var req UnlockState
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	state, _ := loadState()
	groups, _ := loadGroups()

	expiryTime := time.Now().Add(time.Duration(DEFAULT_TIMELIMIT) * time.Minute)
	newUnlock := UnlockState{
		Target: req.Target,
		Type:   "url",
		Expiry: expiryTime,
	}

	updated := false
	for i, existing := range state.ActiveUnlocks {
		if existing.Target == req.Target && existing.Type == "url" {
			state.ActiveUnlocks[i] = newUnlock
			updated = true
			break
		}
	}
	if !updated {
		state.ActiveUnlocks = append(state.ActiveUnlocks, newUnlock)
	}

	if err := saveState(state); err != nil {
		http.Error(w, "Failed to save state", http.StatusInternalServerError)
		return
	}

	remainingBlocks := calculateBlocklist(groups, state.ActiveUnlocks)
	err := syncHostsFile(remainingBlocks)
	if err != nil {
		http.Error(w, "Failed to update hosts file", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("URL '%s' unlocked until %s\n", req.Target, expiryTime.Format("15:04:05"))))
}

func handleFullLock(w http.ResponseWriter, r *http.Request) {
	state := StateJSON{ActiveUnlocks: []UnlockState{}}
	if err := saveState(state); err != nil {
		http.Error(w, "Failed to save state", http.StatusInternalServerError)
		return
	}

	groups, _ := loadGroups()
	allURLs := []string{}
	for _, urls := range groups {
		allURLs = append(allURLs, urls...)
	}

	err := syncHostsFile(allURLs)
	if err != nil {
		http.Error(w, "Failed to update hosts file", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("All sites locked\n"))
}

func handleGroupLock(w http.ResponseWriter, r *http.Request) {
	var req UnlockState
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	state, _ := loadState()
	groups, _ := loadGroups()

	newActiveUnlocks := []UnlockState{}
	for _, unlock := range state.ActiveUnlocks {
		if !(unlock.Type == "group" && unlock.Target == req.Target) {
			newActiveUnlocks = append(newActiveUnlocks, unlock)
		}
	}
	state.ActiveUnlocks = newActiveUnlocks

	if err := saveState(state); err != nil {
		http.Error(w, "Failed to save state", http.StatusInternalServerError)
		return
	}

	remainingBlocks := calculateBlocklist(groups, state.ActiveUnlocks)
	err := syncHostsFile(remainingBlocks)
	if err != nil {
		http.Error(w, "Failed to update hosts file", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("Group '%s' locked\n", req.Target)))
}

func handleURLLock(w http.ResponseWriter, r *http.Request) {
	var req UnlockState
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	state, _ := loadState()
	groups, _ := loadGroups()

	newActiveUnlocks := []UnlockState{}
	for _, unlock := range state.ActiveUnlocks {
		if !(unlock.Type == "url" && unlock.Target == req.Target) {
			newActiveUnlocks = append(newActiveUnlocks, unlock)
		}
	}
	state.ActiveUnlocks = newActiveUnlocks

	if err := saveState(state); err != nil {
		http.Error(w, "Failed to save state", http.StatusInternalServerError)
		return
	}

	remainingBlocks := calculateBlocklist(groups, state.ActiveUnlocks)
	err := syncHostsFile(remainingBlocks)
	if err != nil {
		http.Error(w, "Failed to update hosts file", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("URL '%s' locked\n", req.Target)))
}

type GroupRequest struct {
	GroupName string   `json:"group_name"`
	URLs      []string `json:"urls"`
}

func handleAddGroup(w http.ResponseWriter, r *http.Request) {
	var req GroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.GroupName == "" {
		http.Error(w, "Group name is required", http.StatusBadRequest)
		return
	}

	groups, err := loadGroups()
	if err != nil {
		groups = make(GroupMap)
	}

	groups[req.GroupName] = req.URLs

	data, err := json.MarshalIndent(groups, "", "  ")
	if err != nil {
		http.Error(w, "JSON Marshal failed", http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(GROUPS_JSON, data, 0644); err != nil {
		http.Error(w, "Failed to save groups", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("Group '%s' created with %d URLs\n", req.GroupName, len(req.URLs))))
}

type AddURLRequest struct {
	GroupName string `json:"group_name"`
	URL       string `json:"url"`
}

func handleAddURLToGroup(w http.ResponseWriter, r *http.Request) {
	var req AddURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.GroupName == "" || req.URL == "" {
		http.Error(w, "Group name and URL are required", http.StatusBadRequest)
		return
	}

	groups, err := loadGroups()
	if err != nil {
		groups = make(GroupMap)
	}

	if _, exists := groups[req.GroupName]; !exists {
		groups[req.GroupName] = []string{}
	}

	groups[req.GroupName] = append(groups[req.GroupName], req.URL)

	data, err := json.MarshalIndent(groups, "", "  ")
	if err != nil {
		http.Error(w, "JSON Marshal failed", http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(GROUPS_JSON, data, 0644); err != nil {
		http.Error(w, "Failed to save groups", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("URL '%s' added to group '%s'\n", req.URL, req.GroupName)))
}
