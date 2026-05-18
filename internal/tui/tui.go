package tui

import (
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pavandhadge/dopamine_blocker/internal/model"
	"github.com/pavandhadge/dopamine_blocker/internal/platform"
)

type commandKind int

const (
	commandBlock commandKind = iota
	commandAllow
	commandPanic
	commandPlan
)

type targetKind int

const (
	targetURL targetKind = iota
	targetGroup
	targetAll
)

type fieldKind int

const (
	fieldCommand fieldKind = iota
	fieldTarget
	fieldValue
	fieldHours
	fieldReason
	fieldSubmit
)

type modeKind int

const (
	modeForm modeKind = iota
	modeWait
	modeChallenge
	modeReason
)

type waitTickMsg time.Time

var (
	pageStyle  = lipgloss.NewStyle().Padding(1, 2)
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#E11D48"))
	mutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	labelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#D1D5DB"))
	activeStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#111827")).
			Background(lipgloss.Color("#FBBF24")).
			Padding(0, 1)
	goodStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#16A34A")).Bold(true)
	badStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#DC2626")).Bold(true)
	warnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#D97706")).Bold(true)
)

type Model struct {
	cfg platform.Config

	width int

	command commandKind
	target  targetKind
	cursor  int

	valueInput  textinput.Model
	reasonInput textinput.Model
	hours       int

	state  model.StateJSON
	groups model.GroupMap
	policy model.FrictionPolicy
	loaded bool

	mode    modeKind
	working bool
	message string
	err     bool

	pending pendingAction

	waitRemaining int
	required      int
	solved        int
	mistakes      int
	question      string
	answer        int
	answerInput   textinput.Model
}

func InitialModel(cfg platform.Config) Model {
	value := textinput.New()
	value.Placeholder = "youtube.com"
	value.CharLimit = 180
	value.Width = 34

	reason := textinput.New()
	reason.Placeholder = "reason"
	reason.CharLimit = 200
	reason.Width = 44

	answer := textinput.New()
	answer.Placeholder = "answer"
	answer.CharLimit = 20
	answer.Width = 16

	m := Model{
		cfg:         cfg,
		command:     commandBlock,
		target:      targetURL,
		valueInput:  value,
		reasonInput: reason,
		answerInput: answer,
		hours:       24,
	}
	m.focusCurrentInput()
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(loadState(m.cfg), loadGroups(m.cfg), loadFriction(m.cfg), textinput.Blink)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case stateMsg:
		m.state = msg.state
		m.loaded = true
		return m, nil
	case groupsMsg:
		m.groups = msg.groups
		return m, nil
	case frictionMsg:
		m.policy = msg.policy
		return m, nil
	case actionResultMsg:
		m.working = false
		m.mode = modeForm
		m.message = msg.message
		m.err = !msg.success
		if msg.success {
			return m, tea.Batch(loadState(m.cfg), loadGroups(m.cfg), loadFriction(m.cfg))
		}
		return m, nil
	case waitTickMsg:
		if m.mode != modeWait {
			return m, nil
		}
		if m.waitRemaining > 0 {
			m.waitRemaining--
		}
		if m.waitRemaining > 0 {
			return m, waitTick()
		}
		m.mode = modeChallenge
		m.message = ""
		m.err = false
		m.nextChallenge()
		m.answerInput.Focus()
		return m, textinput.Blink
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	var cmd tea.Cmd
	switch m.mode {
	case modeChallenge:
		m.answerInput, cmd = m.answerInput.Update(msg)
	case modeReason:
		m.reasonInput, cmd = m.reasonInput.Update(msg)
	default:
		m, cmd = m.updateInput(msg)
	}
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mode == modeWait {
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}
		return m, nil
	}
	if m.mode == modeChallenge {
		return m.handleChallengeKey(msg)
	}
	if m.mode == modeReason {
		return m.handleReasonKey(msg)
	}
	if m.working {
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "r":
		m.message = ""
		return m, tea.Batch(loadState(m.cfg), loadGroups(m.cfg), loadFriction(m.cfg))
	case "down":
		m.move(1)
		return m, nil
	case "up":
		m.move(-1)
		return m, nil
	case "left":
		m.adjust(-1)
		return m, nil
	case "right":
		m.adjust(1)
		return m, nil
	case "+", "=":
		if m.currentField() == fieldHours {
			m.hours++
			return m, nil
		}
	case "-":
		if m.currentField() == fieldHours && m.hours > 1 {
			m.hours--
			return m, nil
		}
	case "enter":
		if m.currentField() == fieldSubmit {
			return m.submit()
		}
		m.move(1)
		return m, nil
	}

	var cmd tea.Cmd
	m, cmd = m.updateInput(msg)
	return m, cmd
}

