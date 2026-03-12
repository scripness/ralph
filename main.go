package main

import (
	"fmt"
	"os"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		showHelp()
		os.Exit(0)
	}

	cmd := os.Args[1]

	switch cmd {
	case "-h", "--help", "help":
		showHelp()
	case "-v", "--version", "version":
		fmt.Printf("scrip v%s\n", version)
	case "prep":
		cmdPrep(os.Args[2:])
	case "plan":
		cmdPlan(os.Args[2:])
	case "exec":
		cmdExec(os.Args[2:])
	case "land":
		cmdLand(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		fmt.Fprintln(os.Stderr, "Run 'scrip --help' for usage.")
		os.Exit(1)
	}
}

func showHelp() {
	fmt.Printf(`scrip v%s — AI-assisted software development

Usage: scrip <command> [options]

Commands:
  prep              Detect project, generate config, cache dependencies
  plan <feature>    Iterative planning with AI consultation
  exec <feature>    Autonomous execution loop
  land <feature>    Verify, summarize, and push

Options:
  -h, --help        Show this help message
  -v, --version     Show version number

Examples:
  scrip prep                      Set up scrip for this project
  scrip plan auth "add login"     Plan the auth feature
  scrip exec auth                 Execute the plan
  scrip land auth                 Verify and land the feature
`, version)
}

// Placeholder command handlers — replaced by cmd_*.go in Sessions 1-4.

func cmdPrep(args []string) {
	_ = args
	fmt.Fprintln(os.Stderr, "scrip prep: not yet implemented")
	os.Exit(1)
}

func cmdPlan(args []string) {
	_ = args
	fmt.Fprintln(os.Stderr, "scrip plan: not yet implemented")
	os.Exit(1)
}

func cmdExec(args []string) {
	_ = args
	fmt.Fprintln(os.Stderr, "scrip exec: not yet implemented")
	os.Exit(1)
}

func cmdLand(args []string) {
	_ = args
	fmt.Fprintln(os.Stderr, "scrip land: not yet implemented")
	os.Exit(1)
}
