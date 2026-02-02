import { parseArgs } from "node:util";
import { resolveConfig } from "../config.js";
import { runLoop } from "../loop.js";

export async function run(args: string[]) {
	const { values, positionals } = parseArgs({
		args,
		options: {
			"no-verify": { type: "boolean" },
			verbose: { type: "boolean" },
		},
		allowPositionals: true,
	});

	const iterations = positionals[0] ? Number.parseInt(positionals[0], 10) : 10;

	if (Number.isNaN(iterations) || iterations < 1) {
		console.error("Invalid iterations value. Must be a positive number.");
		process.exit(1);
	}

	const config = resolveConfig({
		iterations,
		verify: !values["no-verify"],
	});

	await runLoop(config, {
		noVerify: values["no-verify"],
		verbose: values.verbose,
	});
}
