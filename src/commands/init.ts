import { cpSync, existsSync, mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { parseArgs } from "node:util";
import { resolveConfig } from "../config.js";
import { getTemplateDir } from "../templates.js";

export async function init(args: string[]) {
	const { values } = parseArgs({
		args,
		options: {
			force: { type: "boolean", short: "f" },
		},
		allowPositionals: false,
	});

	const config = resolveConfig();
	const ralphDir = join(config.projectRoot, "scripts", "ralph");
	const prdPath = join(ralphDir, "prd.json");

	if (existsSync(prdPath) && !values.force) {
		console.error(`prd.json already exists at ${prdPath}`);
		console.error("Use --force to overwrite.");
		process.exit(1);
	}

	mkdirSync(ralphDir, { recursive: true });

	// Copy example PRD
	const templateDir = getTemplateDir();
	cpSync(join(templateDir, "prd.json.example"), prdPath);

	// Create .gitignore
	const gitignorePath = join(ralphDir, ".gitignore");
	if (!existsSync(gitignorePath)) {
		writeFileSync(gitignorePath, "*.backup\n*.tmp\n");
	}

	console.log("Initialized Ralph:");
	console.log(`  - Created ${prdPath}`);
	console.log(`  - Created ${gitignorePath}`);
	console.log("");
	console.log("Next steps:");
	console.log("  1. Run 'ralph prd' to create a PRD interactively, or");
	console.log("  2. Edit prd.json directly with your user stories");
	console.log("  3. Run 'ralph run' to start the agent loop");
}
