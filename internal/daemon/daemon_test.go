package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pavandhadge/dopamine_blocker/internal/model"
	"github.com/pavandhadge/dopamine_blocker/internal/platform"
	"github.com/pavandhadge/dopamine_blocker/internal/state"
)

func tmpDaemon(t *testing.T) (*Daemon, *state.Store) {
	t.Helper()
	tmp := t.TempDir()
	hostsPath := filepath.Join(tmp, "hosts")
	if err := os.WriteFile(hostsPath, []byte("127.0.0.1 localhost\n"), 0644); err != nil {
		t.Fatalf("create hosts file: %v", err)
	}
	store := state.New(filepath.Join(tmp, "state.json"), filepath.Join(tmp, "groups.json"))
	return &Daemon{
		Config:   platform.Config{HostPath: hostsPath},
		Store:    store,
		token:    "test-token",
	}, &store
}

func setupState(t *testing.T, store *state.Store, st model.StateJSON) {
	t.Helper()
	if err := store.SaveState(st); err != nil {
		t.Fatalf("save state: %v", err)
	}
}

func TestAuthorizeUnlockDeniesWhenDailyBudgetIsUsed(t *testing.T) {
	d, store := tmpDaemon(t)
	today := time.Now().Format("2006-01-02")
	setupState(t, store, model.StateJSON{
		DailyBudgetMinutes: model.DefaultDailyBudgetMinutes,
		UnlockMinutes:      model.DefaultUnlockMinutes,
		UsedBudgetByDate: map[string]int{
			today: model.DefaultDailyBudgetMinutes,
		},
	})

	recorder := httptest.NewRecorder()
	_, ok := d.authorizeUnlock(recorder, "url", "youtube.com", model.UnlockRequest{
		Target: "youtube.com",
		Reason: "testing budget denial",
	})
	if ok {
		t.Fatal("expected unlock to be denied after budget is used")
	}
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status %d, got %d", http.StatusTooManyRequests, recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "daily unlock budget exceeded") {
		t.Fatalf("expected budget error, got %q", recorder.Body.String())
	}
}

