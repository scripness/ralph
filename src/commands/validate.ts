import { existsSync, readFileSync } from "node:fs";
import { resolveConfig } from "../config.js";
import { validatePRD } from "../schema.js";

export async function validate(_args: string[]) {
	const config = resolveConfig();

	if (!existsSync(config.prdPath)) {
		console.error(`PRD not found: ${config.prdPath}`);
		process.exit(1);
	}

	try {
		const content = JSON.parse(readFileSync(config.prdPath, "utf-8"));
		const result = validatePRD(content);

		if (!result.success) {
			console.error(result.error);
			process.exit(1);
		}

		console.log("✓ prd.json is valid");
		console.log(`  - ${result.data.userStories.length} stories`);
		console.log(`  - Schema version: ${result.data.schemaVersion}`);
	} catch (e) {
		console.error(`Failed to parse prd.json: ${e}`);
		process.exit(1);
	}
}
