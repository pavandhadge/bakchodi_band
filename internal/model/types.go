package model

import "time"

const (
	DefaultDailyBudgetMinutes = 125
	DefaultUnlockMinutes      = 25
	DefaultBreakGlassMinutes  = 5
	MaxAuditEvents = 1000
)

type GroupMap map[string][]string

type UnlockState struct {
	Target string    `json:"target"`
	Type   string    `json:"type"`
	Expiry time.Time `json:"expiry"`
}

type StateJSON struct {
	ActiveUnlocks        []UnlockState  `json:"active_unlocks"`
	DailyBudgetMinutes   int            `json:"daily_budget_minutes"`
	UnlockMinutes        int            `json:"unlock_minutes"`
	BreakGlassMinutes    int            `json:"break_glass_minutes"`
	AdvancedProtection    bool           `json:"advanced_protection"`
	UsedBudgetByDate     map[string]int `json:"used_budget_by_date"`
	CommitmentUntil      time.Time      `json:"commitment_until"`
	UnlockAttemptsByDate map[string]int `json:"unlock_attempts_by_date"`
	AuditEvents          []AuditEvent   `json:"audit_events"`
}

type AuditEvent struct {
	Time       time.Time `json:"time"`
	Action     string    `json:"action"`
	Target     string    `json:"target"`
	Type       string    `json:"type"`
	Reason     string    `json:"reason,omitempty"`
	Allowed    bool      `json:"allowed"`
	BreakGlass bool      `json:"break_glass"`
	Message    string    `json:"message,omitempty"`
}

type UnlockRequest struct {
	Target     string `json:"target"`
	Reason     string `json:"reason"`
	BreakGlass bool   `json:"break_glass"`
}

type CommitRequest struct {
	Hours  int    `json:"hours"`
	Reason string `json:"reason"`
}

type FrictionPolicy struct {
	AttemptsToday int `json:"attempts_today"`
	ExtraWait     int `json:"extra_wait"`
	Challenges    int `json:"challenges"`
}

type PolicyConfig struct {
	DailyBudgetMinutes int  `json:"daily_budget_minutes"`
	UnlockMinutes      int  `json:"unlock_minutes"`
	BreakGlassMinutes  int  `json:"break_glass_minutes"`
	AdvancedProtection  bool `json:"advanced_protection"`
}

type ConfigFile struct {
	Policy PolicyConfig `json:"policy"`
	Groups GroupMap     `json:"groups"`
}

type GroupRequest struct {
	GroupName string   `json:"group_name"`
	URLs      []string `json:"urls"`
}

type DeleteGroupRequest struct {
	GroupName string `json:"group_name"`
}

type RenameGroupRequest struct {
	OldName string `json:"old_name"`
	NewName string `json:"new_name"`
}

type GroupsImportRequest struct {
	Groups GroupMap `json:"groups"`
	Merge  bool     `json:"merge"`
}