func (m Model) handleReasonKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "enter":
		reason := strings.TrimSpace(m.reasonInput.Value())
		if reason == "" {
			m.message = "reason is required"
			m.err = true
			return m, nil
		}
		m.pending.reason = reason
		m.mode = modeForm
		m.working = true
		m.message = "running"
		m.err = false
		return m, executeAction(m.cfg, m.pending)
	}

	var cmd tea.Cmd
	m.reasonInput, cmd = m.reasonInput.Update(msg)
	return m, cmd
}

func (m Model) handleChallengeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "enter":
		input := strings.TrimSpace(m.answerInput.Value())
		answer, err := strconv.Atoi(input)
		m.answerInput.Reset()
		if err != nil {
			m.mistakes++
			m.message = fmt.Sprintf("Invalid number. Mistake %d/3.", m.mistakes)
			m.err = true
			if m.mistakes >= 3 {
				return m.cancelResistance()
			}
			return m, nil
		}
		if answer == m.answer {
			m.solved++
			m.message = fmt.Sprintf("Correct. %d more required.", m.required-m.solved)
			m.err = false
			if m.solved >= m.required {
				m.mode = modeReason
				m.working = false
				m.message = "All challenges solved. Enter a reason."
				m.err = false
				m.reasonInput.Reset()
				m.reasonInput.Focus()
				return m, textinput.Blink
			}
			m.nextChallenge()
			return m, nil
		}
		m.mistakes++
		m.message = fmt.Sprintf("Wrong. Mistake %d/3.", m.mistakes)
		m.err = true
		if m.mistakes >= 3 {
			return m.cancelResistance()
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.answerInput, cmd = m.answerInput.Update(msg)
	return m, cmd
}

func (m Model) cancelResistance() (tea.Model, tea.Cmd) {
	m.mode = modeForm
	m.working = false
	m.message = "Too many failed attempts. Unlock cancelled."
	m.err = true
	m.answerInput.Reset()
	m.focusCurrentInput()
	return m, nil
}

func (m *Model) nextChallenge() {
	m.question, m.answer = generateMathChallenge()
	m.answerInput.Reset()
	m.answerInput.Focus()
}

func (m *Model) move(delta int) {
	fields := m.fields()
	if len(fields) == 0 {
		return
	}
	m.cursor = (m.cursor + delta + len(fields)) % len(fields)
	m.focusCurrentInput()
}

func (m *Model) adjust(delta int) {
	switch m.currentField() {
	case fieldCommand:
		m.command = commandKind(wrap(int(m.command)+delta, len(commandNames())))
		if m.command == commandPlan {
			m.target = targetURL
		}
		m.cursor = clampCursor(m.cursor, len(m.fields()))
	case fieldTarget:
		m.target = targetKind(wrap(int(m.target)+delta, len(targetNames())))
	}
	m.message = ""
	m.focusCurrentInput()
}

func (m *Model) focusCurrentInput() {
	m.valueInput.Blur()
	m.reasonInput.Blur()
	m.answerInput.Blur()
	switch m.currentField() {
	case fieldValue:
		m.valueInput.Focus()
	case fieldReason:
		m.reasonInput.Focus()
	}
}

func (m Model) updateInput(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.currentField() {
	case fieldValue:
		m.valueInput, cmd = m.valueInput.Update(msg)
	case fieldReason:
		m.reasonInput, cmd = m.reasonInput.Update(msg)
	}
	return m, cmd
}

