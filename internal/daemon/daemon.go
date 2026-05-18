package daemon

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/pavandhadge/dopamine_blocker/internal/hosts"
	"github.com/pavandhadge/dopamine_blocker/internal/model"
	"github.com/pavandhadge/dopamine_blocker/internal/platform"
	"github.com/pavandhadge/dopamine_blocker/internal/state"
)

type Daemon struct {
	Config platform.Config
	Store  state.Store
}

func New(cfg platform.Config) Daemon {
	return Daemon{
		Config: cfg,
		Store:  state.New(cfg.StatePath, cfg.GroupsPath),
	}
}

func (d Daemon) Run() error {
	if !platform.IsAdmin() {
		if runtime.GOOS == "windows" {
			return fmt.Errorf("please run this terminal as Administrator")
		}
		return fmt.Errorf("this daemon must be run with sudo")
	}

	if err := d.Store.EnsureDirs(); err != nil {
		return fmt.Errorf("prepare data directories: %w", err)
	}

	if d.Config.UsesUnixSocket() {
		_ = os.Remove(d.Config.SocketAddress)
	}

	listener, err := net.Listen(d.Config.SocketNetwork, d.Config.SocketAddress)
	if err != nil {
		return fmt.Errorf("listen on %s %s: %w", d.Config.SocketNetwork, d.Config.SocketAddress, err)
	}
	defer listener.Close()

	if d.Config.UsesUnixSocket() {
		_ = os.Chmod(d.Config.SocketAddress, 0666)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/unlock", d.handleFullUnlock)
	mux.HandleFunc("/unlock-group", d.handleGroupUnlock)
	mux.HandleFunc("/unlock-url", d.handleURLUnlock)
	mux.HandleFunc("/lock", d.handleFullLock)
	mux.HandleFunc("/lock-group", d.handleGroupLock)
	mux.HandleFunc("/lock-url", d.handleURLLock)
	mux.HandleFunc("/add-group", d.handleAddGroup)
	mux.HandleFunc("/add-url-to-group", d.handleAddURLToGroup)
	mux.HandleFunc("/import-groups", d.handleImportGroups)
	mux.HandleFunc("/import-config", d.handleImportConfig)
	mux.HandleFunc("/commit", d.handleCommit)
	mux.HandleFunc("/friction", d.handleFriction)
	mux.HandleFunc("/state", d.handleState)
	mux.HandleFunc("/groups", d.handleGroups)

	go d.runExpiryChecker()

	fmt.Println("Daemon listening on", d.Config.SocketAddress)
	return http.Serve(listener, mux)
}

func (d Daemon) runExpiryChecker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		d.cleanupExpiredUnlocks()
	}
}

func (d Daemon) cleanupExpiredUnlocks() {
	currentState, err := d.Store.LoadState()
	if err != nil || len(currentState.ActiveUnlocks) == 0 {
		return
	}

	groups, _ := d.Store.LoadGroups()
	now := time.Now()
	changed := false
	validUnlocks := make([]model.UnlockState, 0, len(currentState.ActiveUnlocks))
	for _, unlock := range currentState.ActiveUnlocks {
		if now.Before(unlock.Expiry) {
			validUnlocks = append(validUnlocks, unlock)
			continue
		}
		changed = true
		fmt.Printf("Expired: %s (%s)\n", unlock.Target, unlock.Type)
	}

	if changed {
		currentState.ActiveUnlocks = validUnlocks
		_ = d.Store.SaveState(currentState)
		_ = hosts.Sync(d.Config.HostPath, calculateBlocklist(groups, currentState.ActiveUnlocks))
		fmt.Println("Expired unlocks cleaned up and sites re-blocked")
	}
}

func calculateBlocklist(allGroups model.GroupMap, activeUnlocks []model.UnlockState) []string {
	for _, unlock := range activeUnlocks {
		if unlock.Type == "all" && time.Now().Before(unlock.Expiry) {
			return []string{}
		}
	}

	toBlock := make(map[string]bool)
	for _, urls := range allGroups {
		for _, url := range urls {
			toBlock[url] = true
		}
	}

	for _, unlock := range activeUnlocks {
		if time.Now().After(unlock.Expiry) {
			continue
		}
		switch unlock.Type {
		case "group":
			for _, url := range allGroups[unlock.Target] {
				delete(toBlock, url)
			}
		case "url":
			delete(toBlock, unlock.Target)
		}
	}

	finalList := make([]string, 0, len(toBlock))
	for url := range toBlock {
		finalList = append(finalList, url)
	}
	return finalList
}

