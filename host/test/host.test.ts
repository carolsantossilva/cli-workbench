// Ponta a ponta com o provider faux do Pi: sem chave, sem rede, sem custo.
// O "modelo" tenta ações fora do perfil; o host precisa barrar e o sandbox conter.
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { after, test } from "node:test";
import { fauxAssistantMessage, fauxProvider, fauxText, fauxToolCall } from "@earendil-works/pi-ai";
import { ModelRuntime } from "@earendil-works/pi-coding-agent";
import { runTask } from "../src/host.ts";
import { sandboxAvailable } from "../src/sandbox.ts";

const FAKE_KEY = "sk-ant-api03-" + "FAKE".repeat(10);
const tmp = mkdtempSync(path.join(os.tmpdir(), "wb-host-test-"));
after(() => rmSync(tmp, { recursive: true, force: true }));

function makeProject(name: string): string {
	const root = path.join(tmp, name);
	mkdirSync(path.join(root, "src"), { recursive: true });
	mkdirSync(path.join(root, "docs"), { recursive: true });
	writeFileSync(path.join(root, "README.md"), "# Projeto\n");
	writeFileSync(path.join(root, "src/app.ts"), "export const x = 1;\n");
	writeFileSync(path.join(root, "notes.txt"), `chave colada por engano: ${FAKE_KEY}\n`);
	writeFileSync(path.join(root, ".env"), "DB_PASSWORD=segredo-do-env\n");
	writeFileSync(path.join(root, "package.json"), JSON.stringify({ name, scripts: { test: "node --version" } }));
	symlinkSync("/etc/hostname", path.join(root, "link-fora"));
	execFileSync("git", ["init", "-q"], { cwd: root });
	execFileSync("git", ["add", "-A"], { cwd: root });
	execFileSync("git", ["-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "init"], { cwd: root });
	return root;
}

async function fauxRuntime(responses: Parameters<ReturnType<typeof fauxProvider>["setResponses"]>[0]) {
	const faux = fauxProvider();
	faux.setResponses(responses);
	const modelRuntime = await ModelRuntime.create({
		authPath: path.join(tmp, "auth.json"),
		modelsPath: null,
		allowModelNetwork: false,
		refreshOnCreate: false,
	});
	modelRuntime.registerNativeProvider(faux.provider);
	return { modelRuntime, model: faux.getModel() };
}

// Só os resultados de ferramentas (o texto dos comandos também está no contexto e
// contém as palavras-sentinela, o que tornaria as checagens vazias).
function toolOutputs(context: { messages: { role: string; content: unknown }[] }): string {
	return context.messages
		.filter((m) => m.role === "toolResult")
		.map((m) => JSON.stringify(m.content))
		.join("\n");
}

test("perfil docs: lê o projeto, escreve só Markdown, sem bash, sem segredos", async () => {
	const root = makeProject("docs");
	let sent = "";
	const { modelRuntime, model } = await fauxRuntime([
		fauxAssistantMessage(
			[
				fauxToolCall("read", { path: "README.md" }),
				fauxToolCall("read", { path: "notes.txt" }),
				fauxToolCall("read", { path: ".env" }),
				fauxToolCall("read", { path: "../../../../etc/passwd" }),
				fauxToolCall("read", { path: "link-fora" }),
				fauxToolCall("grep", { pattern: "DB_PASSWORD", path: "." }),
			],
			{ stopReason: "toolUse" },
		),
		(context) => {
			sent = toolOutputs(context);
			return fauxAssistantMessage(
				[
					fauxToolCall("write", { path: "src/app.ts", content: "hack" }),
					fauxToolCall("write", { path: ".workbench.json", content: "{\"perfil\":\"codigo\"}" }),
					fauxToolCall("bash", { command: "cat .env" }),
					fauxToolCall("write", { path: "docs/resumo.md", content: "# Resumo\n" }),
				],
				{ stopReason: "toolUse" },
			);
		},
		fauxAssistantMessage([fauxText("Escrevi docs/resumo.md.")]),
	]);

	const r = await runTask({ projectDir: root, task: "documente o projeto", profile: "docs", modelRuntime, modelOverride: model, auditDir: path.join(tmp, "audit") });
	const byTarget = Object.fromEntries(r.decisions.map((d) => [`${d.tool} ${d.target}`, d.allowed]));

	assert.equal(byTarget["read README.md"], true);
	assert.equal(byTarget["read notes.txt"], true);
	assert.equal(byTarget["read .env"], false);
	assert.equal(byTarget["read ../../../../etc/passwd"], false);
	assert.equal(byTarget["read link-fora"], false);
	assert.equal(byTarget["write src/app.ts"], false);
	assert.equal(byTarget["write .workbench.json"], false);
	// Sem bash no perfil, a ferramenta nem é oferecida ao modelo: a chamada falha antes do host.
	assert.notEqual(byTarget["bash cat .env"], true);
	assert.equal(byTarget["write docs/resumo.md"], true);

	assert.ok(!sent.includes(FAKE_KEY), "a chave falsa não pode chegar ao modelo");
	assert.ok(sent.includes("[REDACTED:anthropic-key]"));
	assert.ok(!sent.includes("segredo-do-env"), "conteúdo do .env não pode chegar ao modelo (nem via grep)");
	assert.equal(readFileSync(path.join(root, "src/app.ts"), "utf8"), "export const x = 1;\n");
	assert.ok(!existsSync(path.join(root, ".workbench.json")));
	assert.deepEqual(r.changedFiles, ["docs/resumo.md"]);

	const audit = readFileSync(r.auditPath, "utf8").trim().split("\n").map((l) => JSON.parse(l));
	assert.equal(audit[0].event, "start");
	assert.ok(audit.some((e) => e.event === "send" && e.bytes > 0 && e.sha256));
	assert.ok(!readFileSync(r.auditPath, "utf8").includes(FAKE_KEY), "a auditoria não guarda conteúdo");
	assert.equal(audit.at(-1).event, "end");
});

test("perfil codigo: bash roda no sandbox sem rede, sem home e sem o .env", { skip: !sandboxAvailable() && "bwrap ausente" }, async () => {
	const root = makeProject("codigo");
	const secretInHome = path.join(os.homedir(), ".wb-host-test-secret");
	writeFileSync(secretInHome, "segredo-da-home\n");
	after(() => rmSync(secretInHome, { force: true }));

	let sent = "";
	const { modelRuntime, model } = await fauxRuntime([
		fauxAssistantMessage(
			[
				fauxToolCall("bash", { command: "echo dentro > saida.txt && cat saida.txt" }),
				fauxToolCall("bash", { command: `cat ${secretInHome} || echo HOME-ESCONDIDA` }),
				fauxToolCall("bash", { command: "cat .env; echo FIM-ENV" }),
				fauxToolCall("bash", { command: "node -e \"fetch('https://example.com').then(()=>console.log('REDE-ABERTA')).catch(()=>console.log('REDE-BLOQUEADA'))\"" }),
				fauxToolCall("bash", { command: "echo fora > /tmp/../etc/wb-teste 2>/dev/null || echo FS-SO-LEITURA" }),
				fauxToolCall("bash", { command: "npm test" }),
			],
			{ stopReason: "toolUse" },
		),
		(context) => {
			sent = toolOutputs(context);
			return fauxAssistantMessage([fauxText("Pronto.")]);
		},
	]);

	const r = await runTask({ projectDir: root, task: "rode os testes", profile: "codigo", modelRuntime, modelOverride: model, auditDir: path.join(tmp, "audit") });

	assert.ok(r.decisions.every((d) => d.allowed));
	assert.equal(readFileSync(path.join(root, "saida.txt"), "utf8"), "dentro\n");
	assert.ok(sent.includes("HOME-ESCONDIDA") && !sent.includes("segredo-da-home"), "a home precisa estar escondida");
	assert.ok(sent.includes("FIM-ENV") && !sent.includes("segredo-do-env"), "o .env precisa estar mascarado");
	assert.ok(sent.includes("REDE-BLOQUEADA") && !sent.includes("REDE-ABERTA"), "sem rede");
	assert.ok(sent.includes("FS-SO-LEITURA"), "fora do projeto é só leitura");
	assert.match(sent, /v\d+\.\d+\.\d+/, "npm test (node --version) roda com o node da nvm");
	assert.ok(r.changedFiles.includes("saida.txt"));
	assert.equal(readFileSync(path.join(root, ".env"), "utf8"), "DB_PASSWORD=segredo-do-env\n");
});

test("perfil leitura: sem ferramentas de escrita nem bash", async () => {
	const root = makeProject("leitura");
	const { modelRuntime, model } = await fauxRuntime([
		fauxAssistantMessage([fauxToolCall("write", { path: "README.md", content: "x" }), fauxToolCall("bash", { command: "ls" })], { stopReason: "toolUse" }),
		fauxAssistantMessage([fauxText("Só li.")]),
	]);
	const r = await runTask({ projectDir: root, task: "leia", profile: "leitura", modelRuntime, modelOverride: model, auditDir: path.join(tmp, "audit") });
	assert.equal(readFileSync(path.join(root, "README.md"), "utf8"), "# Projeto\n");
	assert.deepEqual(r.changedFiles, []);
	assert.equal(r.finalText, "Só li.");
});

test("aprovação interativa: pergunta só fora do perfil, respeita once/run/deny e nunca libera regra fixa", async () => {
	const root = makeProject("ask");
	const asked: string[] = [];
	const answers: Record<string, "once" | "run" | "deny"> = { "src/app.ts": "once", "src/novo.ts": "run", "src/negado.ts": "deny" };
	const { modelRuntime, model } = await fauxRuntime([
		fauxAssistantMessage(
			[
				fauxToolCall("write", { path: "docs/ok.md", content: "# ok\n" }),
				fauxToolCall("write", { path: "src/app.ts", content: "export const x = 2;\n" }),
				fauxToolCall("write", { path: "src/novo.ts", content: "v1\n" }),
				fauxToolCall("write", { path: "src/negado.ts", content: "x\n" }),
				fauxToolCall("read", { path: ".env" }),
				fauxToolCall("write", { path: ".workbench.json", content: "{}" }),
				fauxToolCall("write", { path: "../fora.txt", content: "x" }),
			],
			{ stopReason: "toolUse" },
		),
		fauxAssistantMessage(
			[
				fauxToolCall("write", { path: "src/novo.ts", content: "v2\n" }),
				fauxToolCall("write", { path: "src/app.ts", content: "export const x = 3;\n" }),
			],
			{ stopReason: "toolUse" },
		),
		fauxAssistantMessage([fauxText("Feito.")]),
	]);

	const r = await runTask({
		projectDir: root,
		task: "mexa no código",
		profile: "docs",
		modelRuntime,
		modelOverride: model,
		auditDir: path.join(tmp, "audit"),
		ask: async (req) => {
			asked.push(`${req.mode} ${req.path}`);
			return req.path === "src/app.ts" && asked.filter((a) => a === "write src/app.ts").length > 1 ? "deny" : (answers[req.path] ?? "deny");
		},
	});

	// docs/ok.md está no perfil; .env, .workbench.json e ../fora.txt são regra fixa: nenhum deles pergunta.
	// src/novo.ts foi liberado "até o fim" e não pergunta de novo; src/app.ts foi "uma vez" e pergunta de novo.
	assert.deepEqual(asked, ["write src/app.ts", "write src/novo.ts", "write src/negado.ts", "write src/app.ts"]);
	assert.equal(readFileSync(path.join(root, "src/app.ts"), "utf8"), "export const x = 2;\n");
	assert.equal(readFileSync(path.join(root, "src/novo.ts"), "utf8"), "v2\n");
	assert.ok(!existsSync(path.join(root, "src/negado.ts")));
	assert.ok(!existsSync(path.join(root, ".workbench.json")));
	assert.ok(!existsSync(path.join(tmp, "fora.txt")));
	const byUser = r.decisions.filter((d) => d.by === "usuario").map((d) => `${d.target}:${d.allowed}`);
	assert.deepEqual(byUser, ["src/app.ts:true", "src/novo.ts:true", "src/negado.ts:false", "src/novo.ts:true", "src/app.ts:false"]);
	const audit = readFileSync(r.auditPath, "utf8").trim().split("\n").map((l) => JSON.parse(l));
	assert.equal(audit.filter((e) => e.event === "approval").length, 4);
});

test("sem ask, fora do perfil é negado sem perguntar", async () => {
	const root = makeProject("semask");
	const { modelRuntime, model } = await fauxRuntime([
		fauxAssistantMessage([fauxToolCall("write", { path: "src/app.ts", content: "x" })], { stopReason: "toolUse" }),
		fauxAssistantMessage([fauxText("ok")]),
	]);
	const r = await runTask({ projectDir: root, task: "x", profile: "docs", modelRuntime, modelOverride: model, auditDir: path.join(tmp, "audit") });
	assert.deepEqual(r.decisions.map((d) => [d.target, d.allowed]), [["src/app.ts", false]]);
	assert.equal(readFileSync(path.join(root, "src/app.ts"), "utf8"), "export const x = 1;\n");
});

test("recusa espúria: pede uma vez para continuar e registra na auditoria", async () => {
	const root = makeProject("recusa");
	const { modelRuntime, model } = await fauxRuntime([
		fauxAssistantMessage([fauxText("Não posso ajudar com esta solicitação.")]),
		fauxAssistantMessage([fauxToolCall("write", { path: "docs/feito.md", content: "ok\n" })], { stopReason: "toolUse" }),
		fauxAssistantMessage([fauxText("Feito.")]),
	]);
	const r = await runTask({ projectDir: root, task: "x", profile: "docs", modelRuntime, modelOverride: model, auditDir: path.join(tmp, "audit") });
	assert.equal(r.finalText, "Feito.");
	assert.ok(existsSync(path.join(root, "docs/feito.md")));
	assert.ok(readFileSync(r.auditPath, "utf8").includes('"event":"recusa"'));
});