func (m Model) submit() (tea.Model, tea.Cmd) {
	if err := m.validate(); err != nil {
		m.message = err.Error()
		m.err = true
		return m, nil
	}
	m.pending = pendingAction{
		command: m.command,
		target:  m.target,
		value:   strings.TrimSpace(m.valueInput.Value()),
		hours:   m.hours,
	}
	if m.command == commandPlan {
		m.pending.reason = strings.TrimSpace(m.reasonInput.Value())
	}
	if m.command == commandAllow || m.command == commandPanic {
		m.startResistance()
		return m, waitTick()
	}
	m.working = true
	m.message = "running"
	m.err = false
	return m, executeAction(m.cfg, m.pending)
}

func (m *Model) startResistance() {
	waitSeconds, _, ok := resistanceTarget(m.pending.target, m.pending.value)
	if !ok {
		return
	}
	policy := m.policy
	if policy.Challenges == 0 {
		policy.Challenges = 3
	}
	if m.pending.command == commandPanic {
		waitSeconds = waitSeconds*3 + waitSeconds + policy.ExtraWait + 120
		policy.Challenges += 2
	} else {
		waitSeconds += policy.ExtraWait
	}
	m.mode = modeWait
	m.working = true
	m.waitRemaining = waitSeconds
	m.required = policy.Challenges
	if m.required < 3 {
		m.required = 3
	}
	m.solved = 0
	m.mistakes = 0
	m.message = ""
	m.err = false
}

func (m Model) validate() error {
	if m.command != commandPlan && m.target != targetAll && strings.TrimSpace(m.valueInput.Value()) == "" {
		if m.target == targetURL {
			return fmt.Errorf("url is required")
		}
		return fmt.Errorf("group is required")
	}
	if m.command == commandPlan && strings.TrimSpace(m.reasonInput.Value()) == "" {
		return fmt.Errorf("reason is required")
	}
	if m.command == commandPlan && m.hours <= 0 {
		return fmt.Errorf("hours must be greater than zero")
	}
	return nil
}

func (m Model) fields() []fieldKind {
	if m.command == commandPlan {
		return []fieldKind{fieldCommand, fieldHours, fieldReason, fieldSubmit}
	}
	if m.target == targetAll {
		if m.command == commandBlock {
			return []fieldKind{fieldCommand, fieldTarget, fieldSubmit}
		}
		return []fieldKind{fieldCommand, fieldTarget, fieldSubmit}
	}
	if m.command == commandBlock {
		return []fieldKind{fieldCommand, fieldTarget, fieldValue, fieldSubmit}
	}
	return []fieldKind{fieldCommand, fieldTarget, fieldValue, fieldSubmit}
}

func (m Model) currentField() fieldKind {
	fields := m.fields()
	if len(fields) == 0 {
		return fieldCommand
	}
	cursor := clampCursor(m.cursor, len(fields))
	return fields[cursor]
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("bakchodi"))
	b.WriteString(mutedStyle.Render("  TUI"))
	b.WriteString("\n\n")
	b.WriteString(m.statusLine())
	b.WriteString("\n")
	b.WriteString(m.budgetLine())
	b.WriteString("\n\n")
	if m.mode == modeWait {
		b.WriteString(m.waitView())
	} else if m.mode == modeChallenge {
		b.WriteString(m.challengeView())
	} else if m.mode == modeReason {
		b.WriteString(m.reasonView())
	} else {
		b.WriteString(mutedStyle.Render(m.commandDescription()))
		b.WriteString("\n\n")
		b.WriteString(m.commandForm())
	}
	b.WriteString("\n\n")
	b.WriteString(m.groupsLine())

	if m.message != "" {
		b.WriteString("\n\n")
		if m.err {
			b.WriteString(badStyle.Render(m.message))
		} else {
			b.WriteString(goodStyle.Render(m.message))
		}
	}

	b.WriteString("\n\n")
	if m.mode == modeForm {
		b.WriteString(mutedStyle.Render("up/down: move  left/right: change choice  enter: next/run  r: refresh  q: quit"))
	} else if m.mode == modeChallenge {
		b.WriteString(mutedStyle.Render("enter: answer  q: quit"))
	} else if m.mode == modeReason {
		b.WriteString(mutedStyle.Render("enter: submit reason  q: quit"))
	} else {
		b.WriteString(mutedStyle.Render("waiting  q: quit"))
	}
	return pageStyle.Render(b.String())
}

