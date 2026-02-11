package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	selfupdate "github.com/creativeprojects/go-selfupdate"
)

const updateCheckInterval = 24 * time.Hour

type updateCheckCache struct {
	LastCheck     time.Time `json:"lastCheck"`
	LatestVersion string    `json:"latestVersion"`
}

// updateNotice holds the result of a background update check.
var updateNotice chan string

// startUpdateCheck kicks off a background goroutine that checks for a newer
// version. Call printUpdateNotice before exiting to display the result.
func startUpdateCheck() {
	if version == "dev" {
		return
	}

	updateNotice = make(chan string, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				// never crash the main process
			}
		}()

		latest, ok := checkForUpdate()
		if ok {
			updateNotice <- latest
		}
		close(updateNotice)
	}()
}

// printUpdateNotice prints a notification if a newer version was found.
// Non-blocking: if the check hasn't finished yet, it skips.
func printUpdateNotice() {
	if updateNotice == nil {
		return
	}
	select {
	case v, ok := <-updateNotice:
		if ok && v != "" {
			os.Stderr.WriteString("\nA new version of ralph is available: v" + v + " (current: v" + version + ")\nRun 'ralph upgrade' to update.\n")
		}
	default:
		// check still running, don't block
	}
}

func checkForUpdate() (string, bool) {
	cachePath := updateCheckCachePath()

	// Check cache first
	if data, err := os.ReadFile(cachePath); err == nil {
		var cache updateCheckCache
		if json.Unmarshal(data, &cache) == nil {
			if time.Since(cache.LastCheck) < updateCheckInterval {
				if cache.LatestVersion != "" && isNewerVersion(cache.LatestVersion, version) {
					return cache.LatestVersion, true
				}
				return "", false
			}
		}
	}

	// Cache expired or missing, check GitHub
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	latest, found, err := selfupdate.DetectLatest(ctx, selfupdate.ParseSlug("scripness/ralph"))
	if err != nil || !found {
		return "", false
	}

	latestVersion := latest.Version()

	// Write cache
	cache := updateCheckCache{
		LastCheck:     time.Now(),
		LatestVersion: latestVersion,
	}
	if data, err := json.Marshal(cache); err == nil {
		os.MkdirAll(filepath.Dir(cachePath), 0755)
		os.WriteFile(cachePath, data, 0644)
	}

	if latest.LessOrEqual(version) {
		return "", false
	}
	return latestVersion, true
}

// isNewerVersion returns true if latest is strictly newer than current.
// Both versions may optionally have a "v" prefix.
func isNewerVersion(latest, current string) bool {
	latest = strings.TrimPrefix(latest, "v")
	current = strings.TrimPrefix(current, "v")
	// Split into parts and compare numerically
	lp := strings.Split(latest, ".")
	cp := strings.Split(current, ".")
	for i := 0; i < len(lp) && i < len(cp); i++ {
		if lp[i] != cp[i] {
			// Compare as numbers by padding to same length
			for len(lp[i]) < len(cp[i]) {
				lp[i] = "0" + lp[i]
			}
			for len(cp[i]) < len(lp[i]) {
				cp[i] = "0" + cp[i]
			}
			return lp[i] > cp[i]
		}
	}
	return len(lp) > len(cp)
}

func updateCheckCachePath() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "ralph", "update-check.json")
	}
	return filepath.Join(os.TempDir(), "ralph-update-check.json")
}
