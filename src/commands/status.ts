import { existsSync, readFileSync } from "node:fs";
import { resolveConfig } from "../config.js";
import { validatePRD } from "../schema.js";

export async function status(_args: string[]) {
	const config = resolveConfig();

	if (!existsSync(config.prdPath)) {
		console.error(`PRD not found: ${config.prdPath}`);
		console.error("Run 'ralph init' to create one.");
		process.exit(1);
	}

	const content = JSON.parse(readFileSync(config.prdPath, "utf-8"));
	const result = validatePRD(content);

	if (!result.success) {
		console.error(result.error);
		process.exit(1);
	}

	const prd = result.data;

	console.log(`Project: ${prd.project}`);
	console.log(`Branch: ${prd.branchName}`);
	console.log(`Description: ${prd.description}`);
	console.log("");

	const complete = prd.userStories.filter((s) => s.passes).length;
	const total = prd.userStories.length;
	console.log(`Progress: ${complete}/${total} stories complete`);
	console.log("");

	console.log("Stories:");
	for (const story of prd.userStories) {
		const status = story.passes ? "✓" : "○";
		const retries = story.retries > 0 ? ` (retries: ${story.retries})` : "";
		console.log(`  ${status} ${story.id}: ${story.title}${retries}`);
		if (story.notes) {
			console.log(`    └─ Note: ${story.notes}`);
		}
	}
}
