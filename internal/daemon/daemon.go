package daemon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/pavandhadge/dopamine_blocker/internal/hosts"
	"github.com/pavandhadge/dopamine_blocker/internal/sniproxy"
	"github.com/pavandhadge/dopamine_blocker/internal/model"
	"github.com/pavandhadge/dopamine_blocker/internal/platform"
	"github.com/pavandhadge/dopamine_blocker/internal/state"
)

type Daemon struct {
	Config    platform.Config
	Store     state.Store
	mu        sync.Mutex
	token     string
	sniProxy  *sniproxy.Proxy
	sniActive bool
}

func New(cfg platform.Config) *Daemon {
	return &Daemon{
		Config:   cfg,
		Store:    state.New(cfg.StatePath, cfg.GroupsPath),
		sniProxy: sniproxy.New(8443),
	}
}

func (d *Daemon) Run() error {
	if !platform.IsAdmin() {
		if runtime.GOOS == "windows" {
			return fmt.Errorf("please run this terminal as Administrator")
		}
		return fmt.Errorf("this daemon must be run with sudo")
	}

	if err := d.Store.EnsureDirs(); err != nil {
		return fmt.Errorf("prepare data directories: %w", err)
	}

	if err := d.ensureAuthToken(); err != nil {
		return fmt.Errorf("setup auth token: %w", err)
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
	mux.HandleFunc("/unlock", d.requireAuth(d.handleFullUnlock))
	mux.HandleFunc("/unlock-group", d.requireAuth(d.handleGroupUnlock))
	mux.HandleFunc("/unlock-url", d.requireAuth(d.handleURLUnlock))
	mux.HandleFunc("/lock", d.requireAuth(d.handleFullLock))
	mux.HandleFunc("/lock-group", d.requireAuth(d.handleGroupLock))
	mux.HandleFunc("/lock-url", d.requireAuth(d.handleURLLock))
	mux.HandleFunc("/add-group", d.requireAuth(d.handleAddGroup))
	mux.HandleFunc("/add-url-to-group", d.requireAuth(d.handleAddURLToGroup))
	mux.HandleFunc("/delete-group", d.requireAuth(d.handleDeleteGroup))
	mux.HandleFunc("/rename-group", d.requireAuth(d.handleRenameGroup))
	mux.HandleFunc("/import-groups", d.requireAuth(d.handleImportGroups))
	mux.HandleFunc("/import-config", d.requireAuth(d.handleImportConfig))
	mux.HandleFunc("/commit", d.requireAuth(d.handleCommit))
	mux.HandleFunc("/friction", d.requireAuth(d.handleFriction))
	mux.HandleFunc("/state", d.requireAuth(d.handleState))
	mux.HandleFunc("/groups", d.requireAuth(d.handleGroups))

	go d.runExpiryChecker()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		fmt.Println("\nShutting down daemon...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	fmt.Println("Daemon listening on", d.Config.SocketAddress)
	return server.Serve(listener)
}

func (d *Daemon) ensureAuthToken() error {
	data, err := os.ReadFile(d.Config.TokenPath)
	if err == nil && len(data) >= 32 {
		d.token = stringsTrimSpace(string(data))
		return nil
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return fmt.Errorf("generate token: %w", err)
	}
	d.token = hex.EncodeToString(tokenBytes)

	if err := os.MkdirAll(d.Config.DataDir, 0700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	if err := os.WriteFile(d.Config.TokenPath, []byte(d.token), 0600); err != nil {
		return fmt.Errorf("write token: %w", err)
	}
	fmt.Println("Generated new auth token at", d.Config.TokenPath)
	return nil
}

func (d *Daemon) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		expected := "Bearer " + d.token
		if auth != expected {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (d *Daemon) runExpiryChecker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		d.mu.Lock()
		d.cleanupExpiredUnlocks()
		d.mu.Unlock()
	}
}

func (d *Daemon) cleanupExpiredUnlocks() {
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
		_ = d.syncProtection(calculateBlocklist(groups, currentState.ActiveUnlocks), currentState.AdvancedProtection)
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

func (d *Daemon) handleFullUnlock(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()

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
	if err := d.syncProtection([]string{}, currentState.AdvancedProtection); err != nil {
		http.Error(w, "Failed to update hosts file", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "Global unlock activated until %s\n", expiryTime.Format("15:04:05"))
}

func (d *Daemon) handleGroupUnlock(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handleScopedUnlock(w, r, "group")
}

func (d *Daemon) handleURLUnlock(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handleScopedUnlock(w, r, "url")
}

func (d *Daemon) handleScopedUnlock(w http.ResponseWriter, r *http.Request, unlockType string) {
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
	if err := d.syncProtection(calculateBlocklist(groups, currentState.ActiveUnlocks), currentState.AdvancedProtection); err != nil {
		http.Error(w, "Failed to update hosts file", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s '%s' unlocked until %s\n", unlockType, req.Target, expiryTime.Format("15:04:05"))
}

func (d *Daemon) handleFullLock(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()

	currentState, _ := d.Store.LoadState()
	currentState.ActiveUnlocks = []model.UnlockState{}
	currentState.AuditEvents = append(currentState.AuditEvents, model.AuditEvent{
		Time:    time.Now(),
		Action:  "lock",
		Target:  "all",
		Type:    "all",
		Reason:  "",
		Allowed: true,
		Message: "full lock executed",
	})

	if err := d.Store.SaveState(currentState); err != nil {
		http.Error(w, "Failed to save state", http.StatusInternalServerError)
		return
	}

	groups, _ := d.Store.LoadGroups()
	allURLs := []string{}
	for _, urls := range groups {
		allURLs = append(allURLs, urls...)
	}

	if err := d.syncProtection(allURLs, currentState.AdvancedProtection); err != nil {
		http.Error(w, "Failed to update hosts file", http.StatusInternalServerError)
		return
	}
	fmt.Fprintln(w, "All sites locked")
}

func (d *Daemon) handleGroupLock(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handleScopedLock(w, r, "group")
}

func (d *Daemon) handleURLLock(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handleScopedLock(w, r, "url")
}

func (d *Daemon) handleScopedLock(w http.ResponseWriter, r *http.Request, unlockType string) {
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
	if err := d.syncProtection(calculateBlocklist(groups, currentState.ActiveUnlocks), currentState.AdvancedProtection); err != nil {
		http.Error(w, "Failed to update hosts file", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s '%s' locked\n", unlockType, req.Target)
}

func (d *Daemon) handleAddGroup(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()

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

func (d *Daemon) handleAddURLToGroup(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()

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
	for _, u := range groups[req.GroupName] {
		if u == req.URL {
			fmt.Fprintf(w, "URL '%s' already in group '%s'\n", req.URL, req.GroupName)
			return
		}
	}
	groups[req.GroupName] = append(groups[req.GroupName], req.URL)

	if err := d.Store.SaveGroups(groups); err != nil {
		http.Error(w, "Failed to save groups", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "URL '%s' added to group '%s'\n", req.URL, req.GroupName)
}

func (d *Daemon) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var req model.DeleteGroupRequest
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
		http.Error(w, "No groups found", http.StatusNotFound)
		return
	}
	if _, ok := groups[req.GroupName]; !ok {
		http.Error(w, "Group not found", http.StatusNotFound)
		return
	}

	delete(groups, req.GroupName)
	if err := d.Store.SaveGroups(groups); err != nil {
		http.Error(w, "Failed to save groups", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "Group '%s' deleted\n", req.GroupName)
}

func (d *Daemon) handleRenameGroup(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var req model.RenameGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.OldName == "" || req.NewName == "" {
		http.Error(w, "Both old_name and new_name are required", http.StatusBadRequest)
		return
	}

	groups, err := d.Store.LoadGroups()
	if err != nil {
		http.Error(w, "No groups found", http.StatusNotFound)
		return
	}
	urls, ok := groups[req.OldName]
	if !ok {
		http.Error(w, "Group not found", http.StatusNotFound)
		return
	}

	delete(groups, req.OldName)
	groups[req.NewName] = urls

	if err := d.Store.SaveGroups(groups); err != nil {
		http.Error(w, "Failed to save groups", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "Group '%s' renamed to '%s'\n", req.OldName, req.NewName)
}

func (d *Daemon) handleImportGroups(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()

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

func (d *Daemon) handleImportConfig(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()

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
	currentState.AdvancedProtection = cfg.Policy.AdvancedProtection

	if err := d.Store.SaveState(currentState); err != nil {
		http.Error(w, "Failed to save state", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "Imported config with %d groups\n", len(cfg.Groups))
}

func (d *Daemon) authorizeUnlock(w http.ResponseWriter, unlockType, target string, req model.UnlockRequest) (model.StateJSON, bool) {
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

func (d *Daemon) recordAllowedUnlock(st *model.StateJSON, unlockType, target string, req model.UnlockRequest, minutes int) {
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

func (d *Daemon) recordDeniedUnlock(st *model.StateJSON, unlockType, target string, req model.UnlockRequest, message string) {
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

func (d *Daemon) handleCommit(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()

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

func (d *Daemon) handleFriction(w http.ResponseWriter, r *http.Request) {
	currentState, _ := d.Store.LoadState()
	attempts := currentState.UnlockAttemptsByDate[time.Now().Format("2006-01-02")]
	policy := model.FrictionPolicy{
		AttemptsToday: attempts,
		ExtraWait:     attempts * 60,
		Challenges:    1 + attempts,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(policy)
}

func (d *Daemon) handleState(w http.ResponseWriter, r *http.Request) {
	currentState, _ := d.Store.LoadState()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(currentState)
}

func (d *Daemon) handleGroups(w http.ResponseWriter, r *http.Request) {
	groups, _ := d.Store.LoadGroups()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(groups)
}

func (d *Daemon) syncProtection(urlsToBlock []string, advanced bool) error {
	if err := hosts.Sync(d.Config.HostPath, urlsToBlock); err != nil {
		return err
	}

	if !advanced {
		if d.sniActive {
			d.sniProxy.Close()
			redirectDel()
			d.sniActive = false
		}
		return nil
	}

	if len(urlsToBlock) > 0 {
		d.sniProxy.Block(urlsToBlock)
		d.sniProxy.Open()
		redirectAdd()
		d.sniActive = true
	} else if d.sniActive {
		d.sniProxy.Close()
		redirectDel()
		d.sniActive = false
	}
	return nil
}

func stringsTrimSpace(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r' || s[len(s)-1] == ' ') {
		s = s[:len(s)-1]
	}
	return s
}
