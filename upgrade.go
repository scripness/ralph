package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	repoOwner = "scripness"
	repoName  = "ralph"
)

type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func cmdUpgrade(args []string) {
	fmt.Println("Checking for updates...")

	// Get latest release from GitHub
	release, err := getLatestRelease()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to check for updates: %v\n", err)
		os.Exit(1)
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	if latestVersion == Version {
		fmt.Printf("Already at latest version (v%s)\n", Version)
		return
	}

	fmt.Printf("New version available: v%s (current: v%s)\n", latestVersion, Version)

	// Find the right asset for this OS/arch
	assetName := getAssetName()
	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		fmt.Fprintf(os.Stderr, "No binary available for %s/%s\n", runtime.GOOS, runtime.GOARCH)
		fmt.Fprintln(os.Stderr, "Please build from source: go install github.com/scripness/ralph@latest")
		os.Exit(1)
	}

	fmt.Printf("Downloading %s...\n", assetName)

	// Download new binary
	newBinary, err := downloadBinary(downloadURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to download update: %v\n", err)
		os.Exit(1)
	}

	// Get current executable path
	execPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to find current executable: %v\n", err)
		os.Exit(1)
	}
	execPath, _ = filepath.EvalSymlinks(execPath)

	// Replace current binary
	if err := replaceBinary(execPath, newBinary); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to install update: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Successfully upgraded to v%s\n", latestVersion)
}

func getLatestRelease() (*GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", repoOwner, repoName)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}

func getAssetName() string {
	os := runtime.GOOS
	arch := runtime.GOARCH

	// Normalize arch names
	if arch == "amd64" {
		arch = "x64"
	}

	return fmt.Sprintf("ralph-%s-%s", os, arch)
}

func downloadBinary(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func replaceBinary(execPath string, newBinary []byte) error {
	// Write to temp file first
	tmpPath := execPath + ".new"
	if err := os.WriteFile(tmpPath, newBinary, 0755); err != nil {
		return err
	}

	// Backup old binary
	backupPath := execPath + ".old"
	os.Remove(backupPath) // Ignore error
	if err := os.Rename(execPath, backupPath); err != nil {
		os.Remove(tmpPath)
		return err
	}

	// Move new binary into place
	if err := os.Rename(tmpPath, execPath); err != nil {
		// Try to restore backup
		os.Rename(backupPath, execPath)
		return err
	}

	// Clean up backup
	os.Remove(backupPath)
	return nil
}
