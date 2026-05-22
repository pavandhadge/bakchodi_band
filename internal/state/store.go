package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pavandhadge/dopamine_blocker/internal/model"
)

type Store struct {
	StatePath  string
	GroupsPath string
}

func New(statePath, groupsPath string) Store {
	return Store{StatePath: statePath, GroupsPath: groupsPath}
}

func (s Store) EnsureDirs() error {
	for _, path := range []string{s.StatePath, s.GroupsPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
	}
	if _, err := os.Stat(s.GroupsPath); errors.Is(err, os.ErrNotExist) {
		if err := s.SaveGroups(DefaultGroups()); err != nil {
			return err
		}
	}
	if _, err := os.Stat(s.StatePath); errors.Is(err, os.ErrNotExist) {
		if err := s.SaveState(defaultState()); err != nil {
			return err
		}
	}
	return nil
}

func (s Store) LoadState() (model.StateJSON, error) {
	data, err := readFileSafe(s.StatePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultState(), nil
		}
		backupPath := s.StatePath + ".bak"
		if backupData, berr := os.ReadFile(backupPath); berr == nil {
			var state model.StateJSON
			if berr := json.Unmarshal(backupData, &state); berr == nil {
				return normalizeState(state), nil
			}
		}
		return defaultState(), nil
	}

	var state model.StateJSON
	if err := json.Unmarshal(data, &state); err != nil {
		return model.StateJSON{}, fmt.Errorf("corrupt state file, check %s.bak: %w", s.StatePath, err)
	}
	return normalizeState(state), nil
}

func (s Store) SaveState(state model.StateJSON) error {
	if err := os.MkdirAll(filepath.Dir(s.StatePath), 0755); err != nil {
		return err
	}

	state.AuditEvents = capAuditLog(state.AuditEvents)

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	if err := backupFile(s.StatePath); err != nil {
		return err
	}
	return writeFileAtomic(s.StatePath, data, 0644)
}

func (s Store) LoadGroups() (model.GroupMap, error) {
	data, err := readFileSafe(s.GroupsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make(model.GroupMap), err
		}
		backupPath := s.GroupsPath + ".bak"
		if backupData, berr := os.ReadFile(backupPath); berr == nil {
			var groups model.GroupMap
			if berr := json.Unmarshal(backupData, &groups); berr == nil {
				return groups, nil
			}
		}
		return make(model.GroupMap), err
	}

	var groups model.GroupMap
	if err := json.Unmarshal(data, &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

func (s Store) SaveGroups(groups model.GroupMap) error {
	if err := os.MkdirAll(filepath.Dir(s.GroupsPath), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(groups, "", "  ")
	if err != nil {
		return err
	}
	if err := backupFile(s.GroupsPath); err != nil {
		return err
	}
	return writeFileAtomic(s.GroupsPath, data, 0644)
}

func defaultState() model.StateJSON {
	return normalizeState(model.StateJSON{})
}

func normalizeState(st model.StateJSON) model.StateJSON {
	if st.ActiveUnlocks == nil {
		st.ActiveUnlocks = []model.UnlockState{}
	}
	if st.DailyBudgetMinutes == 0 {
		st.DailyBudgetMinutes = model.DefaultDailyBudgetMinutes
	}
	if st.UnlockMinutes == 0 {
		st.UnlockMinutes = model.DefaultUnlockMinutes
	}
	if st.BreakGlassMinutes == 0 {
		st.BreakGlassMinutes = model.DefaultBreakGlassMinutes
	}
	if !st.AdvancedProtection {
		st.AdvancedProtection = true
	}
	if st.UsedBudgetByDate == nil {
		st.UsedBudgetByDate = make(map[string]int)
	}
	if st.UnlockAttemptsByDate == nil {
		st.UnlockAttemptsByDate = make(map[string]int)
	}
	if st.AuditEvents == nil {
		st.AuditEvents = []model.AuditEvent{}
	}
	st.AuditEvents = capAuditLog(st.AuditEvents)
	return st
}

func DefaultGroups() model.GroupMap {
	return model.GroupMap{
		"social": {
			"x.com",
			"instagram.com",
			"facebook.com",
			"reddit.com",
		},
		"video": {
			"youtube.com",
			"netflix.com",
			"tiktok.com",
			"twitch.tv",
		},
	}
}

func capAuditLog(events []model.AuditEvent) []model.AuditEvent {
	if len(events) > model.MaxAuditEvents {
		return events[len(events)-model.MaxAuditEvents:]
	}
	return events
}

func backupFile(path string) error {
	existing, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return os.WriteFile(path+".bak", existing, 0644)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

func readFileSafe(path string) ([]byte, error) {
	return os.ReadFile(path)
}
