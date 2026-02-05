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
	args := os.Args[2:]

	if cmd != "upgrade" {
		startUpdateCheck()
		defer printUpdateNotice()
	}

	switch cmd {
	case "-h", "--help", "help":
		showHelp()
	case "-v", "--version", "version":
		fmt.Printf("ralph v%s\n", version)
	case "init":
		cmdInit(args)
	case "run":
		cmdRun(args)
	case "verify":
		cmdVerify(args)
	case "prd":
		cmdPrd(args)
	case "status":
		cmdStatus(args)
	case "next":
		cmdNext(args)
	case "validate":
		cmdValidate(args)
	case "doctor":
		cmdDoctor(args)
	case "logs":
		cmdLogs(args)
	case "upgrade":
		cmdUpgrade(args)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		fmt.Fprintln(os.Stderr, "Run 'ralph --help' for usage.")
		os.Exit(1)
	}
}

func showHelp() {
	fmt.Printf(`ralph v%s - Autonomous AI agent loop

Usage: ralph <command> [feature] [options]

Commands:
  init [--force]       Initialize Ralph (creates ralph.config.json + .ralph/)
  run <feature>        Run the agent loop for a feature
  verify <feature>     Run verification only for a feature
  prd <feature>        Create/refine/finalize a PRD for a feature
  status [feature]     Show story status (all features or specific)
  next <feature>       Show the next story to work on
  validate <feature>   Validate prd.json schema
  logs <feature>       View run logs (--list, --summary, --follow, etc.)
  doctor               Check Ralph environment
  upgrade              Upgrade Ralph to the latest version

Options:
  -h, --help           Show this help message
  -v, --version        Show version number

Examples:
  ralph init                    # Initialize Ralph in current project
  ralph prd auth                # Create/refine PRD for 'auth' feature
  ralph run auth                # Run the loop for 'auth' feature
  ralph status                  # Show status of all features
  ralph status auth             # Show status of 'auth' feature
  ralph verify auth             # Verify 'auth' feature only

File Structure:
  ralph.config.json             # Project configuration (required)
  .ralph/
    2024-01-15-auth/
      prd.md                    # Human-readable PRD
      prd.json                  # Finalized for execution
    screenshots/                # Browser verification evidence
`, version)
}
