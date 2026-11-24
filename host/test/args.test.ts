// Parser da CLI: flags em qualquer posição, --flag=valor, "--", valores inválidos.
import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync, existsSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { after, test } from "node:test";
import { parseArgs } from "../src/args.ts";
import { approvalStatus, loadState } from "../src/workflows.ts";

test("flags antes ou depois do comando, nas duas formas", () => {
	const a = parseArgs(["-C", "/p", "step", "run", "build", "--sandbox=docs-only", "-m", "x/y", "--thinking", "low", "--no-input", "só", "o", "lote", "2"], "/cwd");
	assert.deepEqual(a.rest, ["step", "run", "build", "só", "o", "lote", "2"]);
	assert.equal(a.project, "/p");
	assert.equal(a.sandbox, "docs-only");
	assert.equal(a.model, "x/y");
	assert.equal(a.thinking, "low");
	assert.equal(a.noInput, true);
	assert.equal(parseArgs(["status"], "/cwd").project, "/cwd", "projeto padrão é o diretório atual");
});

test("-- encerra as opções", () => {
	assert.deepEqual(parseArgs(["run", "--", "--sandbox", "é", "texto"]).rest, ["run", "--sandbox", "é", "texto"]);
});

test("valores ausentes, vazios ou inválidos dão erro", () => {
	assert.throws(() => parseArgs(["run", "--sandbox"]), /precisa de um valor/);
	assert.throws(() => parseArgs(["run", "--sandbox="]), /precisa de um valor/);
	assert.throws(() => parseArgs(["run", "--thinking="]), /precisa de um valor/);
	assert.throws(() => parseArgs(["run", "-m", "--no-input"]), /precisa de um valor/);
	assert.throws(() => parseArgs(["run", "--sandbox", "codigo"]), /sandbox desconhecido/);
	assert.throws(() => parseArgs(["run", "--thinking", "ultra"]), /nível de raciocínio desconhecido/);
	assert.throws(() => parseArgs(["run", "--perfil", "docs"]), /opção desconhecida: --perfil/);
});

const tmp = mkdtempSync(path.join(os.tmpdir(), "wb-args-test-"));
after(() => rmSync(tmp, { recursive: true, force: true }));

test("estado antigo (.workbench/estado.json) é migrado uma vez para state.json", () => {
	const root = path.join(tmp, "migra");
	mkdirSync(path.join(root, ".workbench"), { recursive: true });
	mkdirSync(path.join(root, "docs/workbench"), { recursive: true });
	writeFileSync(path.join(root, "docs/workbench/design.md"), "# D\n");
	const sha256 = "ab31b4ef10b5b2a1bb2e2d2d9ec8e1ab2a7ff9e3f0d77ad43d33ab4b9c58cd47";
	writeFileSync(
		path.join(root, ".workbench/estado.json"),
		JSON.stringify({ aprovacoes: { design: { file: "docs/workbench/design.md", sha256, at: "x" } }, execucoes: { design: { runId: "r1", at: "t", saida: true } } }),
	);
	const s = loadState(root);
	assert.equal(s.approvals.design?.sha256, sha256);
	assert.deepEqual(s.runs.design, { runId: "r1", at: "t", outputWritten: true });
	assert.ok(!existsSync(path.join(root, ".workbench/estado.json")));
	assert.ok(JSON.parse(readFileSync(path.join(root, ".workbench/state.json"), "utf8")).approvals.design);
	assert.equal(approvalStatus(root, "design"), "changed-after", "o hash migrado é comparado normalmente");
});
