#!/usr/bin/env bun

import { existsSync, readFileSync } from "node:fs";
import { parseArgs } from "node:util";
import * as commands from "./commands/index.js";
import { resolveConfig } from "./config.js";

const VERSION = "1.0.0";

function showHelp() {
	console.log(`
ralph v${VERSION} - Autonomous AI agent loop for Amp

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

Options:
  -h, --help        Show this help message
  -v, --version     Show version number

Run Options:
  --no-verify       Skip auto-verification after completion
  --verbose         Show detailed output

Examples:
  ralph init                    # Initialize in current project
  ralph prd                     # Create a new PRD interactively
  ralph convert tasks/prd.md    # Convert PRD to prd.json
  ralph run                     # Run with default 10 iterations
  ralph run 20                  # Run with 20 iterations
  ralph run --no-verify         # Skip verification after completion
  ralph verify                  # Run verification only
  ralph status                  # Check progress
`);
}

async function main() {
	const args = process.argv.slice(2);

	if (args.length === 0) {
		showHelp();
		process.exit(0);
	}

	const command = args[0];

	// Handle global flags
	if (command === "-h" || command === "--help") {
		showHelp();
		process.exit(0);
	}

	if (command === "-v" || command === "--version") {
		console.log(`ralph v${VERSION}`);
		process.exit(0);
	}

	// Route to command handlers
	try {
		switch (command) {
			case "init":
				await commands.init(args.slice(1));
				break;

			case "run":
				await commands.run(args.slice(1));
				break;

			case "verify":
				await commands.verify(args.slice(1));
				break;

			case "prd":
				await commands.prd(args.slice(1));
				break;

			case "convert":
				await commands.convert(args.slice(1));
				break;

			case "status":
				await commands.status(args.slice(1));
				break;

			case "next":
				await commands.next(args.slice(1));
				break;

			case "validate":
				await commands.validate(args.slice(1));
				break;

			case "doctor":
				await commands.doctor(args.slice(1));
				break;

			default:
				console.error(`Unknown command: ${command}`);
				console.error("Run 'ralph --help' for usage.");
				process.exit(1);
		}
	} catch (error) {
		console.error(
			"Error:",
			error instanceof Error ? error.message : String(error),
		);
		process.exit(1);
	}
}

main();
