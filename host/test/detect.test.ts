// Detecção de comandos: raiz e monorepos (subpastas até 2 níveis).
import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { after, test } from "node:test";
import { detectCommands } from "../src/detect.ts";

const tmp = mkdtempSync(path.join(os.tmpdir(), "wb-detect-test-"));
after(() => rmSync(tmp, { recursive: true, force: true }));

const pkg = (dir: string, scripts: Record<string, string>) => {
	mkdirSync(dir, { recursive: true });
	writeFileSync(path.join(dir, "package.json"), JSON.stringify({ scripts }));
};

test("raiz: comandos sem cd", () => {
	const root = path.join(tmp, "raiz");
	pkg(root, { test: "x", lint: "x", start: "x" });
	assert.deepEqual(detectCommands(root), [
		{ command: "npm test", source: "package.json" },
		{ command: "npm run lint", source: "package.json" },
	]);
});

test("monorepo: subpastas com cd, pulando node_modules, ocultas, fundo demais e symlinks", () => {
	const root = path.join(tmp, "mono");
	pkg(path.join(root, "apps/mobile"), { test: "x", typecheck: "x" });
	mkdirSync(path.join(root, "apps/server"), { recursive: true });
	writeFileSync(path.join(root, "apps/server/go.mod"), "module x\n");
	pkg(path.join(root, "node_modules/dep"), { test: "x" });
	pkg(path.join(root, ".cache/x"), { test: "x" });
	pkg(path.join(root, "a/b/c"), { test: "x" });
	symlinkSync(path.join(root, "apps/mobile"), path.join(root, "link"));
	assert.deepEqual(
		detectCommands(root).map((c) => c.command),
		["cd apps/mobile && npm test", "cd apps/mobile && npm run typecheck", "cd apps/server && go build ./...", "cd apps/server && go vet ./...", "cd apps/server && go test ./..."],
	);
	assert.equal(detectCommands(root)[0].source, "apps/mobile/package.json");
});
