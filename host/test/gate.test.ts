// Regras de permissão do gate, sem sessão do Pi: escopo, regra fixa, liberações
// "uma vez"/"até o fim"/"não", falta de quem responda e fila de perguntas.
import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { after, test } from "node:test";
import { type ApprovalAnswer, type ApprovalRequest, createGate, type Decision } from "../src/gate.ts";
import { type EffectiveScope, resolveScope } from "../src/sandbox-profiles.ts";

const tmp = mkdtempSync(path.join(os.tmpdir(), "wb-gate-test-"));
after(() => rmSync(tmp, { recursive: true, force: true }));

let n = 0;
function project(): string {
	const root = path.join(tmp, `p${n++}`);
	mkdirSync(path.join(root, "docs"), { recursive: true });
	mkdirSync(path.join(root, "src"), { recursive: true });
	writeFileSync(path.join(root, "docs/a.md"), "# a\n");
	writeFileSync(path.join(root, "src/app.ts"), "x\n");
	writeFileSync(path.join(root, ".env"), "SEGREDO=1\n");
	return root;
}

// Lê e escreve só em docs/: ler ou escrever em src/ precisa perguntar.
const scope: EffectiveScope = { ...resolveScope("docs-only", {}), read: ["docs/**"], write: ["docs/**"] };

function setup(answers: (req: ApprovalRequest) => ApprovalAnswer | Promise<ApprovalAnswer>, withAsk = true) {
	const root = project();
	const asked: string[] = [];
	const events: { event: string; data: Record<string, unknown> }[] = [];
	const seen: Decision[] = [];
	const gate = createGate({
		root,
		scope,
		audit: (event, data) => events.push({ event, data }),
		onDecision: (d) => seen.push(d),
		ask: withAsk
			? async (req) => {
					asked.push(`${req.mode} ${req.path}`);
					return answers(req);
				}
			: undefined,
	});
	return { gate, asked, events, seen };
}

test("dentro do escopo: libera sem perguntar", async () => {
	const { gate, asked, events, seen } = setup(() => "deny");
	assert.equal(await gate.checkFile("write", "docs/a.md", "write"), undefined);
	assert.deepEqual(asked, []);
	assert.deepEqual(gate.decisions, [{ tool: "write", target: "docs/a.md", allowed: true, reason: undefined, by: "sandbox" }]);
	assert.deepEqual(seen, gate.decisions, "onDecision recebe cada decisão");
	assert.deepEqual(events.map((e) => e.event), ["decision"]);
});

test("regra fixa (arquivo sensível, fora da raiz): nega sem perguntar, mesmo com quem responda", async () => {
	const { gate, asked } = setup(() => "run");
	const env = await gate.checkFile("read", ".env", "read");
	const fora = await gate.checkFile("read", "../fora.txt", "read");
	assert.equal(env?.block, true);
	assert.match(env?.reason ?? "", /arquivo sensível/);
	assert.equal(fora?.block, true);
	assert.deepEqual(asked, []);
	assert.ok(gate.decisions.every((d) => !d.allowed && d.by === "rule"));
});

test('"uma vez" libera só esta chamada', async () => {
	const { gate, asked, events } = setup(() => "once");
	assert.equal(await gate.checkFile("write", "src/app.ts", "write"), undefined);
	assert.equal(await gate.checkFile("write", "src/app.ts", "write"), undefined);
	assert.deepEqual(asked, ["write src/app.ts", "write src/app.ts"], "pergunta de novo");
	assert.equal(gate.decisions[0].reason, "liberado pelo usuário uma vez");
	assert.deepEqual(
		events.filter((e) => e.event === "approval").map((e) => e.data),
		[1, 2].map(() => ({ tool: "write", mode: "write", path: "src/app.ts", answer: "once" })),
	);
});

test('"até o fim" vale para o arquivo e o modo; escrever libera ler, ler não libera escrever', async () => {
	const { gate, asked } = setup(() => "run");
	await gate.checkFile("write", "src/app.ts", "write");
	await gate.checkFile("edit", "src/app.ts", "write");
	await gate.checkFile("read", "src/app.ts", "read");
	assert.deepEqual(asked, ["write src/app.ts"], "write liberou write e read do mesmo arquivo");

	await gate.checkFile("read", "src/outro.ts", "read");
	await gate.checkFile("write", "src/outro.ts", "write");
	await gate.checkFile("read", "src/app2.ts", "read");
	assert.deepEqual(asked, ["write src/app.ts", "read src/outro.ts", "write src/outro.ts", "read src/app2.ts"], "read não libera write, e a liberação é por arquivo");
	assert.ok(gate.decisions.every((d) => d.allowed));
});

test('"não" bloqueia com o motivo e pergunta de novo na próxima', async () => {
	const { gate, asked } = setup(() => "deny");
	const r = await gate.checkFile("write", "src/app.ts", "write");
	assert.equal(r?.block, true);
	assert.match(r?.reason ?? "", /^Bloqueado pela Workbench: .*\(negado pelo usuário\)$/);
	await gate.checkFile("write", "src/app.ts", "write");
	assert.equal(asked.length, 2);
	assert.equal(gate.decisions[0].by, "user");
});

test("sem quem responda, ou se a pergunta falhar: nega", async () => {
	const semAsk = setup(() => "run", false);
	assert.equal((await semAsk.gate.checkFile("write", "src/app.ts", "write"))?.block, true);

	const falha = setup(() => {
		throw new Error("terminal fechado");
	});
	assert.equal((await falha.gate.checkFile("write", "src/app.ts", "write"))?.block, true);
	assert.equal(falha.asked.length, 1);
});

test("perguntas em fila: chamadas paralelas e ask_user nunca ficam abertas juntas", async () => {
	let open = 0;
	let maxOpen = 0;
	const slow = async <T>(v: T): Promise<T> => {
		open++;
		maxOpen = Math.max(maxOpen, open);
		await new Promise((r) => setTimeout(r, 5));
		open--;
		return v;
	};
	const { gate } = setup(() => slow("once" as const));
	await Promise.all([
		gate.checkFile("write", "src/a.ts", "write"),
		gate.serial(() => slow("resposta do ask_user")),
		gate.checkFile("read", "src/b.ts", "read"),
		gate.checkFile("write", "src/c.ts", "write"),
	]);
	assert.equal(maxOpen, 1);
	assert.equal(gate.decisions.length, 3);
});

test("uma pergunta que falha não trava a fila", async () => {
	const { gate } = setup(() => "once");
	await assert.rejects(gate.serial(() => Promise.reject(new Error("falhou"))));
	assert.equal(await gate.serial(async () => "seguiu"), "seguiu");
});

test("decide: nega com o prefixo da Workbench e registra quem decidiu", () => {
	const { gate } = setup(() => "deny");
	assert.deepEqual(gate.decide("rm", "x", false, "ferramenta fora do contrato", "rule"), { block: true, reason: "Bloqueado pela Workbench: ferramenta fora do contrato" });
	assert.equal(gate.decide("bash", "ls", true), undefined);
	assert.deepEqual(gate.decisions.map((d) => d.by), ["rule", "sandbox"]);
});
