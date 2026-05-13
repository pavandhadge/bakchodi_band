package main 

import "time"

type GroupMap map[string][]string


type UnlockState struct {
    Target string    `json:"target"`
    Type   string    `json:"type"` // "group", "url", or "all"
    Expiry time.Time `json:"expiry"`
}

type StateJSON struct {
	ActiveUnlocks []UnlockState `json:"active_unlocks"`
}