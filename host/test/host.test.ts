// Ponta a ponta com o provider faux do Pi: sem chave, sem rede, sem custo.
// O "modelo" tenta ações fora do perfil; o host precisa barrar e o sandbox conter.
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, realpathSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { after, test } from "node:test";
import { fauxAssistantMessage, fauxProvider, fauxText, fauxToolCall } from "@earendil-works/pi-ai";
import { ModelRuntime } from "@earendil-works/pi-coding-agent";
import { runTask } from "../src/host.ts";
import { dropDeniedLines, redactDeep } from "../src/redact.ts";
import { rewritePrompt } from "../src/refusal.ts";
import { beginGuard, endGuard } from "../src/guard.ts";
import { loadProjectConfig } from "../src/project-config.ts";
import { matchesAny } from "../src/policy.ts";
import { ALWAYS_DENY, WRITE_DENY } from "../src/sandbox-profiles.ts";
import { buildSandboxArgs, sandboxAvailable, wrapCommand } from "../src/sandbox.ts";
import { saveState } from "../src/workflows.ts";

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

test("sandbox docs-only: lê o projeto, escreve só Markdown, sem bash, sem segredos", async () => {
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

	const r = await runTask({ projectDir: root, task: "documente o projeto", sandbox: "docs-only", modelRuntime, modelOverride: model, auditDir: path.join(tmp, "audit") });
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

test("sandbox workspace-write: bash roda no sandbox sem rede, sem home e sem o .env", { skip: !sandboxAvailable() && "bwrap ausente" }, async () => {
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
				fauxToolCall("bash", { command: "echo WB-ENTRADAS=$(ls -A .workbench | wc -l); touch .workbench/x 2>/dev/null || echo WB-SO-LEITURA" }),
			],
			{ stopReason: "toolUse" },
		),
		(context) => {
			sent = toolOutputs(context);
			return fauxAssistantMessage([fauxText("Pronto.")]);
		},
	]);

	const r = await runTask({ projectDir: root, task: "rode os testes", sandbox: "workspace-write", modelRuntime, modelOverride: model, auditDir: path.join(tmp, "audit") });

	assert.ok(r.decisions.every((d) => d.allowed));
	assert.equal(readFileSync(path.join(root, "saida.txt"), "utf8"), "dentro\n");
	assert.ok(sent.includes("HOME-ESCONDIDA") && !sent.includes("segredo-da-home"), "a home precisa estar escondida");
	assert.ok(sent.includes("FIM-ENV") && !sent.includes("segredo-do-env"), "o .env precisa estar mascarado");
	assert.ok(sent.includes("REDE-BLOQUEADA") && !sent.includes("REDE-ABERTA"), "sem rede");
	assert.ok(sent.includes("FS-SO-LEITURA"), "fora do projeto é só leitura");
	assert.match(sent, /v\d+\.\d+\.\d+/, "npm test (node --version) roda com o node da nvm");
	assert.ok(sent.includes("WB-ENTRADAS=0") && sent.includes("WB-SO-LEITURA"), ".workbench aparece vazia e só leitura para o bash");
	assert.ok(r.changedFiles.includes("saida.txt"));
	assert.equal(readFileSync(path.join(root, ".env"), "utf8"), "DB_PASSWORD=segredo-do-env\n");
});

test("sandbox read-only: sem ferramentas de escrita nem bash", async () => {
	const root = makeProject("leitura");
	const { modelRuntime, model } = await fauxRuntime([
		fauxAssistantMessage([fauxToolCall("write", { path: "README.md", content: "x" }), fauxToolCall("bash", { command: "ls" })], { stopReason: "toolUse" }),
		fauxAssistantMessage([fauxText("Só li.")]),
	]);
	const r = await runTask({ projectDir: root, task: "leia", sandbox: "read-only", modelRuntime, modelOverride: model, auditDir: path.join(tmp, "audit") });
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
		sandbox: "docs-only",
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
	const byUser = r.decisions.filter((d) => d.by === "user").map((d) => `${d.target}:${d.allowed}`);
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
	const r = await runTask({ projectDir: root, task: "x", sandbox: "docs-only", modelRuntime, modelOverride: model, auditDir: path.join(tmp, "audit") });
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
	const r = await runTask({ projectDir: root, task: "x", sandbox: "docs-only", modelRuntime, modelOverride: model, auditDir: path.join(tmp, "audit") });
	assert.equal(r.finalText, "Feito.");
	assert.ok(existsSync(path.join(root, "docs/feito.md")));
	assert.ok(readFileSync(r.auditPath, "utf8").includes('"event":"refusal"'));
});

