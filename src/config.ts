import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { z } from "zod";
import {
	type ProjectInfo,
	type QualityCommand,
	detectProject,
} from "./detect.js";

const qualityCommandSchema = z.object({
	name: z.string(),
	cmd: z.string(),
});

const configSchema = z.object({
	prdPath: z.string().optional(),
	iterations: z.number().optional(),
	verify: z.boolean().optional(),
	quality: z.array(qualityCommandSchema).optional(),
	amp: z
		.object({
			command: z.string().optional(),
			args: z.array(z.string()).optional(),
		})
		.optional(),
});

export type RalphConfig = z.infer<typeof configSchema>;

export interface ResolvedConfig {
	prdPath: string;
	iterations: number;
	verify: boolean;
	quality: QualityCommand[];
	amp: {
		command: string;
		args: string[];
	};
	projectRoot: string;
	projectType: ProjectInfo["type"];
}

function loadConfigFile(projectRoot: string): RalphConfig | null {
	const paths = [
		join(projectRoot, "ralph.config.json"),
		join(projectRoot, "scripts", "ralph", "ralph.config.json"),
	];

	for (const path of paths) {
		if (existsSync(path)) {
			try {
				const content = JSON.parse(readFileSync(path, "utf-8"));
				const result = configSchema.safeParse(content);
				if (result.success) {
					return result.data;
				}
				console.warn(`Warning: Invalid config at ${path}`);
			} catch {
				console.warn(`Warning: Could not parse ${path}`);
			}
		}
	}
	return null;
}

export interface ConfigOptions {
	iterations?: number;
	verify?: boolean;
	prdPath?: string;
}

export function resolveConfig(options: ConfigOptions = {}): ResolvedConfig {
	const project = detectProject();
	const fileConfig = loadConfigFile(project.root);

	const prdPath =
		options.prdPath ??
		fileConfig?.prdPath ??
		join(project.root, "scripts", "ralph", "prd.json");

	const iterations = options.iterations ?? fileConfig?.iterations ?? 10;
	const verify = options.verify ?? fileConfig?.verify ?? true;

	const quality = fileConfig?.quality ?? project.qualityCommands;

	const amp = {
		command: fileConfig?.amp?.command ?? "amp",
		args: fileConfig?.amp?.args ?? ["--dangerously-allow-all"],
	};

	return {
		prdPath,
		iterations,
		verify,
		quality,
		amp,
		projectRoot: project.root,
		projectType: project.type,
	};
}
