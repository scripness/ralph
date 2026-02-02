import { execSync } from "node:child_process";
import { existsSync } from "node:fs";
import { resolveConfig } from "../config.js";

export async function doctor(_args: string[]) {
	const config = resolveConfig();
	let issues = 0;

	console.log("Ralph Environment Check\n");

	// Check amp is available
	try {
		execSync(`${config.amp.command} --version`, { stdio: "pipe" });
		console.log(`✓ Amp command: ${config.amp.command}`);
	} catch {
		console.log(`✗ Amp command not found: ${config.amp.command}`);
		issues++;
	}

	// Check project root
	console.log(`✓ Project root: ${config.projectRoot}`);
	console.log(`✓ Project type: ${config.projectType}`);

	// Check PRD
	if (existsSync(config.prdPath)) {
		console.log(`✓ PRD file: ${config.prdPath}`);
	} else {
		console.log(`○ PRD file: ${config.prdPath} (not found - run 'ralph init')`);
	}

	// Check quality commands
	console.log("");
	console.log("Quality commands:");
	if (config.quality.length === 0) {
		console.log("  (none detected - configure in ralph.config.json)");
	} else {
		for (const cmd of config.quality) {
			console.log(`  - ${cmd.name}: ${cmd.cmd}`);
		}
	}

	if (issues > 0) {
		console.log(`\n${issues} issue(s) found.`);
		process.exit(1);
	} else {
		console.log("\nAll checks passed.");
	}
}
