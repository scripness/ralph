import { z } from "zod";

export const lastResultSchema = z.object({
	completedAt: z.string(),
	thread: z.string(),
	commit: z.string(),
	summary: z.string(),
});

export const userStorySchema = z.object({
	id: z.string(),
	title: z.string(),
	description: z.string(),
	acceptanceCriteria: z.array(z.string()),
	priority: z.number(),
	passes: z.boolean(),
	retries: z.number(),
	lastResult: lastResultSchema.nullable(),
	notes: z.string(),
});

export const runSchema = z.object({
	startedAt: z.string().nullable(),
	currentStoryId: z.string().nullable(),
	learnings: z.array(z.string()),
});

export const prdSchema = z.object({
	schemaVersion: z.literal(2),
	project: z.string(),
	branchName: z.string(),
	description: z.string(),
	run: runSchema,
	userStories: z.array(userStorySchema).min(1),
});

export type LastResult = z.infer<typeof lastResultSchema>;
export type UserStory = z.infer<typeof userStorySchema>;
export type Run = z.infer<typeof runSchema>;
export type PRD = z.infer<typeof prdSchema>;

export function validatePRD(
	data: unknown,
): { success: true; data: PRD } | { success: false; error: string } {
	const result = prdSchema.safeParse(data);
	if (result.success) {
		return { success: true, data: result.data };
	}
	const errors = result.error.issues
		.map((issue) => `  - ${issue.path.join(".")}: ${issue.message}`)
		.join("\n");
	return { success: false, error: `Invalid prd.json:\n${errors}` };
}

export function getNextStory(prd: PRD): UserStory | null {
	const incomplete = prd.userStories
		.filter((s) => !s.passes)
		.sort((a, b) => a.priority - b.priority);
	return incomplete[0] ?? null;
}

export function allStoriesComplete(prd: PRD): boolean {
	return prd.userStories.every((s) => s.passes);
}
