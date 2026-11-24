// CLI de ponta a ponta, sem modelo: despacho dos comandos, flags e argumentos recusados
// por comando, e a checagem do projeto (-C).
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { after, test } from "node:test";
import { approvalStatus } from "../src/workflows.ts";

const CLI = path.join(import.meta.dirname, "../src/cli.ts");
const tmp = mkdtempSync(path.join(os.tmpdir(), "wb-cli-test-"));
after(() => rmSync(tmp, { recursive: true, force: true }));

function wb(...args: string[]) {
	const r = spawnSync(process.execPath, [CLI, ...args], { encoding: "utf8", env: { ...process.env, NO_COLOR: "1", WB_NO_INPUT: "1" } });
	return { code: r.status, out: r.stdout, err: r.stderr };
}

function project(name: string): string {
	const root = path.join(tmp, name);
	mkdirSync(root, { recursive: true });
	return root;
}

test("despacha status, scope e step approve (que recusa sem terminal)", () => {
	const root = project("ok");
	const status = wb("status", "-C", root);
	assert.equal(status.code, 0, status.err);
	assert.match(status.out, /brainstorm/);

	const scope = wb("-C", root, "scope", "-s", "docs-only");
	assert.equal(scope.code, 0, scope.err);
	assert.match(scope.out, /sandbox "docs-only"/);

	mkdirSync(path.join(root, "docs/workbench"), { recursive: true });
	writeFileSync(path.join(root, "docs/workbench/technical-design.md"), "# Design\n");
	const approve = wb("step", "approve", "design", "-C", root);
	assert.equal(approve.code, 1, "sem terminal, não aprova");
	assert.match(approve.err, /só roda num terminal interativo/);
	assert.equal(approvalStatus(root, "design"), "pending");
});

// Num terminal (simulado com o `script` do util-linux), só aprova com "aprovar" digitado.
test("step approve num terminal: aprova só com \"aprovar\"", { skip: spawnSync("script", ["--version"]).status !== 0 && "script ausente" }, () => {
	const root = project("tty");
	mkdirSync(path.join(root, "docs/workbench"), { recursive: true });
	writeFileSync(path.join(root, "docs/workbench/technical-design.md"), "# Design\n");
	const inTty = (typed: string) =>
		spawnSync("script", ["-qec", `${process.execPath} ${CLI} step approve design -C ${root}`, "/dev/null"], { input: typed, encoding: "utf8", env: { ...process.env, NO_COLOR: "1" } });

	const no = inTty("sim\n");
	assert.match(no.stdout, /não aprovado/);
	assert.equal(approvalStatus(root, "design"), "pending");

	const yes = inTty("aprovar\n");
	assert.match(yes.stdout, /aprovado \(sha256/);
	assert.equal(approvalStatus(root, "design"), "approved");
});

test("recusa flags que o comando não usa", () => {
	const root = project("flags");
	const cases: [string[], RegExp][] = [
		[["status", "-m", "x/y"], /--model não vale para "wb status"/],
		[["scope", "--no-input"], /--no-input não vale para "wb scope"/],
		[["step", "run", "design", "-s", "workspace-write"], /--sandbox não vale para "wb step run"/],
		[["step", "approve", "design", "--thinking", "high"], /--thinking não vale para "wb step approve"/],
	];
	for (const [args, msg] of cases) {
		const r = wb(...args, "-C", root);
		assert.equal(r.code, 1, args.join(" "));
		assert.match(r.err, msg);
	}
});

test("recusa argumentos a mais ou faltando", () => {
	const root = project("args");
	const cases: [string[], RegExp][] = [
		[["status", "extra"], /argumento a mais para "wb status": extra/],
		[["scope", "a", "b"], /argumento a mais para "wb scope": a b/],
		[["step", "approve"], /faltam argumentos para "wb step approve"/],
		[["step", "approve", "design", "extra"], /argumento a mais para "wb step approve": extra/],
		[["step"], /subcomando desconhecido "step "/],
		[["step", "run"], /etapa desconhecida ""/],
		[["run"], /falta a tarefa/],
	];
	for (const [args, msg] of cases) {
		const r = wb(...args, "-C", root);
		assert.equal(r.code, 1, args.join(" "));
		assert.match(r.err, msg);
	}
});

test("checa o projeto, mas só depois de saber o comando", () => {
	const missing = path.join(tmp, "nao-existe");
	const file = path.join(project("arquivo"), "x.txt");
	writeFileSync(file, "x");
	assert.match(wb("status", "-C", missing).err, /^projeto não encontrado: /);
	assert.match(wb("status", "-C", file).err, /^o projeto precisa ser um diretório: /);
	assert.match(wb("nada", "-C", missing).err, /^comando desconhecido "nada"/);
});
