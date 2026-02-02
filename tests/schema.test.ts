import { describe, expect, test } from "bun:test";
import {
	allStoriesComplete,
	getNextStory,
	validatePRD,
	type PRD,
} from "../src/schema";

const validPrd: PRD = {
	schemaVersion: 2,
	project: "TestProject",
	branchName: "ralph/test",
	description: "Test PRD",
	run: {
		startedAt: null,
		currentStoryId: null,
		learnings: [],
	},
	userStories: [
		{
			id: "US-001",
			title: "Test Story",
			description: "A test story",
			acceptanceCriteria: ["Criterion 1"],
			priority: 1,
			passes: false,
			retries: 0,
			lastResult: null,
			notes: "",
		},
	],
};

describe("validatePRD", () => {
	test("accepts valid prd", () => {
		const result = validatePRD(validPrd);
		expect(result.success).toBe(true);
	});

	test("rejects missing schemaVersion", () => {
		const invalid = { ...validPrd, schemaVersion: undefined };
		const result = validatePRD(invalid);
		expect(result.success).toBe(false);
	});

	test("rejects wrong schemaVersion", () => {
		const invalid = { ...validPrd, schemaVersion: 1 };
		const result = validatePRD(invalid);
		expect(result.success).toBe(false);
	});

	test("rejects empty userStories", () => {
		const invalid = { ...validPrd, userStories: [] };
		const result = validatePRD(invalid);
		expect(result.success).toBe(false);
	});

	test("rejects missing story fields", () => {
		const invalid = {
			...validPrd,
			userStories: [{ id: "US-001", title: "Incomplete" }],
		};
		const result = validatePRD(invalid);
		expect(result.success).toBe(false);
	});
});

describe("getNextStory", () => {
	test("returns highest priority incomplete story", () => {
		const prd: PRD = {
			...validPrd,
			userStories: [
				{ ...validPrd.userStories[0], id: "US-001", priority: 2 },
				{
					...validPrd.userStories[0],
					id: "US-002",
					priority: 1,
					passes: false,
				},
				{
					...validPrd.userStories[0],
					id: "US-003",
					priority: 3,
					passes: false,
				},
			],
		};
		const next = getNextStory(prd);
		expect(next?.id).toBe("US-002");
	});

	test("skips completed stories", () => {
		const prd: PRD = {
			...validPrd,
			userStories: [
				{
					...validPrd.userStories[0],
					id: "US-001",
					priority: 1,
					passes: true,
				},
				{
					...validPrd.userStories[0],
					id: "US-002",
					priority: 2,
					passes: false,
				},
			],
		};
		const next = getNextStory(prd);
		expect(next?.id).toBe("US-002");
	});

	test("returns null when all complete", () => {
		const prd: PRD = {
			...validPrd,
			userStories: [
				{
					...validPrd.userStories[0],
					id: "US-001",
					passes: true,
				},
			],
		};
		const next = getNextStory(prd);
		expect(next).toBeNull();
	});
});

describe("allStoriesComplete", () => {
	test("returns true when all pass", () => {
		const prd: PRD = {
			...validPrd,
			userStories: [
				{ ...validPrd.userStories[0], passes: true },
				{ ...validPrd.userStories[0], id: "US-002", passes: true },
			],
		};
		expect(allStoriesComplete(prd)).toBe(true);
	});

	test("returns false when any fail", () => {
		const prd: PRD = {
			...validPrd,
			userStories: [
				{ ...validPrd.userStories[0], passes: true },
				{ ...validPrd.userStories[0], id: "US-002", passes: false },
			],
		};
		expect(allStoriesComplete(prd)).toBe(false);
	});
});
