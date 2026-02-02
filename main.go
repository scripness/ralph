package main

import (
	"fmt"
	"os"
)

var Version = "1.0.0"

func main() {
	if len(os.Args) < 2 {
		showHelp()
		os.Exit(0)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "-h", "--help", "help":
		showHelp()
	case "-v", "--version", "version":
		fmt.Printf("ralph v%s\n", Version)
	case "init":
		cmdInit(args)
	case "run":
		cmdRun(args)
	case "verify":
		cmdVerify(args)
	case "prd":
		cmdPrd(args)
	case "convert":
		cmdConvert(args)
	case "status":
		cmdStatus(args)
	case "next":
		cmdNext(args)
	case "validate":
		cmdValidate(args)
	case "doctor":
		cmdDoctor(args)
	case "upgrade":
		cmdUpgrade(args)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		fmt.Fprintln(os.Stderr, "Run 'ralph --help' for usage.")
		os.Exit(1)
	}
}

func showHelp() {
	fmt.Printf(`ralph v%s - Autonomous AI agent loop for Amp

Usage: ralph <command> [options]

Commands:
  init              Initialize Ralph in the current project
  run [iterations]  Run the agent loop (default: 10 iterations)
  verify            Run verification only
  prd               Create a PRD document interactively
  convert <file>    Convert a PRD markdown file to prd.json
  status            Show PRD story status
  next              Show the next story to work on
  validate          Validate prd.json schema
  doctor            Check Ralph environment
  upgrade           Upgrade Ralph to the latest version

Options:
  -h, --help        Show this help message
  -v, --version     Show version number

Run Options:
  --no-verify       Skip auto-verification after completion

Examples:
  ralph init                    # Initialize in current project
  ralph prd                     # Create a new PRD interactively
  ralph convert tasks/prd.md    # Convert PRD to prd.json
  ralph run                     # Run with default 10 iterations
  ralph run 20                  # Run with 20 iterations
  ralph run --no-verify         # Skip verification after completion
  ralph verify                  # Run verification only
  ralph status                  # Check progress
  ralph upgrade                 # Update to latest version
`, Version)
}
