package main

import (
	"context"
	"fmt"
	"os"
	"runtime"

	selfupdate "github.com/creativeprojects/go-selfupdate"
)

func cmdUpgrade(args []string) {
	fmt.Println("Checking for updates...")

	latest, found, err := selfupdate.DetectLatest(context.Background(), selfupdate.ParseSlug("scripness/ralph"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to check for updates: %v\n", err)
		os.Exit(1)
	}
	if !found {
		fmt.Fprintf(os.Stderr, "No release found for %s/%s\n", runtime.GOOS, runtime.GOARCH)
		os.Exit(1)
	}

	if latest.LessOrEqual(version) {
		fmt.Printf("Already at latest version (v%s)\n", version)
		return
	}

	fmt.Printf("New version available: v%s (current: v%s)\n", latest.Version(), version)

	exe, err := selfupdate.ExecutablePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to find executable path: %v\n", err)
		os.Exit(1)
	}

	if err := selfupdate.UpdateTo(context.Background(), latest.AssetURL, latest.AssetName, exe); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to update: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully upgraded to v%s\n", latest.Version())
}
