import assert from "node:assert/strict";
import { chmodSync, mkdirSync, mkdtempSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { after, test } from "node:test";
import { resolveWikiDir, showWikiPage, wikiIndexes, wikiSearch } from "../src/wiki.ts";
import { loadProjectConfig } from "../src/project-config.ts";
import { decideToolCall, workbenchExtension } from "../src/extension.ts";
import { describeScope } from "../src/host.ts";
import { resolveScope } from "../src/sandbox-profiles.ts";
import { STEPS, stepScope } from "../src/workflows.ts";

const tmp = mkdtempSync(path.join(os.tmpdir(), "wb-wiki-test-"));
after(() => rmSync(tmp, { recursive: true, force: true }));

function wiki(name: string) {
	const root = path.join(tmp, name, "wiki");
	mkdirSync(path.join(root, "sub"), { recursive: true });
	mkdirSync(path.join(root, "alfa"), { recursive: true });
	writeFileSync(path.join(root, "alfa/index.md"), "# Alfa\n");
	writeFileSync(path.join(root, "index.md"), "# Root\n");
	writeFileSync(path.join(root, "sub/index.md"), "# Nested\n");
	writeFileSync(path.join(root, "sub/page.md"), "---\ntitle: {en: Sample}\ndescription: details\nstatus: draft\nstale_after: 2000-01-01T00:00:00Z\n---\n⚠️ Divergência\nNeedle\n");
	return root;
}

test("resolveWikiDir aplica env, config do usuário e padrão, validando raiz", () => {
	const root = wiki("precedence");
	const home = path.join(tmp, "home");
	mkdirSync(path.join(home, ".config/workbench"), { recursive: true });
	writeFileSync(path.join(home, ".config/workbench/config.json"), JSON.stringify({ kbAi: { wikiDir: root } }));
	assert.equal(resolveWikiDir({ env: { WB_WIKI_DIR: root }, home }), root);
	assert.equal(resolveWikiDir({ env: {}, home }), root);
	assert.throws(() => resolveWikiDir({ env: { WB_WIKI_DIR: path.join(tmp, "missing") }, home }), /diretório/);
	assert.throws(() => resolveWikiDir({ env: { WB_WIKI_DIR: "" }, home }), /vazio/);
	assert.throws(() => resolveWikiDir({ env: { WB_WIKI_DIR: path.dirname(root) }, home }), /diretório wiki/);
	assert.throws(() => resolveWikiDir({ env: {}, home: path.join(tmp, "empty-home") }), /diretório/);
});

test("busca limita-se a markdown sob raiz e show rejeita escapes", () => {
	const root = wiki("boundary");
	writeFileSync(path.join(root, "sub/other.txt"), "Needle");
	writeFileSync(path.join(root, "sub/invalid.md"), "---\nstale_after: not-a-date\n---\nInvalidNeedle\n");
	const outside = path.join(tmp, "outside.md");
	writeFileSync(outside, "Needle");
	symlinkSync(outside, path.join(root, "escape.md"));
	const results = wikiSearch(root, "needle");
	assert.deepEqual(results.map((r) => r.path), ["sub/invalid.md", "sub/page.md"]);
	const invalidDate = wikiSearch(root, "invalidneedle")[0];
	assert.equal(invalidDate.quality, "stale_after inválido");
	assert.equal(invalidDate.title, "(ausente)");
	assert.equal(invalidDate.description, "(ausente)");
	assert.deepEqual(Object.keys(results[0]), ["path", "title", "description", "quality"]);
	assert.deepEqual(results.find((r) => r.path === "sub/page.md"), { path: "sub/page.md", title: "Sample", description: "details", quality: "stale (venceu 2000-01-01), draft, divergência" });
	assert.throws(() => showWikiPage(root, "../outside.md"), /indisponível/);
	assert.throws(() => showWikiPage(root, path.join(root, "index.md")), /indisponível/);
	assert.equal(showWikiPage(root, "index.md"), "# Root\n");
	assert.deepEqual(wikiIndexes(root).map((i) => i.path), ["index.md", "alfa/index.md", "sub/index.md"]);
});

test("extensão registra wiki somente com opt-in", () => {
	const register = (enabled: boolean) => {
		const names: string[] = [];
		const pi = { registerTool: (tool: { name: string }) => names.push(tool.name), on: () => undefined };
		workbenchExtension({ root: tmp, scope: { wiki: enabled } as never, gate: {} as never, audit: (() => {}) as never })(pi as never);
		return names;
	};
	assert.ok(!register(false).includes("wiki"));
	assert.ok(register(true).includes("wiki"));
});

test("opt-in da wiki faz parte do escopo e aparece na descrição", () => {
	const on = { kbAi: { wiki: true } };
	assert.equal(resolveScope("read-only", {}).wiki, false);
	assert.equal(resolveScope("read-only", on).wiki, true);
	assert.equal(stepScope(STEPS.review, {}).wiki, false);
	assert.equal(stepScope(STEPS.review, on).wiki, true);
	assert.match(describeScope(resolveScope("read-only", on), []), /Wiki \(kb-ai\): consulta só leitura/);
	assert.match(describeScope(resolveScope("read-only", {}), []), /Wiki \(kb-ai\): desabilitada/);
});

test("decideToolCall só permite wiki com opt-in", () => {
	const calls: unknown[] = [];
	const gate = { decide: (...args: unknown[]) => { calls.push(args); return args[2] ? undefined : { block: true, reason: "denied" }; } };
	const options = { root: tmp, scope: { wiki: false } as never, gate: gate as never, audit: (() => {}) as never };
	assert.deepEqual(decideToolCall("wiki", { action: "search" }, options), { block: true, reason: "denied" });
	assert.equal(decideToolCall("wiki", { action: "search" }, { ...options, scope: { wiki: true } as never }), undefined);
	assert.equal(calls.length, 2);
});

test("config kbAi malformada ou com chaves desconhecidas é erro", () => {
	const root = path.join(tmp, "config-project");
	mkdirSync(root);
	for (const value of ['{"kbAi":true}', '{"kbAi":{"wiki":"true"}}', '{"kbAi":{"wiki":true,"other":1}}']) {
		writeFileSync(path.join(root, ".workbench.json"), value);
		assert.throws(() => loadProjectConfig(root), /kbAi/);
	}
	writeFileSync(path.join(root, ".workbench.json"), '{"kbAi":{"wiki":true}}');
	assert.equal(loadProjectConfig(root).kbAi?.wiki, true);
});

test("pasta ilegível fica de fora sem derrubar busca e índices", { skip: process.getuid?.() === 0 && "root lê tudo" }, () => {
	const root = wiki("ilegivel");
	mkdirSync(path.join(root, "trancada"));
	writeFileSync(path.join(root, "trancada/index.md"), "# Trancada Needle\n");
	chmodSync(path.join(root, "trancada"), 0o000);
	try {
		assert.deepEqual(wikiIndexes(root).map((i) => i.path), ["index.md", "alfa/index.md", "sub/index.md"]);
		assert.deepEqual(wikiSearch(root, "needle").map((r) => r.path), ["sub/page.md"]);
	} finally {
		chmodSync(path.join(root, "trancada"), 0o755);
	}
});