test("reescrita: troca \"evidência\" no pedido antes de enviar e registra na auditoria", async () => {
	assert.deepEqual(rewritePrompt("Mostre a evidência e as Evidências. EVIDÊNCIA!"), { text: "Mostre a prova e as Provas. PROVA!", count: 3 });
	assert.deepEqual(rewritePrompt("isso evidencia o bug; evidenciar; inevidência"), { text: "isso evidencia o bug; evidenciar; inevidência", count: 0 });
	assert.deepEqual(rewritePrompt("abra docs/evidência.md e evidência_2; falta evidência."), { text: "abra docs/evidência.md e evidência_2; falta prova.", count: 1 });

	const root = makeProject("reescrita");
	let sent = "";
	const { modelRuntime, model } = await fauxRuntime([
		(context) => {
			sent = JSON.stringify(context.messages);
			return fauxAssistantMessage([fauxText("ok")]);
		},
	]);
	const r = await runTask({ projectDir: root, task: "liste a evidência do bug", sandbox: "read-only", modelRuntime, modelOverride: model, auditDir: path.join(tmp, "audit") });
	assert.match(sent, /liste a prova do bug/);
	assert.doesNotMatch(sent, /evidência/);
	assert.ok(readFileSync(r.auditPath, "utf8").includes('"event":"prompt_rewrite"'));
});

test("globs casam arquivos e pastas com ponto (.config/, .ssh/, .devcontainer/)", () => {
	for (const f of [".env", "app/.env", ".devcontainer/.env", ".config/credentials.json", "a/.ssh/id_rsa", ".docker/certs/x.pem"]) {
		assert.ok(matchesAny(f, ALWAYS_DENY), f);
	}
	for (const f of [".git", ".git/config", ".git/hooks/.x", "sub/.git/config", ".workbench/state.json", ".workbench.json"]) {
		assert.ok(matchesAny(f, WRITE_DENY), f);
	}
	assert.ok(matchesAny("docs/a/b.md", ["docs/**"]) && matchesAny("docs", ["docs/**"]) && !matchesAny("docsx/a", ["docs/**"]));
	assert.ok(matchesAny(".github/x.md", ["**/*.md"]) && !matchesAny("a.mdx", ["**/*.md"]));
});