func (d Daemon) handleFullUnlock(w http.ResponseWriter, r *http.Request) {
	var req model.UnlockRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	currentState, ok := d.authorizeUnlock(w, "all", "all", req)
	if !ok {
		return
	}

	duration := currentState.UnlockMinutes
	if req.BreakGlass {
		duration = currentState.BreakGlassMinutes
	}
	expiryTime := time.Now().Add(time.Duration(duration) * time.Minute)
	currentState.ActiveUnlocks = []model.UnlockState{{
		Target: "all",
		Type:   "all",
		Expiry: expiryTime,
	}}
	d.recordAllowedUnlock(&currentState, "all", "all", req, duration)

	if err := d.Store.SaveState(currentState); err != nil {
		http.Error(w, "Failed to save state", http.StatusInternalServerError)
		return
	}
	if err := hosts.Sync(d.Config.HostPath, []string{}); err != nil {
		http.Error(w, "Failed to update hosts file", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "Global unlock activated until %s\n", expiryTime.Format("15:04:05"))
}

func (d Daemon) handleGroupUnlock(w http.ResponseWriter, r *http.Request) {
	d.handleScopedUnlock(w, r, "group")
}

func (d Daemon) handleURLUnlock(w http.ResponseWriter, r *http.Request) {
	d.handleScopedUnlock(w, r, "url")
}

func (d Daemon) handleScopedUnlock(w http.ResponseWriter, r *http.Request, unlockType string) {
	var req model.UnlockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	currentState, ok := d.authorizeUnlock(w, unlockType, req.Target, req)
	if !ok {
		return
	}
	groups, _ := d.Store.LoadGroups()
	duration := currentState.UnlockMinutes
	if req.BreakGlass {
		duration = currentState.BreakGlassMinutes
	}
	expiryTime := time.Now().Add(time.Duration(duration) * time.Minute)
	newUnlock := model.UnlockState{Target: req.Target, Type: unlockType, Expiry: expiryTime}

	updated := false
	for i, existing := range currentState.ActiveUnlocks {
		if existing.Target == req.Target && existing.Type == unlockType {
			currentState.ActiveUnlocks[i] = newUnlock
			updated = true
			break
		}
	}
	if !updated {
		currentState.ActiveUnlocks = append(currentState.ActiveUnlocks, newUnlock)
	}
	d.recordAllowedUnlock(&currentState, unlockType, req.Target, req, duration)

	if err := d.Store.SaveState(currentState); err != nil {
		http.Error(w, "Failed to save state", http.StatusInternalServerError)
		return
	}
	if err := hosts.Sync(d.Config.HostPath, calculateBlocklist(groups, currentState.ActiveUnlocks)); err != nil {
		http.Error(w, "Failed to update hosts file", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s '%s' unlocked until %s\n", unlockType, req.Target, expiryTime.Format("15:04:05"))
}

func (d Daemon) handleFullLock(w http.ResponseWriter, r *http.Request) {
	if err := d.Store.SaveState(model.StateJSON{ActiveUnlocks: []model.UnlockState{}}); err != nil {
		http.Error(w, "Failed to save state", http.StatusInternalServerError)
		return
	}

	groups, _ := d.Store.LoadGroups()
	allURLs := []string{}
	for _, urls := range groups {
		allURLs = append(allURLs, urls...)
	}

	if err := hosts.Sync(d.Config.HostPath, allURLs); err != nil {
		http.Error(w, "Failed to update hosts file", http.StatusInternalServerError)
		return
	}
	fmt.Fprintln(w, "All sites locked")
}

func (d Daemon) handleGroupLock(w http.ResponseWriter, r *http.Request) {
	d.handleScopedLock(w, r, "group")
}

func (d Daemon) handleURLLock(w http.ResponseWriter, r *http.Request) {
	d.handleScopedLock(w, r, "url")
}

func (d Daemon) handleScopedLock(w http.ResponseWriter, r *http.Request, unlockType string) {
	var req model.UnlockState
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	currentState, _ := d.Store.LoadState()
	groups, _ := d.Store.LoadGroups()
	newActiveUnlocks := []model.UnlockState{}
	for _, unlock := range currentState.ActiveUnlocks {
		if !(unlock.Type == unlockType && unlock.Target == req.Target) {
			newActiveUnlocks = append(newActiveUnlocks, unlock)
		}
	}
	currentState.ActiveUnlocks = newActiveUnlocks

	if err := d.Store.SaveState(currentState); err != nil {
		http.Error(w, "Failed to save state", http.StatusInternalServerError)
		return
	}
	if err := hosts.Sync(d.Config.HostPath, calculateBlocklist(groups, currentState.ActiveUnlocks)); err != nil {
		http.Error(w, "Failed to update hosts file", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s '%s' locked\n", unlockType, req.Target)
}

func (d Daemon) handleAddGroup(w http.ResponseWriter, r *http.Request) {
	var req model.GroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.GroupName == "" {
		http.Error(w, "Group name is required", http.StatusBadRequest)
		return
	}

	groups, err := d.Store.LoadGroups()
	if err != nil {
		groups = make(model.GroupMap)
	}
	groups[req.GroupName] = req.URLs

	if err := d.Store.SaveGroups(groups); err != nil {
		http.Error(w, "Failed to save groups", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "Group '%s' created with %d URLs\n", req.GroupName, len(req.URLs))
}

type addURLRequest struct {
	GroupName string `json:"group_name"`
	URL       string `json:"url"`
}

func (d Daemon) handleAddURLToGroup(w http.ResponseWriter, r *http.Request) {
	var req addURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.GroupName == "" || req.URL == "" {
		http.Error(w, "Group name and URL are required", http.StatusBadRequest)
		return
	}

	groups, err := d.Store.LoadGroups()
	if err != nil {
		groups = make(model.GroupMap)
	}
	groups[req.GroupName] = append(groups[req.GroupName], req.URL)

	if err := d.Store.SaveGroups(groups); err != nil {
		http.Error(w, "Failed to save groups", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "URL '%s' added to group '%s'\n", req.URL, req.GroupName)
}

func (d Daemon) handleImportGroups(w http.ResponseWriter, r *http.Request) {
	var req model.GroupsImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Groups == nil {
		http.Error(w, "groups are required", http.StatusBadRequest)
		return
	}

	groups := req.Groups
	if req.Merge {
		existing, err := d.Store.LoadGroups()
		if err == nil {
			for name, urls := range req.Groups {
				existing[name] = urls
			}
			groups = existing
		}
	}

	if err := d.Store.SaveGroups(groups); err != nil {
		http.Error(w, "Failed to save groups", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "Imported %d groups\n", len(req.Groups))
}

func (d Daemon) handleImportConfig(w http.ResponseWriter, r *http.Request) {
	var cfg model.ConfigFile
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if cfg.Groups != nil {
		if err := d.Store.SaveGroups(cfg.Groups); err != nil {
			http.Error(w, "Failed to save groups", http.StatusInternalServerError)
			return
		}
	}

	currentState, _ := d.Store.LoadState()
	if cfg.Policy.DailyBudgetMinutes > 0 {
		currentState.DailyBudgetMinutes = cfg.Policy.DailyBudgetMinutes
	}
	if cfg.Policy.UnlockMinutes > 0 {
		currentState.UnlockMinutes = cfg.Policy.UnlockMinutes
	}
	if cfg.Policy.BreakGlassMinutes > 0 {
		currentState.BreakGlassMinutes = cfg.Policy.BreakGlassMinutes
	}
	if err := d.Store.SaveState(currentState); err != nil {
		http.Error(w, "Failed to save state", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "Imported config with %d groups\n", len(cfg.Groups))
}

func (d Daemon) authorizeUnlock(w http.ResponseWriter, unlockType, target string, req model.UnlockRequest) (model.StateJSON, bool) {
	currentState, _ := d.Store.LoadState()
	today := time.Now().Format("2006-01-02")
	currentState.UnlockAttemptsByDate[today]++

	if req.Reason == "" {
		d.recordDeniedUnlock(&currentState, unlockType, target, req, "reason is required")
		_ = d.Store.SaveState(currentState)
		http.Error(w, "Reason is required", http.StatusBadRequest)
		return currentState, false
	}

	if req.BreakGlass {
		return currentState, true
	}

	if time.Now().Before(currentState.CommitmentUntil) {
		msg := fmt.Sprintf("commitment active until %s", currentState.CommitmentUntil.Format(time.RFC3339))
		d.recordDeniedUnlock(&currentState, unlockType, target, req, msg)
		_ = d.Store.SaveState(currentState)
		http.Error(w, msg, http.StatusLocked)
		return currentState, false
	}

	used := currentState.UsedBudgetByDate[today]
	if used+currentState.UnlockMinutes > currentState.DailyBudgetMinutes {
		msg := fmt.Sprintf("daily unlock budget exceeded: used %d/%d minutes", used, currentState.DailyBudgetMinutes)
		d.recordDeniedUnlock(&currentState, unlockType, target, req, msg)
		_ = d.Store.SaveState(currentState)
		http.Error(w, msg, http.StatusTooManyRequests)
		return currentState, false
	}

	return currentState, true
}

func (d Daemon) recordAllowedUnlock(st *model.StateJSON, unlockType, target string, req model.UnlockRequest, minutes int) {
	today := time.Now().Format("2006-01-02")
	if !req.BreakGlass {
		st.UsedBudgetByDate[today] += minutes
	}
	st.AuditEvents = append(st.AuditEvents, model.AuditEvent{
		Time:       time.Now(),
		Action:     "unlock",
		Target:     target,
		Type:       unlockType,
		Reason:     req.Reason,
		Allowed:    true,
		BreakGlass: req.BreakGlass,
		Message:    fmt.Sprintf("%d minute unlock", minutes),
	})
}

func (d Daemon) recordDeniedUnlock(st *model.StateJSON, unlockType, target string, req model.UnlockRequest, message string) {
	st.AuditEvents = append(st.AuditEvents, model.AuditEvent{
		Time:       time.Now(),
		Action:     "unlock",
		Target:     target,
		Type:       unlockType,
		Reason:     req.Reason,
		Allowed:    false,
		BreakGlass: req.BreakGlass,
		Message:    message,
	})
}

func (d Daemon) handleCommit(w http.ResponseWriter, r *http.Request) {
	var req model.CommitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Hours <= 0 {
		http.Error(w, "commit hours must be greater than zero", http.StatusBadRequest)
		return
	}
	if req.Reason == "" {
		http.Error(w, "reason is required", http.StatusBadRequest)
		return
	}

	currentState, _ := d.Store.LoadState()
	currentState.CommitmentUntil = time.Now().Add(time.Duration(req.Hours) * time.Hour)
	currentState.AuditEvents = append(currentState.AuditEvents, model.AuditEvent{
		Time:    time.Now(),
		Action:  "commit",
		Target:  "all",
		Type:    "commitment",
		Reason:  req.Reason,
		Allowed: true,
		Message: fmt.Sprintf("commitment active until %s", currentState.CommitmentUntil.Format(time.RFC3339)),
	})
	if err := d.Store.SaveState(currentState); err != nil {
		http.Error(w, "Failed to save state", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "Commitment active until %s\n", currentState.CommitmentUntil.Format(time.RFC3339))
}

func (d Daemon) handleFriction(w http.ResponseWriter, r *http.Request) {
	currentState, _ := d.Store.LoadState()
	attempts := currentState.UnlockAttemptsByDate[time.Now().Format("2006-01-02")]
	policy := model.FrictionPolicy{
		AttemptsToday: attempts,
		ExtraWait:     attempts * 60,
		Challenges:    3 + attempts,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(policy)
}

func (d Daemon) handleState(w http.ResponseWriter, r *http.Request) {
	currentState, _ := d.Store.LoadState()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(currentState)
}

func (d Daemon) handleGroups(w http.ResponseWriter, r *http.Request) {
	groups, _ := d.Store.LoadGroups()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(groups)
}
