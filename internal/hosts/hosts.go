package hosts

import (
	"os"
	"strings"
)

const (
	startMarker = "# --- DOPAMINE-LOCK-START ---"
	endMarker   = "# --- DOPAMINE-LOCK-END ---"
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
	for _, url := range urls {
		block = append(block, "127.0.0.1 "+url)
		block = append(block, "127.0.0.1 www."+url)
	}
	return append(block, endMarker)
}