test("sandbox: bash não altera .git nem a configuração/estado da Workbench, e não lê segredos em pastas ocultas", { skip: !sandboxAvailable() && "bwrap ausente" }, async () => {
	const root = makeProject("fuga");
	mkdirSync(path.join(root, ".config"), { recursive: true });
	writeFileSync(path.join(root, ".config/credentials.json"), '{"token":"segredo-oculto"}\n');
	const gitConfig = readFileSync(path.join(root, ".git/config"), "utf8");
	let sent = "";
	const { modelRuntime, model } = await fauxRuntime([
		fauxAssistantMessage(
			[
				fauxToolCall("bash", { command: "git config core.fsmonitor 'touch /tmp/pwned' 2>&1; echo '[core]' >> .git/config 2>&1; echo FIM-GIT" }),
				fauxToolCall("bash", { command: "echo '#!/bin/sh' > .git/hooks/pre-commit 2>&1; echo FIM-HOOK" }),
				fauxToolCall("bash", { command: `echo '{"approvals":{}}' > .workbench/state.json 2>&1; echo '{"sandbox":"workspace-write"}' > .workbench.json; echo FIM-WB` }),
				fauxToolCall("bash", { command: "cat .config/credentials.json; echo FIM-CRED" }),
			],
			{ stopReason: "toolUse" },
		),
		(context) => {
			sent = toolOutputs(context);
			return fauxAssistantMessage([fauxText("ok")]);
		},
	]);
	const r = await runTask({ projectDir: root, task: "x", sandbox: "workspace-write", modelRuntime, modelOverride: model, auditDir: path.join(tmp, "audit") });
	assert.ok(sent.includes("FIM-GIT") && sent.includes("FIM-HOOK") && sent.includes("FIM-WB") && sent.includes("FIM-CRED"));
	assert.equal(readFileSync(path.join(root, ".git/config"), "utf8"), gitConfig, ".git/config intacto");
	assert.ok(!existsSync(path.join(root, ".git/hooks/pre-commit")), "sem hook plantado");
	assert.ok(!existsSync(path.join(root, ".workbench/state.json")), "estado da Workbench intacto");
	assert.ok(!existsSync(path.join(root, ".workbench.json")), ".workbench.json criado no run é posto de lado");
	assert.ok(readFileSync(r.auditPath, "utf8").includes('"event":"violation"'));
	assert.ok(!sent.includes("segredo-oculto"), "segredo em pasta oculta mascarado");

	const { modelRuntime: m2, model: model2 } = await fauxRuntime([
		fauxAssistantMessage([fauxToolCall("read", { path: ".config/credentials.json" })], { stopReason: "toolUse" }),
		fauxAssistantMessage([fauxText("ok")]),
	]);
	const r2 = await runTask({ projectDir: root, task: "x", sandbox: "read-only", modelRuntime: m2, modelOverride: model2, auditDir: path.join(tmp, "audit") });
	assert.ok(r2.decisions.some((d) => d.target.includes(".config/credentials.json") && !d.allowed), "read direto negado");
});

test("sandbox: num projeto sem .git, o .git criado pelo bash (raiz ou subpasta) é posto de lado", { skip: !sandboxAvailable() && "bwrap ausente" }, async () => {
	const root = path.join(tmp, "semgit");
	mkdirSync(path.join(root, "sub"), { recursive: true });
	writeFileSync(path.join(root, "a.txt"), "a\n");
	const { modelRuntime, model } = await fauxRuntime([
		fauxAssistantMessage(
			[fauxToolCall("bash", { command: "git init -q . && git config core.fsmonitor 'touch /tmp/wb-pwned' && git init -q sub && echo FEITO" })],
			{ stopReason: "toolUse" },
		),
		fauxAssistantMessage([fauxText("ok")]),
	]);
	const r = await runTask({ projectDir: root, task: "x", sandbox: "workspace-write", modelRuntime, modelOverride: model, auditDir: path.join(tmp, "audit") });
	assert.ok(!existsSync(path.join(root, ".git")), ".git da raiz posto de lado");
	assert.ok(!existsSync(path.join(root, "sub/.git")), ".git da subpasta posto de lado");
	assert.ok(existsSync(path.join(root, `.git.rejected-${r.runId}`)));
	assert.equal(readFileSync(r.auditPath, "utf8").split('"event":"violation"').length - 1, 2);
});

test("guarda: .workbench.json plantado é posto de lado mesmo se o run terminar com erro", { skip: !sandboxAvailable() && "bwrap ausente" }, async () => {
	const root = makeProject("erro");
	const { modelRuntime, model } = await fauxRuntime([
		fauxAssistantMessage([fauxToolCall("bash", { command: `echo '{"sandbox":"workspace-write"}' > .workbench.json` })], { stopReason: "toolUse" }),
		fauxAssistantMessage([], { stopReason: "error", errorMessage: "falha simulada" }),
	]);
	await assert.rejects(runTask({ projectDir: root, task: "x", sandbox: "workspace-write", modelRuntime, modelOverride: model, auditDir: path.join(tmp, "audit") }), /falha simulada/);
	assert.ok(!existsSync(path.join(root, ".workbench.json")));
	assert.equal(loadProjectConfig(root).sandbox, undefined);
});

