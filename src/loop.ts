import { spawn } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import type { ResolvedConfig } from "./config.js";
import { type PRD, getNextStory, validatePRD } from "./schema.js";
import { generateRunPrompt, generateVerifyPrompt } from "./templates.js";

const COMPLETE_SIGNAL = "<promise>COMPLETE</promise>";

function loadPRD(path: string): PRD {
	if (!existsSync(path)) {
		throw new Error(
			`PRD not found: ${path}\n\nRun 'ralph init' to create one.`,
		);
	}

	const content = JSON.parse(readFileSync(path, "utf-8"));
	const result = validatePRD(content);

	if (!result.success) {
		throw new Error(result.error);
	}

	return result.data;
}

async function runAmpIteration(
	config: ResolvedConfig,
	prompt: string,
): Promise<{ output: string; complete: boolean }> {
	return new Promise((resolve, reject) => {
		const child = spawn(config.amp.command, config.amp.args, {
			cwd: config.projectRoot,
			stdio: ["pipe", "pipe", "pipe"],
			env: { ...process.env },
		});

		let output = "";

		child.stdout.on("data", (data) => {
			const text = data.toString();
			output += text;
			process.stdout.write(text);
		});

		child.stderr.on("data", (data) => {
			const text = data.toString();
			output += text;
			process.stderr.write(text);
		});

		child.on("close", (_code) => {
			const complete = output.includes(COMPLETE_SIGNAL);
			resolve({ output, complete });
		});

		child.on("error", (err) => {
			reject(new Error(`Failed to spawn amp: ${err.message}`));
		});

		child.stdin.write(prompt);
		child.stdin.end();
	});
}

async function runVerify(config: ResolvedConfig): Promise<boolean> {
	console.log(`\n${"=".repeat(60)}`);
	console.log(" Ralph Verify Mode");
	console.log("=".repeat(60));

	const verifyPrompt = generateVerifyPrompt(config);

	return new Promise((resolve) => {
		const child = spawn(config.amp.command, config.amp.args, {
			cwd: config.projectRoot,
			stdio: ["pipe", "inherit", "inherit"],
		});

		child.on("close", (code) => {
			resolve(code === 0);
		});

		child.stdin.write(verifyPrompt);
		child.stdin.end();
	});
}

export interface LoopOptions {
	verifyOnly?: boolean;
	noVerify?: boolean;
	verbose?: boolean;
}

export async function runLoop(
	config: ResolvedConfig,
	options: LoopOptions = {},
): Promise<void> {
	// Verify-only mode
	if (options.verifyOnly) {
		const prd = loadPRD(config.prdPath);
		console.log(`PRD: ${config.prdPath}`);
		console.log(`Project: ${prd.project}`);
		await runVerify(config);
		return;
	}

	console.log("=".repeat(60));
	console.log(" Ralph - Autonomous Agent Loop");
	console.log("=".repeat(60));
	console.log(` Max iterations: ${config.iterations}`);
	console.log(` PRD: ${config.prdPath}`);
	console.log(` Project root: ${config.projectRoot}`);
	console.log("=".repeat(60));

	const runPrompt = generateRunPrompt(config);

	for (let i = 1; i <= config.iterations; i++) {
		console.log(`\n${"=".repeat(60)}`);
		console.log(` Ralph Iteration ${i} of ${config.iterations}`);
		console.log("=".repeat(60));

		const prd = loadPRD(config.prdPath);
		const nextStory = getNextStory(prd);

		if (!nextStory) {
			console.log("All stories already complete.");
			break;
		}

		console.log(`\nNext story: ${nextStory.id} - ${nextStory.title}`);

		const { complete } = await runAmpIteration(config, runPrompt);

		if (complete) {
			// Re-read PRD to verify all stories pass
			const updatedPrd = loadPRD(config.prdPath);
			const incomplete = updatedPrd.userStories.filter((s) => !s.passes);

			if (incomplete.length > 0) {
				console.log("\nWarning: COMPLETE signal but these stories still fail:");
				for (const s of incomplete) {
					console.log(`  - ${s.id}: ${s.title}`);
				}
				console.log("Continuing...");
				continue;
			}

			console.log(`\n${"=".repeat(60)}`);
			console.log(" Ralph completed all stories!");
			console.log(` Completed at iteration ${i} of ${config.iterations}`);
			console.log("=".repeat(60));

			// Run verification unless --no-verify
			if (!options.noVerify && config.verify) {
				console.log("\nRunning verification...");
				await runVerify(config);

				// Check if verification reset any stories
				const verifiedPrd = loadPRD(config.prdPath);
				const resetStories = verifiedPrd.userStories.filter((s) => !s.passes);

				if (resetStories.length > 0) {
					console.log(`\n${"=".repeat(60)}`);
					console.log(" Verification found issues - stories reset for retry:");
					for (const s of resetStories) {
						console.log(`  - ${s.id}: ${s.title}`);
					}
					console.log("=".repeat(60));
					console.log("\nRestarting loop...");
					continue;
				}
			}

			console.log(`\n${"=".repeat(60)}`);
			console.log(" Ralph completed and verified!");
			console.log("=".repeat(60));
			console.log("\nResults:");
			console.log(`  - PRD: ${config.prdPath}`);
			console.log("  - Git log: git log --oneline -20");
			console.log("\nReady to merge.");
			return;
		}

		console.log(`\nIteration ${i} complete. Continuing...`);
	}

	console.log(`\n${"=".repeat(60)}`);
	console.log(` Ralph reached max iterations (${config.iterations})`);
	console.log(" Not all tasks completed - check prd.json for status");
	console.log("=".repeat(60));
	console.log(`\nTo continue: ralph run ${config.iterations}`);
	process.exit(1);
}
