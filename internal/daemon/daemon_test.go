package daemon

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pavandhadge/dopamine_blocker/internal/model"
	"github.com/pavandhadge/dopamine_blocker/internal/platform"
	"github.com/pavandhadge/dopamine_blocker/internal/state"
)

func TestAuthorizeUnlockDeniesWhenDailyBudgetIsUsed(t *testing.T) {
	tmp := t.TempDir()
	store := state.New(filepath.Join(tmp, "state.json"), filepath.Join(tmp, "groups.json"))
	today := time.Now().Format("2006-01-02")
	if err := store.SaveState(model.StateJSON{
		DailyBudgetMinutes: model.DefaultDailyBudgetMinutes,
		UnlockMinutes:      model.DefaultUnlockMinutes,
		UsedBudgetByDate: map[string]int{
			today: model.DefaultDailyBudgetMinutes,
		},
	}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	d := Daemon{
		Config: platform.Config{HostPath: filepath.Join(tmp, "hosts")},
		Store:  store,
	}
	recorder := httptest.NewRecorder()
	request := model.UnlockRequest{
		Target: "youtube.com",
		Reason: "testing budget denial",
	}

	_, ok := d.authorizeUnlock(recorder, "url", request.Target, request)
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