test("guarda: run interrompido (sem finally) é limpo no próximo uso da configuração", () => {
	const root = makeProject("interrompido");
	beginGuard(root, "run-morto");
	writeFileSync(path.join(root, ".workbench.json"), '{"sandbox":"workspace-write"}\n');
	assert.equal(loadProjectConfig(root).sandbox, undefined, "a configuração plantada não vale");
	assert.ok(existsSync(path.join(root, ".workbench.json.rejected-run-morto")));
	assert.ok(!existsSync(path.join(root, ".workbench/run-run-morto.json")));
});

test("redação percorre cada string: segredo depois de quebra de linha e chave partida entre mensagens", () => {
	const key = "sk-ant-api03-" + "Z".repeat(40);
	const r = redactDeep([{ content: `linha\n${key}` }, { content: "-----BEGIN RSA PRIVATE KEY-----\nabc" }, { content: "xyz\n-----END RSA PRIVATE KEY-----" }]);
	assert.ok(!JSON.stringify(r.value).includes(key));
	assert.equal(r.value.length, 3, "a estrutura continua íntegra");
	assert.equal(r.value[1].content, "-----BEGIN RSA PRIVATE KEY-----\nabc");
});

test("filtro de grep tira também linhas de contexto de arquivos negados", () => {
	const root = makeProject("grepctx");
	writeFileSync(path.join(root, "server.key"), "segredo\n");
	const out = ["src/app.ts:1:export const x = 1;", "server.key-1-segredo", "server.key:2:segredo", "./server.key", "src/app.ts-2-fim"].join("\n");
	assert.equal(dropDeniedLines(out, root, ["**/*.key"]), ["src/app.ts:1:export const x = 1;", "src/app.ts-2-fim"].join("\n"));
});

test("sandbox: .git aninhado que é arquivo (submódulo) não pode ser redirecionado pelo bash", { skip: !sandboxAvailable() && "bwrap ausente" }, async () => {
	const root = makeProject("submodulo");
	mkdirSync(path.join(root, "sub"), { recursive: true });
	writeFileSync(path.join(root, "sub/.git"), "gitdir: ../.git/modules/sub\n");
	let sent = "";
	const { modelRuntime, model } = await fauxRuntime([
		fauxAssistantMessage(
			[fauxToolCall("bash", { command: "mkdir -p evil && echo 'gitdir: ../evil' > sub/.git 2>&1; echo FIM" })],
			{ stopReason: "toolUse" },
		),
		(context) => {
			sent = toolOutputs(context);
			return fauxAssistantMessage([fauxText("ok")]);
		},
	]);
	await runTask({ projectDir: root, task: "x", sandbox: "workspace-write", modelRuntime, modelOverride: model, auditDir: path.join(tmp, "audit") });
	assert.ok(sent.includes("FIM"));
	assert.equal(readFileSync(path.join(root, "sub/.git"), "utf8"), "gitdir: ../.git/modules/sub\n");
});

test("sandbox: credenciais dentro das toolchains da home ficam mascaradas", { skip: !sandboxAvailable() && "bwrap ausente" }, () => {
	const fakeHome = path.join(tmp, "home-falsa");
	mkdirSync(path.join(fakeHome, ".cargo/bin"), { recursive: true });
	writeFileSync(path.join(fakeHome, ".cargo/credentials.toml"), "[registry]\ntoken = \"cio-segredo\"\n");
	writeFileSync(path.join(fakeHome, ".cargo/bin/ferramenta"), "binario\n");
	const root = makeProject("toolchain");
	const oldHome = process.env.HOME;
	process.env.HOME = fakeHome;
	try {
		const cmd = wrapCommand("cat ~/.cargo/credentials.toml; cat ~/.cargo/bin/ferramenta", root, root, []);
		const out = execFileSync("/bin/bash", ["-c", cmd], { encoding: "utf8" });
		assert.ok(out.includes("binario"), "a toolchain continua visível");
		assert.ok(!out.includes("cio-segredo"), "o token do cargo fica mascarado");
	} finally {
		process.env.HOME = oldHome;
	}
});

