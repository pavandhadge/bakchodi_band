package client

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/pavandhadge/dopamine_blocker/internal/friction"
	"github.com/pavandhadge/dopamine_blocker/internal/model"
	"github.com/pavandhadge/dopamine_blocker/internal/platform"
)

type Client struct {
	Config platform.Config
	token  string
}

func New(cfg platform.Config) Client {
	c := Client{Config: cfg}
	c.loadAuthToken()
	return c
}

func (c *Client) loadAuthToken() {
	data, err := os.ReadFile(c.Config.TokenPath)
	if err != nil {
		return
	}
	c.token = strings.TrimSpace(string(data))
}

func (c Client) Block(targetType, target string) error {
	return c.send("/"+targetType, map[string]any{"target": target})
}

func (c Client) Unlock(targetType, target string) error {
	policy := c.FrictionPolicy()
	ApplyResistance(friction.ParseTargetType(targetType), target, policy)
	reason := promptReason()
	return c.send("/"+targetType, map[string]any{"target": target, "reason": reason})
}

func (c Client) BreakGlass(targetType, target string) error {
	policy := c.FrictionPolicy()
	ApplyBreakGlassResistance(friction.ParseTargetType(targetType), target, policy)
	reason := promptReason()
	return c.send("/"+targetType, map[string]any{"target": target, "reason": reason, "break_glass": true})
}

func (c Client) FrictionPolicy() model.FrictionPolicy {
	body, err := c.get("/friction")
	if err != nil {
		return model.FrictionPolicy{Challenges: 3}
	}
	var policy model.FrictionPolicy
	if err := json.Unmarshal(body, &policy); err != nil || policy.Challenges == 0 {
		return model.FrictionPolicy{Challenges: 3}
	}
	return policy
}

func (c Client) Commit(hours int, reason string) error {
	if strings.TrimSpace(reason) == "" {
		reason = promptReason()
	}
	return c.send("/commit", map[string]any{"hours": hours, "reason": reason})
}

func (c Client) AddGroup(name string, urls []string, file string) error {
	fileURLs, err := readURLFile(file)
	if err != nil {
		return err
	}
	urls = append(urls, fileURLs...)
	urls = uniqueStrings(urls)
	if name == "" {
		return fmt.Errorf("group name is required")
	}
	if len(urls) == 0 {
		return fmt.Errorf("at least one URL is required")
	}
	return c.send("/add-group", map[string]any{"group_name": name, "urls": urls})
}

func (c Client) DeleteGroup(name string) error {
	if name == "" {
		return fmt.Errorf("group name is required")
	}
	return c.send("/delete-group", map[string]any{"group_name": name})
}

func (c Client) RenameGroup(oldName, newName string) error {
	if oldName == "" || newName == "" {
		return fmt.Errorf("both old and new group names are required")
	}
	return c.send("/rename-group", map[string]any{"old_name": oldName, "new_name": newName})
}

func (c Client) ImportGroups(path string, merge bool) error {
	groups, err := readGroupsFile(path)
	if err != nil {
		return err
	}
	return c.send("/import-groups", map[string]any{"groups": groups, "merge": merge})
}

func (c Client) ImportConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var cfg model.ConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	return c.send("/import-config", map[string]any{"policy": cfg.Policy, "groups": cfg.Groups})
}

func WriteDefaultConfig(path string) error {
	cfg := model.ConfigFile{
		Policy: model.PolicyConfig{
			DailyBudgetMinutes: model.DefaultDailyBudgetMinutes,
			UnlockMinutes:      model.DefaultUnlockMinutes,
			BreakGlassMinutes:  model.DefaultBreakGlassMinutes,
		},
		Groups: map[string][]string{
			"social": {"x.com", "instagram.com", "facebook.com", "reddit.com"},
			"video":  {"youtube.com", "netflix.com", "twitch.tv"},
			"shorts": {"tiktok.com", "youtube.com", "instagram.com"},
		},
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil && filepath.Dir(path) != "." {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (c Client) send(endpoint string, payload map[string]any) error {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	httpClient := c.httpClient()
	url := "http://localhost" + endpoint

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s", bytes.TrimSpace(body))
	}
	_, err = os.Stdout.Write(body)
	return err
}

func (c Client) get(endpoint string) ([]byte, error) {
	httpClient := c.httpClient()
	url := "http://localhost" + endpoint

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func (c Client) httpClient() http.Client {
	dial := c.dialFunc()
	if dial == nil {
		return http.Client{}
	}
	return http.Client{
		Transport: &http.Transport{
			Dial: dial,
		},
	}
}

func (c Client) dialFunc() func(network, addr string) (net.Conn, error) {
	if c.Config.UsesUnixSocket() {
		return func(network, addr string) (net.Conn, error) {
			return net.Dial(c.Config.SocketNetwork, c.Config.SocketAddress)
		}
	}
	return nil
}

func promptReason() string {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("Reason: ")
		input, _ := reader.ReadString('\n')
		reason := strings.TrimSpace(input)
		if reason != "" {
			return reason
		}
		fmt.Println("A reason is required.")
	}
}

func readURLFile(path string) ([]string, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var urls []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		urls = append(urls, line)
	}
	return urls, scanner.Err()
}

func readGroupsFile(path string) (model.GroupMap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var groups model.GroupMap
	if err := json.Unmarshal(data, &groups); err == nil && groups != nil {
		return groups, nil
	}

	var cfg model.ConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return cfg.Groups, nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
