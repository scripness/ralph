import { spawn } from "node:child_process";
import { existsSync, mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { parseArgs } from "node:util";
import { resolveConfig } from "../config.js";
import { getTemplate } from "../templates.js";

export async function prd(args: string[]) {
	const { values, positionals } = parseArgs({
		args,
		options: {
			output: { type: "string", short: "o" },
		},
		allowPositionals: true,
	});

	const config = resolveConfig();
	const featureName = positionals[0];

	// Default output to tasks/prd-{feature}.md or tasks/prd.md
	const outputPath =
		values.output ??
		join(
			config.projectRoot,
			"tasks",
			featureName ? `prd-${featureName}.md` : "prd.md",
		);

	// Ensure tasks directory exists
	const tasksDir = dirname(outputPath);
	if (!existsSync(tasksDir)) {
		mkdirSync(tasksDir, { recursive: true });
	}

	const prompt = getTemplate("prd", {
		outputPath,
		featureName,
		projectRoot: config.projectRoot,
	});

	console.log("Starting PRD creation...");
	console.log(`Output will be saved to: ${outputPath}`);
	console.log("");

	return new Promise<void>((resolve, reject) => {
		const child = spawn(config.amp.command, config.amp.args, {
			cwd: config.projectRoot,
			stdio: ["pipe", "inherit", "inherit"],
		});

		child.on("close", (code) => {
			if (code === 0) {
				console.log("");
				console.log("PRD created successfully!");
				console.log(`  File: ${outputPath}`);
				console.log("");
				console.log(
					`Next: Run 'ralph convert ${outputPath}' to generate prd.json`,
				);
				resolve();
			} else {
				reject(new Error(`Amp exited with code ${code}`));
			}
		});

		child.on("error", (err) => {
			reject(new Error(`Failed to spawn amp: ${err.message}`));
		});

		child.stdin.write(prompt);
		child.stdin.end();
	});
}
