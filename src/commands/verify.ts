import { parseArgs } from "node:util";
import { resolveConfig } from "../config.js";
import { runLoop } from "../loop.js";

export async function verify(args: string[]) {
	const { values } = parseArgs({
		args,
		options: {
			verbose: { type: "boolean" },
		},
		allowPositionals: false,
	});

	const config = resolveConfig();

	await runLoop(config, {
		verifyOnly: true,
		verbose: values.verbose,
	});
}