func TestAuthorizeUnlockDeniesWhenNoReason(t *testing.T) {
	d, store := tmpDaemon(t)
	setupState(t, store, model.StateJSON{})

	recorder := httptest.NewRecorder()
	_, ok := d.authorizeUnlock(recorder, "url", "youtube.com", model.UnlockRequest{})
	if ok {
		t.Fatal("expected unlock to be denied without reason")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
}

func TestAuthorizeUnlockDeniesWhenCommitmentActive(t *testing.T) {
	d, store := tmpDaemon(t)
	setupState(t, store, model.StateJSON{
		CommitmentUntil: time.Now().Add(1 * time.Hour),
	})

	recorder := httptest.NewRecorder()
	_, ok := d.authorizeUnlock(recorder, "url", "youtube.com", model.UnlockRequest{
		Target: "youtube.com",
		Reason: "need break",
	})
	if ok {
		t.Fatal("expected unlock to be denied during commitment")
	}
	if recorder.Code != http.StatusLocked {
		t.Fatalf("expected status %d, got %d", http.StatusLocked, recorder.Code)
	}
}

func TestAuthorizeUnlockAllowsBreakGlass(t *testing.T) {
	d, store := tmpDaemon(t)
	setupState(t, store, model.StateJSON{
		CommitmentUntil:   time.Now().Add(1 * time.Hour),
		DailyBudgetMinutes: model.DefaultDailyBudgetMinutes,
		UnlockMinutes:      model.DefaultUnlockMinutes,
		UsedBudgetByDate: map[string]int{
			time.Now().Format("2006-01-02"): model.DefaultDailyBudgetMinutes,
		},
	})

	recorder := httptest.NewRecorder()
	_, ok := d.authorizeUnlock(recorder, "all", "all", model.UnlockRequest{
		Reason:     "emergency",
		BreakGlass: true,
	})
	if !ok {
		t.Fatal("expected break glass to bypass restrictions")
	}
}

func TestAuthorizeUnlockAllowsWithinBudget(t *testing.T) {
	d, store := tmpDaemon(t)
	setupState(t, store, model.StateJSON{
		DailyBudgetMinutes: 125,
		UnlockMinutes:      25,
		UsedBudgetByDate: map[string]int{
			time.Now().Format("2006-01-02"): 50,
		},
	})

	recorder := httptest.NewRecorder()
	_, ok := d.authorizeUnlock(recorder, "url", "youtube.com", model.UnlockRequest{
		Target: "youtube.com",
		Reason: "break time",
	})
	if !ok {
		t.Fatal("expected unlock to be allowed within budget")
	}
}

func TestCalculateBlocklistNoUnlocks(t *testing.T) {
	groups := model.GroupMap{
		"social": {"x.com", "instagram.com"},
		"video":  {"youtube.com"},
	}
	result := calculateBlocklist(groups, nil)
	if len(result) != 3 {
		t.Fatalf("expected 3 blocked urls, got %d: %v", len(result), result)
	}
}

func TestCalculateBlocklistGlobalUnlock(t *testing.T) {
	groups := model.GroupMap{
		"social": {"x.com"},
		"video":  {"youtube.com"},
	}
	unlocks := []model.UnlockState{
		{Target: "all", Type: "all", Expiry: time.Now().Add(1 * time.Hour)},
	}
	result := calculateBlocklist(groups, unlocks)
	if len(result) != 0 {
		t.Fatalf("expected empty blocklist with global unlock, got %d", len(result))
	}
}

func TestCalculateBlocklistExpiredGlobalUnlock(t *testing.T) {
	groups := model.GroupMap{
		"social": {"x.com"},
	}
	unlocks := []model.UnlockState{
		{Target: "all", Type: "all", Expiry: time.Now().Add(-1 * time.Hour)},
	}
	result := calculateBlocklist(groups, unlocks)
	if len(result) != 1 {
		t.Fatalf("expected 1 blocked url with expired unlock, got %d", len(result))
	}
}

func TestCalculateBlocklistGroupUnlock(t *testing.T) {
	groups := model.GroupMap{
		"social": {"x.com", "instagram.com"},
		"video":  {"youtube.com"},
	}
	unlocks := []model.UnlockState{
		{Target: "social", Type: "group", Expiry: time.Now().Add(1 * time.Hour)},
	}
	result := calculateBlocklist(groups, unlocks)
	if len(result) != 1 {
		t.Fatalf("expected 1 blocked url (video group), got %d: %v", len(result), result)
	}
}

func TestCalculateBlocklistURLUnlock(t *testing.T) {
	groups := model.GroupMap{
		"social": {"x.com", "instagram.com"},
	}
	unlocks := []model.UnlockState{
		{Target: "x.com", Type: "url", Expiry: time.Now().Add(1 * time.Hour)},
	}
	result := calculateBlocklist(groups, unlocks)
	if len(result) != 1 {
		t.Fatalf("expected 1 blocked url (instagram.com), got %d: %v", len(result), result)
	}
}

func TestHandleFullLockPreservesState(t *testing.T) {
	d, store := tmpDaemon(t)
	now := time.Now()
	setupState(t, store, model.StateJSON{
		DailyBudgetMinutes:   99,
		UnlockMinutes:        33,
		BreakGlassMinutes:    7,
		AdvancedProtection:    true,
		UnlockAttemptsByDate: map[string]int{"2026-01-01": 5},
		UsedBudgetByDate:     map[string]int{"2026-01-01": 50},
		CommitmentUntil:      now.Add(2 * time.Hour),
		ActiveUnlocks: []model.UnlockState{
			{Target: "social", Type: "group", Expiry: now.Add(1 * time.Hour)},
		},
		AuditEvents: []model.AuditEvent{
			{Time: now, Action: "unlock", Target: "social"},
		},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/lock", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	d.requireAuth(d.handleFullLock)(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	stateAfter, _ := store.LoadState()
	if len(stateAfter.ActiveUnlocks) != 0 {
		t.Fatalf("expected empty unlocks after full lock, got %d", len(stateAfter.ActiveUnlocks))
	}
	if stateAfter.DailyBudgetMinutes != 99 {
		t.Fatalf("budget was reset: got %d, want 99", stateAfter.DailyBudgetMinutes)
	}
	if stateAfter.UnlockMinutes != 33 {
		t.Fatalf("unlock minutes was reset: got %d, want 33", stateAfter.UnlockMinutes)
	}
	if stateAfter.BreakGlassMinutes != 7 {
		t.Fatalf("break glass minutes was reset: got %d, want 7", stateAfter.BreakGlassMinutes)
	}
	if !stateAfter.AdvancedProtection {
		t.Fatal("advanced_protection was reset")
	}
}

func TestHandleFullLockWithGroups(t *testing.T) {
	d, store := tmpDaemon(t)
	setupState(t, store, model.StateJSON{})
	if err := store.SaveGroups(model.GroupMap{
		"social": {"x.com", "instagram.com"},
	}); err != nil {
		t.Fatalf("save groups: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/lock", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	d.requireAuth(d.handleFullLock)(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAddGroupAndAddURL(t *testing.T) {
	d, store := tmpDaemon(t)
	setupState(t, store, model.StateJSON{})

	recorder := httptest.NewRecorder()
	addBody := mustJSON(t, model.GroupRequest{
		GroupName: "test-group",
		URLs:      []string{"site1.com", "site2.com"},
	})
	request := httptest.NewRequest(http.MethodPost, "/add-group", strings.NewReader(addBody))
	request.Header.Set("Authorization", "Bearer test-token")
	d.requireAuth(d.handleAddGroup)(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("add-group: expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	groups, _ := store.LoadGroups()
	if _, ok := groups["test-group"]; !ok {
		t.Fatal("test-group should exist after creation")
	}
	if len(groups["test-group"]) != 2 {
		t.Fatalf("expected 2 URLs, got %d", len(groups["test-group"]))
	}

	recorder = httptest.NewRecorder()
	addURLBody := mustJSON(t, addURLRequest{GroupName: "test-group", URL: "site3.com"})
	request = httptest.NewRequest(http.MethodPost, "/add-url-to-group", strings.NewReader(addURLBody))
	request.Header.Set("Authorization", "Bearer test-token")
	d.requireAuth(d.handleAddURLToGroup)(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("add-url-to-group: expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	groups, _ = store.LoadGroups()
	if len(groups["test-group"]) != 3 {
		t.Fatalf("expected 3 URLs after add, got %d", len(groups["test-group"]))
	}

	recorder = httptest.NewRecorder()
	addURLBody2 := mustJSON(t, addURLRequest{GroupName: "test-group", URL: "site1.com"})
	request = httptest.NewRequest(http.MethodPost, "/add-url-to-group", strings.NewReader(addURLBody2))
	request.Header.Set("Authorization", "Bearer test-token")
	d.requireAuth(d.handleAddURLToGroup)(recorder, request)

	if strings.Contains(recorder.Body.String(), "already in group") {
		if len(groups["test-group"]) != 3 {
			t.Fatalf("expected no duplicate, got %d", len(groups["test-group"]))
		}
	}
}

func TestDeleteGroup(t *testing.T) {
	d, store := tmpDaemon(t)
	setupState(t, store, model.StateJSON{})
	store.SaveGroups(model.GroupMap{
		"social": {"x.com"},
		"video":  {"youtube.com"},
	})

	recorder := httptest.NewRecorder()
	body := mustJSON(t, model.DeleteGroupRequest{GroupName: "social"})
	request := httptest.NewRequest(http.MethodPost, "/delete-group", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer test-token")
	d.requireAuth(d.handleDeleteGroup)(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("delete-group: expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	groups, _ := store.LoadGroups()
	if _, ok := groups["social"]; ok {
		t.Fatal("social group should be deleted")
	}
	if _, ok := groups["video"]; !ok {
		t.Fatal("video group should remain")
	}
}

func TestDeleteGroupNotFound(t *testing.T) {
	d, store := tmpDaemon(t)
	setupState(t, store, model.StateJSON{})
	store.SaveGroups(model.GroupMap{"social": {"x.com"}})

	recorder := httptest.NewRecorder()
	body := mustJSON(t, model.DeleteGroupRequest{GroupName: "nonexistent"})
	request := httptest.NewRequest(http.MethodPost, "/delete-group", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer test-token")
	d.requireAuth(d.handleDeleteGroup)(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", recorder.Code)
	}
}

func TestRenameGroup(t *testing.T) {
	d, store := tmpDaemon(t)
	setupState(t, store, model.StateJSON{})
	store.SaveGroups(model.GroupMap{
		"social": {"x.com", "instagram.com"},
	})

	recorder := httptest.NewRecorder()
	body := mustJSON(t, model.RenameGroupRequest{OldName: "social", NewName: "media"})
	request := httptest.NewRequest(http.MethodPost, "/rename-group", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer test-token")
	d.requireAuth(d.handleRenameGroup)(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("rename-group: expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	groups, _ := store.LoadGroups()
	if _, ok := groups["social"]; ok {
		t.Fatal("old group name should not exist")
	}
	media, ok := groups["media"]
	if !ok {
		t.Fatal("new group name should exist")
	}
	if len(media) != 2 {
		t.Fatalf("expected 2 URLs in renamed group, got %d", len(media))
	}
}

func TestImportGroups(t *testing.T) {
	d, store := tmpDaemon(t)
	setupState(t, store, model.StateJSON{})
	store.SaveGroups(model.GroupMap{"existing": {"old.com"}})

	recorder := httptest.NewRecorder()
	body := mustJSON(t, model.GroupsImportRequest{
		Groups: model.GroupMap{"new-group": {"new.com"}},
	})
	request := httptest.NewRequest(http.MethodPost, "/import-groups", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer test-token")
	d.requireAuth(d.handleImportGroups)(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("import-groups: expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	groups, _ := store.LoadGroups()
	if _, ok := groups["existing"]; ok {
		t.Fatal("existing groups should be replaced, not merged")
	}
	if _, ok := groups["new-group"]; !ok {
		t.Fatal("new-group should exist")
	}
}

func TestImportGroupsMerge(t *testing.T) {
	d, store := tmpDaemon(t)
	setupState(t, store, model.StateJSON{})
	store.SaveGroups(model.GroupMap{"existing": {"old.com"}})

	recorder := httptest.NewRecorder()
	body := mustJSON(t, model.GroupsImportRequest{
		Merge:  true,
		Groups: model.GroupMap{"new-group": {"new.com"}},
	})
	request := httptest.NewRequest(http.MethodPost, "/import-groups", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer test-token")
	d.requireAuth(d.handleImportGroups)(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("import-groups merge: expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	groups, _ := store.LoadGroups()
	if _, ok := groups["existing"]; !ok {
		t.Fatal("existing groups should be preserved when merging")
	}
	if _, ok := groups["new-group"]; !ok {
		t.Fatal("new-group should exist after merge")
	}
}

func TestImportConfig(t *testing.T) {
	d, store := tmpDaemon(t)
	setupState(t, store, model.StateJSON{})

	recorder := httptest.NewRecorder()
	body := mustJSON(t, model.ConfigFile{
		Policy: model.PolicyConfig{
			DailyBudgetMinutes: 200,
			UnlockMinutes:      30,
			BreakGlassMinutes:  10,
			AdvancedProtection:  true,
		},
		Groups: model.GroupMap{"test": {"test.com"}},
	})
	request := httptest.NewRequest(http.MethodPost, "/import-config", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer test-token")
	d.requireAuth(d.handleImportConfig)(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("import-config: expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	st, _ := store.LoadState()
	if st.DailyBudgetMinutes != 200 {
		t.Fatalf("budget: got %d, want 200", st.DailyBudgetMinutes)
	}
	if st.UnlockMinutes != 30 {
		t.Fatalf("unlock: got %d, want 30", st.UnlockMinutes)
	}
	if st.BreakGlassMinutes != 10 {
		t.Fatalf("break glass: got %d, want 10", st.BreakGlassMinutes)
	}
	if !st.AdvancedProtection {
		t.Fatal("advanced_protection should be true")
	}

	groups, _ := store.LoadGroups()
	if _, ok := groups["test"]; !ok {
		t.Fatal("test group should exist after import")
	}
}

func TestHandleGroupsEndpoint(t *testing.T) {
	d, store := tmpDaemon(t)
	setupState(t, store, model.StateJSON{})
	store.SaveGroups(model.GroupMap{"social": {"x.com"}})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/groups", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	d.requireAuth(d.handleGroups)(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("groups endpoint: expected 200, got %d", recorder.Code)
	}

	var groups model.GroupMap
	if err := json.NewDecoder(recorder.Body).Decode(&groups); err != nil {
		t.Fatalf("decode groups: %v", err)
	}
	if _, ok := groups["social"]; !ok {
		t.Fatal("social group should be in response")
	}
}

func TestHandleStateEndpoint(t *testing.T) {
	d, store := tmpDaemon(t)
	setupState(t, store, model.StateJSON{
		DailyBudgetMinutes: 100,
		UnlockMinutes:      10,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/state", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	d.requireAuth(d.handleState)(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("state endpoint: expected 200, got %d", recorder.Code)
	}

	var st model.StateJSON
	if err := json.NewDecoder(recorder.Body).Decode(&st); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if st.DailyBudgetMinutes != 100 {
		t.Fatalf("budget: got %d, want 100", st.DailyBudgetMinutes)
	}
}

func TestHandleFrictionEndpoint(t *testing.T) {
	d, store := tmpDaemon(t)
	setupState(t, store, model.StateJSON{
		UnlockAttemptsByDate: map[string]int{
			time.Now().Format("2006-01-02"): 3,
		},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/friction", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	d.requireAuth(d.handleFriction)(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("friction endpoint: expected 200, got %d", recorder.Code)
	}

	var policy model.FrictionPolicy
	if err := json.NewDecoder(recorder.Body).Decode(&policy); err != nil {
		t.Fatalf("decode friction: %v", err)
	}
	if policy.ExtraWait != 180 {
		t.Fatalf("extra wait: got %d, want 180", policy.ExtraWait)
	}
	if policy.Challenges != 4 {
		t.Fatalf("challenges: got %d, want 4", policy.Challenges)
	}
}

func TestHandleCommit(t *testing.T) {
	d, store := tmpDaemon(t)
	setupState(t, store, model.StateJSON{})

	recorder := httptest.NewRecorder()
	body := mustJSON(t, model.CommitRequest{Hours: 4, Reason: "exam prep"})
	request := httptest.NewRequest(http.MethodPost, "/commit", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer test-token")
	d.requireAuth(d.handleCommit)(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("commit: expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	st, _ := store.LoadState()
	if st.CommitmentUntil.Before(time.Now().Add(3*time.Hour)) || st.CommitmentUntil.After(time.Now().Add(5*time.Hour)) {
		t.Fatalf("commitment time out of range: %v", st.CommitmentUntil)
	}
}

func TestHandleCommitBadRequest(t *testing.T) {
	d, store := tmpDaemon(t)
	setupState(t, store, model.StateJSON{})

	recorder := httptest.NewRecorder()
	body := mustJSON(t, model.CommitRequest{Hours: 0})
	request := httptest.NewRequest(http.MethodPost, "/commit", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer test-token")
	d.requireAuth(d.handleCommit)(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for 0 hours, got %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	body = mustJSON(t, model.CommitRequest{Hours: 2})
	request = httptest.NewRequest(http.MethodPost, "/commit", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer test-token")
	d.requireAuth(d.handleCommit)(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty reason, got %d", recorder.Code)
	}
}

func TestRequireAuth(t *testing.T) {
	d, store := tmpDaemon(t)
	d.token = "secret123"
	setupState(t, store, model.StateJSON{})

	handler := d.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	handler(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("no auth: expected 401, got %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer wrong-token")
	handler(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("wrong auth: expected 401, got %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer secret123")
	handler(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("correct auth: expected 200, got %d", recorder.Code)
	}
}

func TestStringsTrimSpace(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello\n", "hello"},
		{"hello\r\n", "hello"},
		{"hello ", "hello"},
		{"hello\n\r ", "hello"},
		{"\nhello", "\nhello"},
		{"hello", "hello"},
		{"", ""},
		{"  ", ""},
	}
	for _, tt := range tests {
		got := stringsTrimSpace(tt.input)
		if got != tt.want {
			t.Fatalf("stringsTrimSpace(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCleanupExpiredUnlocks(t *testing.T) {
	d, store := tmpDaemon(t)
	now := time.Now()
	setupState(t, store, model.StateJSON{
		ActiveUnlocks: []model.UnlockState{
			{Target: "youtube.com", Type: "url", Expiry: now.Add(-1 * time.Hour)},
			{Target: "social", Type: "group", Expiry: now.Add(1 * time.Hour)},
		},
	})

	if err := store.SaveGroups(model.GroupMap{
		"social": {"x.com"},
		"video":  {"youtube.com"},
	}); err != nil {
		t.Fatalf("save groups: %v", err)
	}

	d.cleanupExpiredUnlocks()

	st, _ := store.LoadState()
	if len(st.ActiveUnlocks) != 1 {
		t.Fatalf("expected 1 active unlock after cleanup, got %d", len(st.ActiveUnlocks))
	}
	if st.ActiveUnlocks[0].Target != "social" {
		t.Fatalf("expected social to remain, got %s", st.ActiveUnlocks[0].Target)
	}
}

func TestConcurrentSafeUnlockLockCycle(t *testing.T) {
	d, store := tmpDaemon(t)
	setupState(t, store, model.StateJSON{
		DailyBudgetMinutes: 125,
		UnlockMinutes:      25,
	})
	store.SaveGroups(model.GroupMap{
		"social": {"x.com", "instagram.com"},
		"video":  {"youtube.com"},
	})

	unlockBody := mustJSON(t, model.UnlockRequest{
		Target: "youtube.com",
		Reason: "lunch break",
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/unlock-url", strings.NewReader(unlockBody))
	request.Header.Set("Authorization", "Bearer test-token")
	d.requireAuth(d.handleURLUnlock)(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unlock: expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	st, _ := store.LoadState()
	if len(st.ActiveUnlocks) != 1 {
		t.Fatalf("expected 1 active unlock, got %d", len(st.ActiveUnlocks))
	}

	lockBody := mustJSON(t, model.UnlockState{Target: "youtube.com", Type: "url"})
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/lock-url", strings.NewReader(lockBody))
	request.Header.Set("Authorization", "Bearer test-token")
	d.requireAuth(d.handleURLLock)(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("lock: expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	st, _ = store.LoadState()
	if len(st.ActiveUnlocks) != 0 {
		t.Fatalf("expected 0 active unlocks after lock, got %d", len(st.ActiveUnlocks))
	}
}

func TestGlobalLockThenUnlock(t *testing.T) {
	d, store := tmpDaemon(t)
	setupState(t, store, model.StateJSON{
		DailyBudgetMinutes: 125,
		UnlockMinutes:      25,
	})
	store.SaveGroups(model.GroupMap{
		"social": {"x.com"},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/lock", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	d.requireAuth(d.handleFullLock)(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("global lock: expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	unlockBody := mustJSON(t, model.UnlockRequest{Reason: "need break"})
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/unlock", strings.NewReader(unlockBody))
	request.Header.Set("Authorization", "Bearer test-token")
	d.requireAuth(d.handleFullUnlock)(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("global unlock: expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	st, _ := store.LoadState()
	if len(st.ActiveUnlocks) != 1 {
		t.Fatalf("expected 1 global unlock, got %d", len(st.ActiveUnlocks))
	}
	if st.ActiveUnlocks[0].Type != "all" {
		t.Fatalf("expected type 'all', got %q", st.ActiveUnlocks[0].Type)
	}
}

func TestGroupLockAndUnlockFlow(t *testing.T) {
	d, store := tmpDaemon(t)
	setupState(t, store, model.StateJSON{
		DailyBudgetMinutes: 125,
		UnlockMinutes:      25,
	})
	store.SaveGroups(model.GroupMap{
		"social": {"x.com", "instagram.com"},
		"video":  {"youtube.com"},
	})

	unlockBody := mustJSON(t, model.UnlockRequest{
		Target: "social",
		Reason: "break",
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/unlock-group", strings.NewReader(unlockBody))
	request.Header.Set("Authorization", "Bearer test-token")
	d.requireAuth(d.handleGroupUnlock)(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("group unlock: expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	lockBody := mustJSON(t, model.UnlockState{Target: "social", Type: "group"})
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/lock-group", strings.NewReader(lockBody))
	request.Header.Set("Authorization", "Bearer test-token")
	d.requireAuth(d.handleGroupLock)(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("group lock: expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(data)
}
