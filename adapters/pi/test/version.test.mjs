import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));

test("adapter CLI version matches protocol SSoT", () => {
	const versionPath = path.join(here, "..", "..", "..", "protocol", "command-code-version");
	const expected = fs.readFileSync(versionPath, "utf8").trim();
	const source = fs.readFileSync(path.join(here, "..", "src", "index.ts"), "utf8");
	assert.match(source, new RegExp(`const COMMAND_CODE_VERSION = "${expected}"`));
});

test("adapter model ids match static catalog", () => {
	const catalogPath = path.join(here, "..", "..", "..", "protocol", "catalog", "static-models-v0.52.1.json");
	const catalog = JSON.parse(fs.readFileSync(catalogPath, "utf8"));
	const source = fs.readFileSync(path.join(here, "..", "src", "index.ts"), "utf8");
	for (const model of catalog.models) {
		assert.match(source, new RegExp(`id: "${model.id.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}"`));
	}
});