test("symlink quebrado não leva escrita para fora da raiz nem para .workbench", async () => {
	const root = makeProject("symlink-quebrado");
	const outside = path.join(tmp, "fora-ainda-nao-existe.txt");
	symlinkSync(outside, path.join(root, "notas.md"));
	symlinkSync(".workbench/state.json", path.join(root, "aprova.md"));
	const { modelRuntime, model } = await fauxRuntime([
		fauxAssistantMessage(
			[
				fauxToolCall("write", { path: "notas.md", content: "fuga\n" }),
				fauxToolCall("write", { path: "aprova.md", content: "{}\n" }),
			],
			{ stopReason: "toolUse" },
		),
		fauxAssistantMessage([fauxText("ok")]),
	]);
	const r = await runTask({ projectDir: root, task: "x", sandbox: "docs-only", modelRuntime, modelOverride: model, auditDir: path.join(tmp, "audit") });
	assert.ok(!existsSync(outside), "nada escrito fora da raiz");
	assert.ok(!existsSync(path.join(root, ".workbench/state.json")), "aprovação não forjada");
	assert.ok(r.decisions.filter((d) => !d.allowed).length >= 2);
});

test("guarda: não limpa a foto de um run que ainda está vivo", () => {
	const root = makeProject("vivo");
	mkdirSync(path.join(root, ".workbench"), { recursive: true });
	const alivePid = process.ppid;
	writeFileSync(path.join(root, ".workbench/run-vivo.json"), JSON.stringify({ runId: "vivo", pid: alivePid, configExisted: false, gits: [".git"] }));
	writeFileSync(path.join(root, ".workbench.json"), '{"sandbox":"docs-only"}\n');
	assert.equal(loadProjectConfig(root).sandbox, "docs-only");
	assert.ok(existsSync(path.join(root, ".workbench/run-vivo.json")), "a foto do run vivo continua lá");
});

test("auditoria: pedido com segredo é gravado redigido e cada envio registra a redação", async () => {
	const root = makeProject("audit-segredo");
	let sentText = "";
	const { modelRuntime, model } = await fauxRuntime([
		(context) => {
			sentText = JSON.stringify(context.messages);
			return fauxAssistantMessage([fauxText("ok")]);
		},
	]);
	const r = await runTask({ projectDir: root, task: `use a chave ${FAKE_KEY}`, sandbox: "read-only", modelRuntime, modelOverride: model, auditDir: path.join(tmp, "audit") });
	const log = readFileSync(r.auditPath, "utf8");
	assert.ok(!log.includes(FAKE_KEY), "a chave não fica na auditoria");
	assert.ok(!sentText.includes(FAKE_KEY), "a chave não vai ao modelo");
	const sends = log.split("\n").filter(Boolean).map((l) => JSON.parse(l)).filter((e) => e.event === "send" && e.source === "context");
	assert.ok(sends.length > 0 && sends.every((e) => e.secretsRedacted.length > 0));
});

test("guarda: dois runs no mesmo projeto têm fotos separadas", () => {
	const root = makeProject("dois-runs");
	const a = beginGuard(root, "run-a");
	beginGuard(root, "run-b");
	endGuard(root, a);
	assert.ok(existsSync(path.join(root, ".workbench/run-run-b.json")), "o fim do run A não apaga a foto do B");
	writeFileSync(path.join(root, ".workbench.json"), '{"sandbox":"workspace-write"}\n');
	assert.equal(loadProjectConfig(root).sandbox, undefined, "o B morto ainda é limpo");
});

test("guarda: foto da versão anterior (execucao-ativa.json) é limpa uma vez só", () => {
	const root = makeProject("legado");
	mkdirSync(path.join(root, ".workbench"), { recursive: true });
	writeFileSync(path.join(root, ".workbench/execucao-ativa.json"), JSON.stringify({ runId: "20261120T000000-abc123", configExisted: false, gits: [".git"] }));
	writeFileSync(path.join(root, ".workbench.json"), '{"sandbox":"workspace-write"}\n');
	assert.equal(loadProjectConfig(root).sandbox, undefined);
	assert.ok(!existsSync(path.join(root, ".workbench/execucao-ativa.json")), "a foto antiga foi removida");
	writeFileSync(path.join(root, ".workbench.json"), '{"sandbox":"docs-only"}\n');
	assert.equal(loadProjectConfig(root).sandbox, "docs-only", "uma configuração criada depois continua valendo");
	assert.equal(loadProjectConfig(root).sandbox, "docs-only");
});

