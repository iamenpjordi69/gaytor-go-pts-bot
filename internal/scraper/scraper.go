package scraper

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// WinLog represents a single scraped Territorial.io match
type WinLog struct {
	Time           string
	MapName        string
	PlayerCount    int
	IsContest      bool
	BasePoints     float64
	FinalPoints    float64
	WinningClan    string
	PrevPoints     string
	CurrPoints     string
	PayoutAccounts []string
}

// ScrapeMatchLogs fetches and parses the latest matches
func ScrapeMatchLogs() ([]*WinLog, error) {
	client := http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", "https://territorial.io/clan-results", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d from territorial.io", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	content := string(body)
	lines := strings.Split(content, "\n")
	var cleanLines []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" {
			cleanLines = append(cleanLines, trimmed)
		}
	}

	if len(cleanLines) < 8 {
		return nil, fmt.Errorf("not enough lines")
	}

	var startIndices []int
	for i, l := range cleanLines {
		if strings.HasPrefix(l, "Time:") {
			startIndices = append(startIndices, i)
		}
	}

	if len(startIndices) == 0 {
		return nil, fmt.Errorf("could not find valid Time block")
	}

	var winlogs []*WinLog

	// Pre-compile regexes for performance
	rxTime := regexp.MustCompile(`Time:\s*(.+)`)
	rxContest := regexp.MustCompile(`(?i)Contest:\s*(\w+)`)
	rxMap := regexp.MustCompile(`Map:\s*(.+)`)
	rxPlayers := regexp.MustCompile(`Player\s*Count:\s*(\d+)`)
	rxClan := regexp.MustCompile(`Winning\s*Clan:\s*\[([^\]]+)\]`)
	rxPrev := regexp.MustCompile(`Prev\.?\s*Points:\s*([\d.]+)`)
	rxCurr := regexp.MustCompile(`Curr\.?\s*Points:\s*([\d.]+)`)
	rx5char := regexp.MustCompile(`\b([a-zA-Z0-9]{5})\b`)

	for _, startIdx := range startIndices {
		if startIdx+7 >= len(cleanLines) {
			continue // incomplete block at the end
		}

		wl := &WinLog{}

		// Time
		if m := rxTime.FindStringSubmatch(cleanLines[startIdx]); len(m) > 1 {
			wl.Time = strings.TrimSpace(m[1])
		}

		// Contest
		if m := rxContest.FindStringSubmatch(cleanLines[startIdx+1]); len(m) > 1 {
			val := strings.ToLower(strings.TrimSpace(m[1]))
			wl.IsContest = (val == "yes" || val == "true" || val == "1")
		}

		// Map
		if m := rxMap.FindStringSubmatch(cleanLines[startIdx+2]); len(m) > 1 {
			wl.MapName = strings.TrimSpace(m[1])
		}

		// Player Count
		if m := rxPlayers.FindStringSubmatch(cleanLines[startIdx+3]); len(m) > 1 {
			pc, _ := strconv.Atoi(m[1])
			wl.PlayerCount = pc
		}

		// Winning Clan
		if m := rxClan.FindStringSubmatch(cleanLines[startIdx+4]); len(m) > 1 {
			wl.WinningClan = strings.TrimSpace(m[1])
		}

		// Prev Points
		if m := rxPrev.FindStringSubmatch(cleanLines[startIdx+5]); len(m) > 1 {
			wl.PrevPoints = m[1]
		}

		// Curr Points
		if m := rxCurr.FindStringSubmatch(cleanLines[startIdx+7]); len(m) > 1 {
			wl.CurrPoints = m[1]
		}

		// Base / Final points logic
		wl.BasePoints = float64(wl.PlayerCount)
		wl.FinalPoints = wl.BasePoints
		if wl.IsContest {
			wl.FinalPoints *= 2
		}

		// Extract Payouts natively
		for i := startIdx + 8; i < startIdx+15 && i < len(cleanLines); i++ {
			line := cleanLines[i]
			if strings.Contains(strings.ToLower(line), "Time:") {
				break // Hit the next block
			}
			if strings.Contains(strings.ToLower(line), "payout") {
				parts := strings.SplitN(line, ":", 2)
				textToParse := line
				if len(parts) > 1 {
					textToParse = parts[1]
				}

				matches := rx5char.FindAllStringSubmatch(textToParse, -1)

				seen := make(map[string]bool)
				for _, matchGroup := range matches {
					if len(matchGroup) > 1 {
						acc := matchGroup[1]
						if !seen[acc] {
							wl.PayoutAccounts = append(wl.PayoutAccounts, acc)
							seen[acc] = true
						}
					}
				}
				break
			}
		}

		winlogs = append(winlogs, wl)
	}

	return winlogs, nil
}