func (m Model) statusLine() string {
	if !m.loaded {
		return mutedStyle.Render("status: loading")
	}
	if len(m.state.ActiveUnlocks) == 0 {
		return badStyle.Render("status: blocked")
	}
	parts := []string{}
	for _, unlock := range m.state.ActiveUnlocks {
		if time.Now().After(unlock.Expiry) {
			continue
		}
		left := time.Until(unlock.Expiry).Round(time.Minute)
		parts = append(parts, fmt.Sprintf("%s:%s (%s)", unlock.Type, unlock.Target, left))
	}
	if len(parts) == 0 {
		return badStyle.Render("status: blocked")
	}
	return goodStyle.Render("active unlocks: ") + strings.Join(parts, ", ")
}

func (m Model) budgetLine() string {
	total := m.state.DailyBudgetMinutes
	if total == 0 {
		total = model.DefaultDailyBudgetMinutes
	}
	used := 0
	if m.state.UsedBudgetByDate != nil {
		used = m.state.UsedBudgetByDate[time.Now().Format("2006-01-02")]
	}
	remaining := total - used
	if remaining < 0 {
		remaining = 0
	}

	line := fmt.Sprintf("budget: %d/%d min used, %d min left", used, total, remaining)
	if !m.state.CommitmentUntil.IsZero() && time.Now().Before(m.state.CommitmentUntil) {
		line += "  " + warnStyle.Render("commitment until "+m.state.CommitmentUntil.Format("2006-01-02 15:04"))
	}
	return line
}

func (m Model) commandForm() string {
	rows := []string{
		m.row(fieldCommand, "command", selector(commandNames(), int(m.command))),
	}
	if m.command != commandPlan {
		rows = append(rows, m.row(fieldTarget, "target", selector(targetNames(), int(m.target))))
		if m.target != targetAll {
			rows = append(rows, m.row(fieldValue, targetNames()[m.target], m.valueInput.View()))
		}
	}
	if m.command == commandPlan {
		rows = append(rows, m.row(fieldHours, "hours", strconv.Itoa(m.hours)))
	}
	if m.command == commandPlan {
		rows = append(rows, m.row(fieldReason, "reason", m.reasonInput.View()))
	}
	rows = append(rows, m.row(fieldSubmit, "run", "execute"))
	rows = append(rows, "", mutedStyle.Render(m.fieldHelp()))
	return strings.Join(rows, "\n")
}

func (m Model) row(field fieldKind, label string, value string) string {
	prefix := "  "
	if m.currentField() == field {
		prefix = "> "
		value = activeStyle.Render(value)
	}
	return prefix + labelStyle.Render(label+":") + " " + value
}

func (m Model) groupsLine() string {
	if len(m.groups) == 0 {
		return mutedStyle.Render("groups: none loaded")
	}
	names := make([]string, 0, len(m.groups))
	for name := range m.groups {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) > 8 {
		names = append(names[:8], fmt.Sprintf("+%d more", len(names)-8))
	}
	return mutedStyle.Render("groups: " + strings.Join(names, ", "))
}

func (m Model) commandDescription() string {
	switch m.command {
	case commandBlock:
		return "Block access by locking one URL, one saved group, or every saved group."
	case commandAllow:
		return "Temporarily unlock access. This will force a wait, math checks, and then ask for a reason."
	case commandPanic:
		return "Emergency unlock. This is slower, stricter, limited to a short window, and logged."
	case commandPlan:
		return "Create a no-break commitment so normal unlocks are blocked for the chosen hours."
	default:
		return ""
	}
}