test("guarda: runId malicioso na foto não sai de .workbench/", () => {
	const root = makeProject("runid");
	mkdirSync(path.join(root, ".workbench"), { recursive: true });
	writeFileSync(path.join(root, "alvo.json"), "{}\n");
	writeFileSync(path.join(root, ".workbench/run-x.json"), JSON.stringify({ runId: "/../../alvo", configExisted: true, gits: [".git"] }));
	loadProjectConfig(root);
	assert.ok(existsSync(path.join(root, "alvo.json")));
	assert.ok(!existsSync(path.join(root, ".workbench/run-x.json")));
});

test("aprovação e ask_user nunca ficam abertas ao mesmo tempo", async () => {
	const root = makeProject("serial");
	const { modelRuntime, model } = await fauxRuntime([
		fauxAssistantMessage(
			[
				fauxToolCall("ask_user", { question: "qual nome?" }),
				fauxToolCall("write", { path: "src/a.ts", content: "a\n" }),
				fauxToolCall("ask_user", { question: "e a cor?" }),
				fauxToolCall("write", { path: "src/b.ts", content: "b\n" }),
			],
			{ stopReason: "toolUse" },
		),
		fauxAssistantMessage([fauxText("ok")]),
	]);
	let open = 0;
	let maxOpen = 0;
	const calls: string[] = [];
	// Um terminal real (readline) só atende uma pergunta por vez: a segunda nunca resolveria.
	const prompt = async <T>(label: string, answer: T): Promise<T> => {
		open++;
		maxOpen = Math.max(maxOpen, open);
		calls.push(label);
		await new Promise((r) => setTimeout(r, 10));
		open--;
		return answer;
	};
	await runTask({
		projectDir: root,
		task: "x",
		sandbox: "docs-only",
		modelRuntime,
		modelOverride: model,
		auditDir: path.join(tmp, "audit"),
		ask: (req) => prompt(`ask ${req.path}`, "once" as const),
		askUser: (q) => prompt(`user ${q}`, "azul"),
	});
	assert.equal(calls.length, 4);
	assert.equal(maxOpen, 1);
});

test("auditoria em .workbench/runs do projeto, fora do git, com tokens; o agente não lê .workbench", async () => {
	const root = makeProject("audit");
	const { modelRuntime, model } = await fauxRuntime([
		fauxAssistantMessage([fauxToolCall("read", { path: "README.md" }), fauxToolCall("read", { path: ".workbench/.gitignore" })], { stopReason: "toolUse" }),
		fauxAssistantMessage([fauxText("Li.")]),
	]);
	const r = await runTask({ projectDir: root, task: "leia", sandbox: "read-only", modelRuntime, modelOverride: model });

	assert.equal(path.dirname(r.auditPath), path.join(root, ".workbench/runs"));
	assert.equal(readFileSync(path.join(root, ".workbench/.gitignore"), "utf8").trim().split("\n").at(-1), "*");
	const gitStatus = execFileSync("git", ["status", "--porcelain", "--untracked-files=all"], { cwd: root, encoding: "utf8" });
	assert.doesNotMatch(gitStatus, /\.workbench/, "a auditoria não aparece no git");
	assert.deepEqual(r.changedFiles, []);

	assert.ok(r.decisions.some((d) => d.target === "README.md" && d.allowed));
	assert.ok(r.decisions.some((d) => d.target === ".workbench/.gitignore" && !d.allowed), "o agente não lê .workbench");

	const audit = readFileSync(r.auditPath, "utf8").trim().split("\n").map((l) => JSON.parse(l));
	const usage = audit.filter((e) => e.event === "usage");
	assert.equal(usage.length, 2, "um evento por resposta do modelo");
	assert.ok(usage.every((e) => e.total > 0 && e.model));
	const end = audit.find((e) => e.event === "end");
	assert.equal(end.usage.responses, 2);
	assert.equal(end.usage.total, usage.reduce((s, e) => s + e.total, 0));
	assert.deepEqual(r.usage, end.usage);
});

