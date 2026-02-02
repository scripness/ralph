import { existsSync, readFileSync } from "node:fs";
import { resolveConfig } from "../config.js";
import { getNextStory, validatePRD } from "../schema.js";

export async function next(_args: string[]) {
	const config = resolveConfig();

	if (!existsSync(config.prdPath)) {
		console.error(`PRD not found: ${config.prdPath}`);
		process.exit(1);
	}

	const content = JSON.parse(readFileSync(config.prdPath, "utf-8"));
	const result = validatePRD(content);

	if (!result.success) {
		console.error(result.error);
		process.exit(1);
	}

	const nextStory = getNextStory(result.data);

	if (!nextStory) {
		console.log("All stories complete!");
		return;
	}

	console.log(`${nextStory.id}: ${nextStory.title}`);
	console.log(`Priority: ${nextStory.priority}`);
	console.log(`Retries: ${nextStory.retries}`);
	if (nextStory.notes) {
		console.log(`Notes: ${nextStory.notes}`);
	}
	console.log("");
	console.log("Acceptance Criteria:");
	for (const criterion of nextStory.acceptanceCriteria) {
		console.log(`  - ${criterion}`);
	}
}
