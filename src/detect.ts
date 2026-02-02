import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";

export interface QualityCommand {
	name: string;
	cmd: string;
}

export interface ProjectInfo {
	type: "bun" | "node" | "cargo" | "go" | "unknown";
	root: string;
	qualityCommands: QualityCommand[];
}

function detectGitRoot(cwd: string): string | null {
	let dir = cwd;
	while (dir !== "/") {
		if (existsSync(join(dir, ".git"))) {
			return dir;
		}
		dir = join(dir, "..");
	}
	return null;
}

function detectBunProject(root: string): QualityCommand[] | null {
	const pkgPath = join(root, "package.json");
	const lockPath = join(root, "bun.lock");

	if (!existsSync(pkgPath)) return null;

	const isBun = existsSync(lockPath);
	const runner = isBun ? "bun run" : "npm run";

	try {
		const pkg = JSON.parse(readFileSync(pkgPath, "utf-8"));
		const scripts = pkg.scripts ?? {};
		const commands: QualityCommand[] = [];

		if (scripts.typecheck) {
			commands.push({ name: "typecheck", cmd: `${runner} typecheck` });
		} else if (existsSync(join(root, "tsconfig.json"))) {
			commands.push({
				name: "typecheck",
				cmd: isBun ? "bun run tsc --noEmit" : "npx tsc --noEmit",
			});
		}

		if (scripts.lint) {
			commands.push({ name: "lint", cmd: `${runner} lint` });
		}

		if (scripts.test) {
			commands.push({ name: "test", cmd: `${runner} test` });
		}

		return commands.length > 0 ? commands : null;
	} catch {
		return null;
	}
}

function detectCargoProject(root: string): QualityCommand[] | null {
	if (!existsSync(join(root, "Cargo.toml"))) return null;

	return [
		{ name: "check", cmd: "cargo check" },
		{ name: "clippy", cmd: "cargo clippy -- -D warnings" },
		{ name: "test", cmd: "cargo test" },
	];
}

function detectGoProject(root: string): QualityCommand[] | null {
	if (!existsSync(join(root, "go.mod"))) return null;

	return [
		{ name: "build", cmd: "go build ./..." },
		{ name: "vet", cmd: "go vet ./..." },
		{ name: "test", cmd: "go test ./..." },
	];
}

export function detectProject(cwd: string = process.cwd()): ProjectInfo {
	const root = detectGitRoot(cwd) ?? cwd;

	const bunCommands = detectBunProject(root);
	if (bunCommands) {
		const isBun = existsSync(join(root, "bun.lock"));
		return { type: isBun ? "bun" : "node", root, qualityCommands: bunCommands };
	}

	const cargoCommands = detectCargoProject(root);
	if (cargoCommands) {
		return { type: "cargo", root, qualityCommands: cargoCommands };
	}

	const goCommands = detectGoProject(root);
	if (goCommands) {
		return { type: "go", root, qualityCommands: goCommands };
	}

	return { type: "unknown", root, qualityCommands: [] };
}
