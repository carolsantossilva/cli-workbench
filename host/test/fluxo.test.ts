// Workflows com o provider faux: encadeamento por arquivos, escopo por etapa,
// pergunta ao usuário e aprovação do design por hash antes do build.
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { after, test } from "node:test";
import { fauxAssistantMessage, fauxProvider, fauxText, fauxToolCall } from "@earendil-works/pi-ai";
import { ModelRuntime } from "@earendil-works/pi-coding-agent";
import { runWorkflow } from "../src/fluxo.ts";
import { sandboxAvailable } from "../src/sandbox.ts";
import { approvalStatus, approve, loadState } from "../src/workflows.ts";

const tmp = mkdtempSync(path.join(os.tmpdir(), "wb-fluxo-test-"));
after(() => rmSync(tmp, { recursive: true, force: true }));

function makeProject(name: string): string {
	const root = path.join(tmp, name);
	mkdirSync(path.join(root, "src"), { recursive: true });
	writeFileSync(path.join(root, "src/app.js"), "export const x = 1;\n");
	execFileSync("git", ["init", "-q"], { cwd: root });
	execFileSync("git", ["add", "-A"], { cwd: root });
	execFileSync("git", ["-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "init"], { cwd: root });
	return root;
}

async function faux(responses: Parameters<ReturnType<typeof fauxProvider>["setResponses"]>[0]) {
	const f = fauxProvider();
	f.setResponses(responses);
	const modelRuntime = await ModelRuntime.create({ authPath: path.join(tmp, "auth.json"), modelsPath: null, allowModelNetwork: false, refreshOnCreate: false });
	modelRuntime.registerNativeProvider(f.provider);
	return { modelRuntime, modelOverride: f.getModel(), auditDir: path.join(tmp, "audit") };
}

const toolResults = (context: { messages: { role: string; content: unknown }[] }) =>
	context.messages.filter((m) => m.role === "toolResult").map((m) => JSON.stringify(m.content)).join("\n");

test("brainstorm: grava a ideia, pergunta ao usuário e só escreve o brief", async () => {
	const root = makeProject("brainstorm");
	let prompt = "";
	let results = "";
	const questions: string[] = [];
	const m = await faux([
		(context) => {
			prompt = JSON.stringify(context);
			return fauxAssistantMessage(
				[fauxToolCall("read", { path: "docs/workbench/idea.md" }), fauxToolCall("perguntar", { pergunta: "Quem é o público?" })],
				{ stopReason: "toolUse" },
			);
		},
		(context) => {
			results = toolResults(context);
			return fauxAssistantMessage(
				[
					fauxToolCall("write", { path: "src/app.js", content: "hack" }),
					fauxToolCall("write", { path: "docs/workbench/brief.md", content: "# Brief\n\nPúblico: pessoas com TDAH.\n" }),
				],
				{ stopReason: "toolUse" },
			);
		},
		fauxAssistantMessage([fauxText("Brief pronto.")]),
	]);

	const r = await runWorkflow({ ...m, projectDir: root, id: "brainstorm", note: "app de rotina visual", askUser: async (q) => (questions.push(q), "pessoas com TDAH") });

	assert.equal(readFileSync(path.join(root, "docs/workbench/idea.md"), "utf8"), "# Ideia\n\napp de rotina visual\n");
	assert.match(prompt, /Etapa brainstorm/);
	assert.deepEqual(questions, ["Quem é o público?"]);
	assert.match(results, /app de rotina visual/);
	assert.match(results, /pessoas com TDAH/);
	assert.equal(readFileSync(path.join(root, "src/app.js"), "utf8"), "export const x = 1;\n");
	assert.ok(r.outputWritten);
	assert.equal(loadState(root).execucoes.brainstorm?.saida, true);
});

test("grill-me sem brief não roda e aponta a etapa anterior", async () => {
	const root = makeProject("semBrief");
	const m = await faux([fauxAssistantMessage([fauxText("nada")])]);
	await assert.rejects(runWorkflow({ ...m, projectDir: root, id: "grill-me" }), /falta docs\/workbench\/brief\.md; rode "wb fluxo brainstorm" antes/);
});

test("design: sem a ferramenta perguntar e escrevendo só design.md", async () => {
	const root = makeProject("design");
	mkdirSync(path.join(root, "docs/workbench"), { recursive: true });
	writeFileSync(path.join(root, "docs/workbench/brief.md"), "# Brief\n");
	let results = "";
	const m = await faux([
		fauxAssistantMessage(
			[fauxToolCall("perguntar", { pergunta: "posso?" }), fauxToolCall("write", { path: "docs/workbench/brief.md", content: "reescrito" })],
			{ stopReason: "toolUse" },
		),
		(context) => {
			results = toolResults(context);
			return fauxAssistantMessage([fauxToolCall("write", { path: "docs/workbench/design.md", content: "# Design\n" })], { stopReason: "toolUse" });
		},
		fauxAssistantMessage([fauxText("ok")]),
	]);
	const asked: string[] = [];
	const r = await runWorkflow({ ...m, projectDir: root, id: "design", askUser: async (q) => (asked.push(q), "sim") });
	assert.deepEqual(asked, [], "design não pergunta ao usuário");
	assert.equal(readFileSync(path.join(root, "docs/workbench/brief.md"), "utf8"), "# Brief\n", "design não reescreve o brief");
	assert.ok(r.outputWritten);
	assert.equal(approvalStatus(root, "design"), "pendente");
	assert.ok(results.length > 0);
});

test("build exige o design aprovado e inalterado", { skip: !sandboxAvailable() && "bwrap ausente" }, async () => {
	const root = makeProject("build");
	mkdirSync(path.join(root, "docs/workbench"), { recursive: true });
	writeFileSync(path.join(root, "docs/workbench/design.md"), "# Design v1\n");
	const m = await faux([
		fauxAssistantMessage(
			[
				fauxToolCall("write", { path: "docs/workbench/plan.md", content: "# Plano\n- [x] lote 1\n" }),
				fauxToolCall("edit", { path: "src/app.js", edits: [{ oldText: "x = 1", newText: "x = 2" }] }),
				fauxToolCall("bash", { command: "node -e \"import('./src/app.js').then(m=>console.log('x='+m.x))\"" }),
			],
			{ stopReason: "toolUse" },
		),
		fauxAssistantMessage([fauxText("Lote 1 feito.")]),
	]);

	await assert.rejects(runWorkflow({ ...m, projectDir: root, id: "build" }), /ainda não foi aprovado/);
	approve(root, "design");
	writeFileSync(path.join(root, "docs/workbench/design.md"), "# Design v2\n");
	await assert.rejects(runWorkflow({ ...m, projectDir: root, id: "build" }), /mudou depois da aprovação/);
	approve(root, "design");

	const r = await runWorkflow({ ...m, projectDir: root, id: "build" });
	assert.ok(r.outputWritten);
	assert.equal(readFileSync(path.join(root, "src/app.js"), "utf8"), "export const x = 2;\n");
	assert.deepEqual(r.changedFiles, ["docs/workbench/plan.md", "src/app.js"]);
});

test("review: sandbox com o projeto só leitura, escreve só review.md", { skip: !sandboxAvailable() && "bwrap ausente" }, async () => {
	const root = makeProject("review");
	mkdirSync(path.join(root, "docs/workbench"), { recursive: true });
	writeFileSync(path.join(root, "docs/workbench/design.md"), "# Design\n");
	let results = "";
	const m = await faux([
		fauxAssistantMessage(
			[
				fauxToolCall("bash", { command: "git status --short; echo mudou > src/app.js 2>/dev/null || echo PROJETO-SO-LEITURA" }),
				fauxToolCall("write", { path: "src/app.js", content: "hack" }),
				fauxToolCall("write", { path: "docs/workbench/review.md", content: "# Review\n\nSem achados.\n" }),
			],
			{ stopReason: "toolUse" },
		),
		(context) => {
			results = toolResults(context);
			return fauxAssistantMessage([fauxText("ok")]);
		},
	]);
	const r = await runWorkflow({ ...m, projectDir: root, id: "review" });
	assert.match(results, /PROJETO-SO-LEITURA/);
	assert.equal(readFileSync(path.join(root, "src/app.js"), "utf8"), "export const x = 1;\n");
	assert.ok(r.outputWritten);
	assert.ok(existsSync(path.join(root, "docs/workbench/review.md")));
});