func (m Model) fieldHelp() string {
	switch m.currentField() {
	case fieldCommand:
		return "Choose what you want to do."
	case fieldTarget:
		return "Choose whether this affects one URL, a saved group, or everything."
	case fieldValue:
		if m.target == targetGroup {
			return "Enter the saved group name, for example social or video."
		}
		return "Enter a domain or URL, for example youtube.com."
	case fieldHours:
		return "Choose how long the commitment should last. Use left/right or +/-."
	case fieldReason:
		return "Enter the reason that will be logged."
	case fieldSubmit:
		return m.submitHelp()
	default:
		return ""
	}
}

func (m Model) submitHelp() string {
	switch m.command {
	case commandBlock:
		return "Execute now. Blocking does not require wait or math checks."
	case commandAllow:
		return "Start unlock flow: wait, solve math, enter reason, then unlock."
	case commandPanic:
		return "Start emergency flow: longer wait, harder math, enter reason, then unlock."
	case commandPlan:
		return "Create the commitment with the selected hours and reason."
	default:
		return "Execute the selected action."
	}
}

func (m Model) waitView() string {
	_, targetName, _ := resistanceTarget(m.pending.target, m.pending.value)
	if m.pending.command == commandPanic {
		return warnStyle.Render("BREAK GLASS unlock requested for "+targetName) + "\n" +
			fmt.Sprintf("This bypasses normal budget/commitment checks, is limited to %d minutes, and is logged.\n", model.DefaultBreakGlassMinutes) +
			fmt.Sprintf("Wait %d seconds and solve all challenges to continue.", m.waitRemaining)
	}
	line := fmt.Sprintf("You're about to unlock %s\n", targetName)
	if m.policy.AttemptsToday > 0 {
		line += fmt.Sprintf("Escalation: %d unlock attempt(s) today, adding %d seconds and %d challenge(s).\n", m.policy.AttemptsToday, m.policy.ExtraWait, m.policy.Challenges)
	}
	line += fmt.Sprintf("Wait %d seconds to confirm you really want this.", m.waitRemaining)
	return line
}

func (m Model) challengeView() string {
	return fmt.Sprintf(
		"Challenge %d/%d: %s = ?\nMistakes: %d/3\n\nanswer: %s",
		m.solved+1,
		m.required,
		m.question,
		m.mistakes,
		m.answerInput.View(),
	)
}

func (m Model) reasonView() string {
	return "Reason: " + m.reasonInput.View()
}

type stateMsg struct{ state model.StateJSON }
type groupsMsg struct{ groups model.GroupMap }
type frictionMsg struct{ policy model.FrictionPolicy }
type actionResultMsg struct {
	success bool
	message string
}
type pendingAction struct {
	command commandKind
	target  targetKind
	value   string
	reason  string
	hours   int
}

func loadState(cfg platform.Config) tea.Cmd {
	return func() tea.Msg {
		body, err := httpGet(cfg, "/state")
		if err != nil {
			return stateMsg{state: model.StateJSON{}}
		}
		var state model.StateJSON
		_ = json.Unmarshal(body, &state)
		return stateMsg{state: state}
	}
}

func loadGroups(cfg platform.Config) tea.Cmd {
	return func() tea.Msg {
		body, err := httpGet(cfg, "/groups")
		if err != nil {
			return groupsMsg{groups: model.GroupMap{}}
		}
		var groups model.GroupMap
		_ = json.Unmarshal(body, &groups)
		return groupsMsg{groups: groups}
	}
}

func loadFriction(cfg platform.Config) tea.Cmd {
	return func() tea.Msg {
		body, err := httpGet(cfg, "/friction")
		if err != nil {
			return frictionMsg{policy: model.FrictionPolicy{Challenges: 3}}
		}
		var policy model.FrictionPolicy
		if err := json.Unmarshal(body, &policy); err != nil || policy.Challenges == 0 {
			return frictionMsg{policy: model.FrictionPolicy{Challenges: 3}}
		}
		return frictionMsg{policy: policy}
	}
}

func executeAction(cfg platform.Config, action pendingAction) tea.Cmd {
	return func() tea.Msg {
		endpoint, payload := requestFor(action)
		body, err := httpPost(cfg, endpoint, payload)
		if err != nil {
			return actionResultMsg{success: false, message: err.Error()}
		}
		return actionResultMsg{success: true, message: strings.TrimSpace(string(body))}
	}
}

