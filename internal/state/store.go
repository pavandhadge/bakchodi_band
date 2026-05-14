package state

import (
	"encoding/json"
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
	return nil
}

func (s Store) LoadState() (model.StateJSON, error) {
	data, err := os.ReadFile(s.StatePath)
	if err != nil {
		return model.StateJSON{ActiveUnlocks: []model.UnlockState{}}, nil
	}

	var state model.StateJSON
	if err := json.Unmarshal(data, &state); err != nil {
		return model.StateJSON{}, err
	}
	return state, nil
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
