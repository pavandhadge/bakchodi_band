package hosts

import (
	"net"
	"net/url"
	"os"
	"strings"
)

const (
	startMarker = "# --- BAKCHODI-BAND-START ---"
	endMarker   = "# --- BAKCHODI-BAND-END ---"
)

func Sync(path string, urlsToBlock []string) error {
	input, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(input), "\n")
	newLines := make([]string, 0, len(lines))
	inBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == startMarker {
			inBlock = true
			continue
		}
		if trimmed == endMarker {
			inBlock = false
			continue
		}
		if !inBlock {
			newLines = append(newLines, line)
		}
	}

	blockContent := buildBlock(urlsToBlock)
	finalOutput := strings.TrimSpace(strings.Join(newLines, "\n"))
	if len(blockContent) > 0 {
		finalOutput += "\n\n" + strings.Join(blockContent, "\n")
	}

	return os.WriteFile(path, []byte(strings.TrimSpace(finalOutput)+"\n"), 0644)
}

func buildBlock(urls []string) []string {
	if len(urls) == 0 {
		return nil
	}

	block := []string{startMarker}
	seen := make(map[string]bool)
	for _, rawURL := range urls {
		host := normalizeHost(rawURL)
		if host == "" {
			continue
		}

		for _, name := range hostVariants(host) {
			for _, target := range []string{"0.0.0.0", "::1"} {
				line := target + " " + name
				if !seen[line] {
					block = append(block, line)
					seen[line] = true
				}
			}
		}
	}
	return append(block, endMarker)
}

func normalizeHost(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := parsed.Hostname()
	if host == "" {
		return ""
	}
	return strings.TrimPrefix(host, "www.")
}

func hostVariants(host string) []string {
	if net.ParseIP(host) != nil {
		return []string{host}
	}
	return []string{host, "www." + host}
}
