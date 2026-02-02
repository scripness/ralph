import { spawn } from "node:child_process";
import { existsSync, mkdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { parseArgs } from "node:util";
import { resolveConfig } from "../config.js";
import { getTemplate } from "../templates.js";

export async function convert(args: string[]) {
	const { values, positionals } = parseArgs({
		args,
		options: {
			output: { type: "string", short: "o" },
		},
		allowPositionals: true,
	});

	const prdFile = positionals[0];

	if (!prdFile) {
		console.error("Usage: ralph convert <prd-file.md> [-o output.json]");
		console.error("");
		console.error("Example: ralph convert tasks/prd-auth.md");
		process.exit(1);
	}

	const config = resolveConfig();
	const prdPath = join(config.projectRoot, prdFile);

	if (!existsSync(prdPath)) {
		console.error(`PRD file not found: ${prdPath}`);
		process.exit(1);
	}

	const prdContent = readFileSync(prdPath, "utf-8");

	// Default output to scripts/ralph/prd.json
	const outputPath =
		values.output ?? join(config.projectRoot, "scripts", "ralph", "prd.json");

	// Ensure output directory exists
	const outputDir = dirname(outputPath);
	if (!existsSync(outputDir)) {
		mkdirSync(outputDir, { recursive: true });
	}

	const prompt = getTemplate("convert", {
		prdContent,
		outputPath,
		projectRoot: config.projectRoot,
	});

	console.log(`Converting PRD: ${prdPath}`);
	console.log(`Output: ${outputPath}`);
	console.log("");

	return new Promise<void>((resolve, reject) => {
		const child = spawn(config.amp.command, config.amp.args, {
			cwd: config.projectRoot,
			stdio: ["pipe", "inherit", "inherit"],
		});

		child.on("close", (code) => {
			if (code === 0) {
				console.log("");
				console.log("Conversion complete!");
				console.log(`  prd.json: ${outputPath}`);
				console.log("");
				console.log("Next: Run 'ralph run' to start implementing stories");
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
