import { existsSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import type { ResolvedConfig } from "./config.js";

const __dirname = dirname(fileURLToPath(import.meta.url));

export function getTemplateDir(): string {
	const devPath = join(__dirname, "..", "templates");
	if (existsSync(devPath)) return devPath;

	const pkgPath = join(__dirname, "..", "..", "templates");
	if (existsSync(pkgPath)) return pkgPath;

	throw new Error("Could not find templates directory");
}

function readTemplate(name: string): string {
	const templatePath = join(getTemplateDir(), `${name}.md`);
	if (!existsSync(templatePath)) {
		throw new Error(`Template not found: ${templatePath}`);
	}
	return readFileSync(templatePath, "utf-8");
}

export function getTemplate(
	name: string,
	vars: Record<string, string | undefined>,
): string {
	const template = readTemplate(name);

	// Simple variable substitution
	return template.replace(/\{\{(\w+)\}\}/g, (_, key: string) => {
		return vars[key] ?? `{{${key}}}`;
	});
}

export function generateRunPrompt(config: ResolvedConfig): string {
	const qualityCommands =
		config.quality.length > 0
			? config.quality.map((q) => `   ${q.cmd}   # ${q.name}`).join("\n")
			: "   # No quality commands configured - check project setup";

	return getTemplate("run", {
		prdPath: config.prdPath,
		projectRoot: config.projectRoot,
		qualityCommands,
	});
}

export function generateVerifyPrompt(config: ResolvedConfig): string {
	const qualityCommands =
		config.quality.length > 0
			? config.quality.map((q) => `   ${q.cmd}   # ${q.name}`).join("\n")
			: "   # No quality commands configured - check project setup";

	return getTemplate("verify", {
		prdPath: config.prdPath,
		projectRoot: config.projectRoot,
		qualityCommands,
	});
}
