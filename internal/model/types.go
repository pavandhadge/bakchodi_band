package model

import "time"

const DefaultTimeLimitMinutes = 45

type GroupMap map[string][]string

type UnlockState struct {
	Target string    `json:"target"`
	Type   string    `json:"type"`
	Expiry time.Time `json:"expiry"`
}

type StateJSON struct {
	ActiveUnlocks []UnlockState `json:"active_unlocks"`
}
