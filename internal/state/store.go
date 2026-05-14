package state

import (
	"encoding/json"
	"errors"
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
	data, err := os.ReadFile(s.StatePath)
	if err != nil {
		return defaultState(), nil
	}

	var state model.StateJSON
	if err := json.Unmarshal(data, &state); err != nil {
		return model.StateJSON{}, err
	}
	return normalizeState(state), nil
}

func (s Store) SaveState(state model.StateJSON) error {
	if err := os.MkdirAll(filepath.Dir(s.StatePath), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.StatePath, data, 0644)
}

func (s Store) LoadGroups() (model.GroupMap, error) {
	data, err := os.ReadFile(s.GroupsPath)
	if err != nil {
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
	return os.WriteFile(s.GroupsPath, data, 0644)
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
	if st.UsedBudgetByDate == nil {
		st.UsedBudgetByDate = make(map[string]int)
	}
	if st.UnlockAttemptsByDate == nil {
		st.UnlockAttemptsByDate = make(map[string]int)
	}
	if st.AuditEvents == nil {
		st.AuditEvents = []model.AuditEvent{}
	}
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