func requestFor(action pendingAction) (string, map[string]any) {
	if action.command == commandPlan {
		return "/commit", map[string]any{"hours": action.hours, "reason": action.reason}
	}

	prefix := "lock"
	if action.command == commandAllow || action.command == commandPanic {
		prefix = "unlock"
	}

	endpoint := "/" + prefix
	payload := map[string]any{}
	switch action.target {
	case targetURL:
		endpoint += "-url"
		payload["target"] = action.value
	case targetGroup:
		endpoint += "-group"
		payload["target"] = action.value
	}
	if action.command == commandAllow || action.command == commandPanic {
		payload["reason"] = action.reason
	}
	if action.command == commandPanic {
		payload["break_glass"] = true
	}
	return endpoint, payload
}

func httpGet(cfg platform.Config, endpoint string) ([]byte, error) {
	client := http.Client{}
	url := "http://localhost" + endpoint
	if cfg.UsesUnixSocket() {
		client.Transport = &http.Transport{Dial: func(network, addr string) (net.Conn, error) {
			return net.Dial(cfg.SocketNetwork, cfg.SocketAddress)
		}}
		url = "http://unix" + endpoint
	}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func httpPost(cfg platform.Config, endpoint string, payload map[string]any) ([]byte, error) {
	data, _ := json.Marshal(payload)
	client := http.Client{}
	url := "http://localhost" + endpoint
	if cfg.UsesUnixSocket() {
		client.Transport = &http.Transport{Dial: func(network, addr string) (net.Conn, error) {
			return net.Dial(cfg.SocketNetwork, cfg.SocketAddress)
		}}
		url = "http://unix" + endpoint
	}
	resp, err := client.Post(url, "application/json", strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s", strings.TrimSpace(string(body)))
	}
	return body, nil
}

func commandNames() []string {
	return []string{"block", "allow", "panic", "plan"}
}

func targetNames() []string {
	return []string{"url", "group", "all"}
}

func selector(values []string, selected int) string {
	out := make([]string, len(values))
	for i, value := range values {
		if i == selected {
			out[i] = "[" + value + "]"
		} else {
			out[i] = value
		}
	}
	return strings.Join(out, "  ")
}

func wrap(value int, total int) int {
	if total <= 0 {
		return 0
	}
	for value < 0 {
		value += total
	}
	return value % total
}

func clampCursor(cursor int, total int) int {
	if total <= 0 {
		return 0
	}
	if cursor >= total {
		return total - 1
	}
	if cursor < 0 {
		return 0
	}
	return cursor
}

func waitTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return waitTickMsg(t)
	})
}

func resistanceTarget(target targetKind, value string) (int, string, bool) {
	switch target {
	case targetAll:
		return 60, "all sites", true
	case targetGroup:
		return 60, "group: " + value, true
	case targetURL:
		return 30, "URL: " + value, true
	default:
		return 0, "", false
	}
}

func generateMathChallenge() (string, int) {
	a := randomInt(37, 96)
	b := randomInt(12, 39)
	c := randomInt(120, 460)
	d := randomInt(7, 28)
	e := randomInt(4, 17)

	switch randomInt(0, 2) {
	case 0:
		return fmt.Sprintf("(%d x %d) - %d + (%d x %d)", a, b, c, d, e), (a*b - c) + (d * e)
	case 1:
		return fmt.Sprintf("(%d + %d) x %d - %d", a, c, e, b*d), (a+c)*e - (b * d)
	default:
		return fmt.Sprintf("(%d x %d) + %d - (%d x %d)", c, e, a, b, d), (c*e + a) - (b * d)
	}
}

func randomInt(min, max int) int {
	if max < min {
		min, max = max, min
	}
	n, err := crand.Int(crand.Reader, big.NewInt(int64(max-min+1)))
	if err != nil {
		return min
	}
	return min + int(n.Int64())
}

func Run(cfg platform.Config) error {
	p := tea.NewProgram(InitialModel(cfg), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
