import { describe, expect, test } from "bun:test";
import { existsSync } from "node:fs";
import { join } from "node:path";
import { detectProject } from "../src/detect";

describe("detectProject", () => {
	test("detects bun project from current directory", () => {
		const project = detectProject(process.cwd());
		expect(project.type).toBe("bun");
		expect(project.qualityCommands.length).toBeGreaterThan(0);
	});

	test("finds typecheck command", () => {
		const project = detectProject(process.cwd());
		const typecheck = project.qualityCommands.find(
			(c) => c.name === "typecheck",
		);
		expect(typecheck).toBeDefined();
		expect(typecheck?.cmd).toContain("typecheck");
	});

	test("finds lint command", () => {
		const project = detectProject(process.cwd());
		const lint = project.qualityCommands.find((c) => c.name === "lint");
		expect(lint).toBeDefined();
	});

	test("finds test command", () => {
		const project = detectProject(process.cwd());
		const testCmd = project.qualityCommands.find((c) => c.name === "test");
		expect(testCmd).toBeDefined();
	});

	test("returns unknown for non-project directory", () => {
		const project = detectProject("/tmp");
		expect(project.type).toBe("unknown");
		expect(project.qualityCommands.length).toBe(0);
	});
});