test(".workbench que é symlink é recusado", async () => {
	const root = makeProject("wb-link");
	const outside = path.join(tmp, "fora-do-projeto");
	mkdirSync(outside);
	symlinkSync(outside, path.join(root, ".workbench"));
	await assert.rejects(runTask({ projectDir: root, task: "x", sandbox: "read-only" }), /\.workbench é um symlink/);
	assert.ok(!existsSync(path.join(outside, ".gitignore")));
});

test("sandbox mascara .workbench com um mount só, qualquer que seja o número de runs", () => {
	const root = makeProject("muitos-runs");
	mkdirSync(path.join(root, ".workbench/runs"), { recursive: true });
	for (let i = 0; i < 200; i++) writeFileSync(path.join(root, `.workbench/runs/run-${i}.jsonl`), "{}\n");
	const args = buildSandboxArgs(root, root, ALWAYS_DENY);
	assert.ok(!args.some((a) => a.includes(".workbench/runs")), "nenhum arquivo da auditoria vira argumento");
	const real = realpathSync(path.join(root, ".workbench"));
	const i = args.indexOf("--tmpfs", args.indexOf(real));
	assert.deepEqual(args.slice(i, i + 4), ["--tmpfs", real, "--remount-ro", real], "uma pasta vazia e só leitura no lugar de .workbench");
});

test("symlinks dentro de .workbench (vindos do repositório) não são seguidos", async () => {
	const outside = path.join(tmp, "alvo-externo");
	mkdirSync(outside);

	const runsLink = makeProject("wb-runs-link");
	mkdirSync(path.join(runsLink, ".workbench"));
	symlinkSync(outside, path.join(runsLink, ".workbench/runs"));
	await assert.rejects(runTask({ projectDir: runsLink, task: "x", sandbox: "read-only" }), /runs é um symlink/);

	const ignoreLink = makeProject("wb-ignore-link");
	mkdirSync(path.join(ignoreLink, ".workbench"));
	symlinkSync(path.join(outside, "plantado"), path.join(ignoreLink, ".workbench/.gitignore"));
	await assert.rejects(runTask({ projectDir: ignoreLink, task: "x", sandbox: "read-only" }), /\.gitignore é um symlink/);

	const stateLink = makeProject("wb-state-link");
	mkdirSync(path.join(stateLink, ".workbench"));
	writeFileSync(path.join(outside, "state-alvo.json"), "original\n");
	symlinkSync(path.join(outside, "state-alvo.json"), path.join(stateLink, ".workbench/state.json"));
	assert.throws(() => saveState(stateLink, { approvals: {}, runs: {} } as never), /state\.json é um symlink/);

	assert.deepEqual(readdirSync(outside).sort(), ["state-alvo.json"], "nada foi criado fora do projeto");
	assert.equal(readFileSync(path.join(outside, "state-alvo.json"), "utf8"), "original\n");
});

test("criação de .workbench tolera outro wb criando ao mesmo tempo; marcador e auditoria não seguem symlink", async () => {
	const { workbenchDir, auditDirFor, appendInside, writeInside } = await import("../src/workdir.ts");
	const root = makeProject("wb-corrida");
	// Simula a corrida: a pasta e o .gitignore já existem quando o segundo wb chega.
	workbenchDir(root);
	assert.doesNotThrow(() => workbenchDir(root));
	assert.doesNotThrow(() => auditDirFor(root));

	const outside = path.join(tmp, "alvo-append");
	writeFileSync(outside, "original\n");
	symlinkSync(outside, path.join(root, ".workbench/runs/x.jsonl"));
	symlinkSync(outside, path.join(root, ".workbench/run-x.json"));
	assert.throws(() => appendInside(path.join(root, ".workbench/runs/x.jsonl"), "{}\n"), /é um symlink/);
	assert.throws(() => writeInside(path.join(root, ".workbench/run-x.json"), "{}\n"), /é um symlink/);
	assert.equal(readFileSync(outside, "utf8"), "original\n");
});
