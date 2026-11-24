#!/usr/bin/env node
// wb: CLI do micro MVP do host da Workbench.
// Nomes de comandos, flags e chaves em inglês, seguindo as convenções de CLI (clig.dev,
// GNU, POSIX); mensagens e ajuda em português.
import { existsSync, readFileSync, realpathSync, statSync } from "node:fs";
import path from "node:path";
import { createInterface } from "node:readline/promises";
import { detectCommands } from "./detect.ts";
import { type ApprovalAnswer, type ApprovalRequest, DEFAULT_MODEL, DEFAULT_THINKING, type Decision, describeScope, type RunResult, runTask, THINKING_LEVELS, type TokenUsage } from "./host.ts";
import { loadProjectConfig, resolveScope, SANDBOX_NAMES } from "./profiles.ts";
import { findDeniedFiles, sandboxAvailable } from "./sandbox.ts";
import { type Args, parseArgs } from "./args.ts";
import { runStep } from "./step.ts";
import { oneLine, safe } from "./terminal.ts";
import { approvalStatus, approve, loadState, preflight, STEP_IDS, type StepId, STEPS } from "./workflows.ts";

const COMMON_OPTIONS = `Opções comuns:
  -C, --project <dir>        projeto (padrão: diretório atual)
  -h, --help                 mostra esta ajuda`;

const RUN_OPTIONS = `  -s, --sandbox <nome>       ${SANDBOX_NAMES.join("|")} (padrão: o de .workbench.json, senão read-only)
  -m, --model <provider/id>  padrão ${DEFAULT_MODEL} (login: npx pi auth login openai-codex)
      --thinking <nível>     ${THINKING_LEVELS.join("|")} (padrão ${DEFAULT_THINKING})
      --no-input             nunca pergunta: fora do escopo é negado e o agente decide com premissas
                             (também com WB_NO_INPUT=1 ou sem terminal interativo)`;

const HELP: Record<string, string> = {
	main: `uso: wb <comando> [opções]

Comandos:
  run <tarefa...>              roda uma tarefa no projeto, dentro de um sandbox
  step run <etapa> [texto...]  roda uma etapa do fluxo: ${STEP_IDS.join(" → ")}
  step approve <etapa>         aprova a saída de uma etapa (hoje: design) pelo sha256; só no terminal, digitando "aprovar"
  status                       mostra em que etapa o projeto está e o próximo passo
  scope                        mostra o que o agente pode ler, escrever e rodar
  help [comando]               ajuda de um comando

${COMMON_OPTIONS}
  -V, --version              mostra a versão

Auditoria: <projeto>/.workbench/runs/<run>.jsonl (com os tokens; .workbench/ fica fora do git)
Sem cor: NO_COLOR=1.`,
	run: `uso: wb run [opções] <tarefa...>

${COMMON_OPTIONS}
${RUN_OPTIONS}`,
	step: `uso: wb step run <etapa> [opções] [texto...]
     wb step approve <etapa> [-C <dir>]

Etapas: ${STEP_IDS.join(" → ")}
  brainstorm recebe a ideia como texto; nas outras, o texto é um pedido extra.
  build só roda com o design aprovado (wb step approve design) e inalterado.
  Os arquivos de cada etapa ficam em docs/workbench/.

${COMMON_OPTIONS}
  -m, --model, --thinking, --no-input: como em "wb run"`,
	status: `uso: wb status [-C <dir>]\n\n${COMMON_OPTIONS}`,
	scope: `uso: wb scope [-C <dir>] [-s <sandbox>]\n\n${COMMON_OPTIONS}\n  -s, --sandbox <nome>       ${SANDBOX_NAMES.join("|")}`,
};

const color = process.stderr.isTTY && !process.env.NO_COLOR;
const dim = (s: string) => (color ? `\x1b[2m${s}\x1b[0m` : s);
const bold = (s: string) => (color ? `\x1b[1m${s}\x1b[0m` : s);

/**
 * Perguntas no terminal. Uma interface readline por pergunta: entre as perguntas o
 * stdin volta ao modo normal e Ctrl-C vira SIGINT de verdade. Durante uma pergunta, o
 * readline captura o Ctrl-C; ele cancela esta e as próximas perguntas (aprovações viram
 * "não"; perguntas do agente ficam sem resposta) e repassa o sinal (síncrono, direto ao
 * handler do host) para ele interromper o run e limpar o que criou.
 */
function terminalPrompts(args: Args): { ask?: (req: ApprovalRequest) => Promise<ApprovalAnswer>; askUser?: (question: string) => Promise<string> } {
	const noInput = args.noInput || ["1", "true"].includes(process.env.WB_NO_INPUT ?? "");
	if (noInput || !process.stdin.isTTY || !process.stderr.isTTY) return {};
	const cancel = new AbortController();
	const question = async (query: string): Promise<string> => {
		const rl = createInterface({ input: process.stdin, output: process.stderr });
		rl.on("SIGINT", () => {
			cancel.abort();
			process.emit("SIGINT", "SIGINT");
		});
		try {
			return await rl.question(query, { signal: cancel.signal });
		} finally {
			rl.close();
		}
	};
	const ask = async (req: ApprovalRequest): Promise<ApprovalAnswer> => {
		const verbo = req.mode === "write" ? "ESCREVER em" : "LER";
		process.stderr.write(`\n${bold(`? O agente quer ${verbo} ${oneLine(req.path)}`)} ${dim(`(${oneLine(req.reason)})`)}\n`);
		for (;;) {
			const a = (await question("  [s] sim, desta vez   [t] sim, até o fim desta execução   [n] não  > ")).trim().toLowerCase();
			if (a === "s") return "once";
			if (a === "t") return "run";
			if (a === "n" || a === "") return "deny";
		}
	};
	const askUser = async (q: string): Promise<string> => {
		process.stderr.write(`\n${bold("? Pergunta do agente:")} ${safe(q)}\n`);
		// Cancelada pelo Ctrl-C: resposta vazia, que o host trata como "sem resposta".
		return question("  > ").catch(() => "");
	};
	return { ask, askUser };
}

const onDecision = (d: Decision) =>
	process.stderr.write(dim(`\n[${d.allowed ? (d.by === "user" ? "liberado" : "ok") : "BLOQUEADO"}] ${oneLine(d.tool)} ${oneLine(d.target)}${d.allowed ? "" : ` (${oneLine(d.reason ?? "")})`}\n`));

function summary(r: RunResult) {
	const blocked = r.decisions.filter((d) => !d.allowed).length;
	console.error(dim(`\n— ${r.scope.profile.name} · ${r.decisions.length} ações, ${blocked} bloqueadas`));
	console.error(dim(`— arquivos alterados: ${r.changedFiles.join(", ") || "nenhum (ou projeto sem git)"}`));
	console.error(dim(`— tokens: ${tokens(r.usage)}`));
	console.error(dim(`— auditoria: ${r.auditPath}`));
}

const k = (n: number) => (n < 1000 ? String(n) : `${(n / 1000).toFixed(n < 10_000 ? 1 : 0)}k`);

function tokens(u: TokenUsage): string {
	const parts = [`entrada ${k(u.input)}`, `cache ${k(u.cacheRead + u.cacheWrite)}`, `saída ${k(u.output)}${u.reasoning ? ` (${k(u.reasoning)} de raciocínio)` : ""}`];
	const cost = u.cost > 0 ? ` · US$ ${u.cost.toFixed(2)}${u.costFromProvider ? "" : " estimado"}` : "";
	return `${k(u.total)} em ${u.responses} respostas (${parts.join(", ")})${cost}`;
}

function asStep(id: string | undefined): StepId {
	if (!id || !STEP_IDS.includes(id as StepId)) throw new Error(`etapa desconhecida "${id ?? ""}" (use ${STEP_IDS.join(", ")})`);
	return id as StepId;
}

function statusCommand(root: string, args: Args, rest: string[]) {
	checkUsage("status", args, rest, { flags: [] });
	const state = loadState(root);
	const labels: Record<string, string> = { approved: "aprovado", "changed-after": "ALTERADO depois da aprovação", pending: "aguardando aprovação", missing: "" };
	for (const id of STEP_IDS) {
		const def = STEPS[id];
		const has = existsSync(path.join(root, def.output));
		const run = state.runs[id];
		const blocker = preflight(root, def);
		const parts = [has ? `${def.output}` : dim("sem saída")];
		if (id === "design" && has) parts.push(labels[approvalStatus(root, id, state)]);
		if (run) parts.push(dim(`última execução ${run.at.slice(0, 16).replace("T", " ")}`));
		if (!has && blocker) parts.push(dim(`(${blocker})`));
		console.log(`${has ? "●" : "○"} ${id.padEnd(10)} ${parts.filter(Boolean).join(" · ")}`);
	}
	const next = STEP_IDS.find((id) => !existsSync(path.join(root, STEPS[id].output)));
	const design = approvalStatus(root, "design", state);
	if (design === "pending" || design === "changed-after") {
		console.log(`\nPróximo passo: revise ${STEPS.design.output} e rode "wb step approve design".`);
	} else if (next) {
		console.log(`\nPróximo passo: wb step run ${next}${next === "brainstorm" ? ' "sua ideia"' : ""}`);
	}
}

function version(): string {
	const pkg = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8")) as { version: string };
	return `wb ${pkg.version}`;
}

type Flag = "sandbox" | "model" | "thinking" | "noInput";
const FLAG_NAMES: Record<Flag, string> = { sandbox: "--sandbox", model: "--model", thinking: "--thinking", noInput: "--no-input" };

/** Recusa flags e argumentos que o comando não usa, em vez de ignorá-los calado. */
function checkUsage(command: string, args: Args, rest: string[], { flags, min = 0, max = min }: { flags: Flag[]; min?: number; max?: number }) {
	const help = HELP[command.split(" ")[0]] ?? HELP.main;
	for (const f of Object.keys(FLAG_NAMES) as Flag[]) {
		if (!flags.includes(f) && args[f] !== undefined && args[f] !== false) throw new Error(`${FLAG_NAMES[f]} não vale para "wb ${command}"\n\n${help}`);
	}
	if (rest.length > max) throw new Error(`argumento a mais para "wb ${command}": ${rest.slice(max).join(" ")}\n\n${help}`);
	if (rest.length < min) throw new Error(`faltam argumentos para "wb ${command}"\n\n${help}`);
}

function projectRoot(dir: string): string {
	const st = statSync(dir, { throwIfNoEntry: false });
	if (!st) throw new Error(`projeto não encontrado: ${dir}`);
	if (!st.isDirectory()) throw new Error(`o projeto precisa ser um diretório: ${dir}`);
	return realpathSync(dir);
}

function runOptions(root: string, args: Args) {
	return { projectDir: root, model: args.model, thinkingLevel: args.thinking, ...terminalPrompts(args), onText: (t: string) => process.stdout.write(t), onDecision };
}

function scopeCommand(root: string, args: Args, rest: string[]) {
	checkUsage("scope", args, rest, { flags: ["sandbox"] });
	const scope = resolveScope(args.sandbox, loadProjectConfig(root));
	console.log(describeScope(scope, detectCommands(root)));
	// A .workbench/ da raiz também é negada, mas é do próprio wb: não entra na lista de sensíveis.
	const masked = findDeniedFiles(root, scope.deny).map((f) => f.slice(root.length + 1)).filter((f) => f !== ".workbench");
	console.log(`\nArquivos sensíveis encontrados (negados e mascarados no sandbox): ${masked.join(", ") || "nenhum"}`);
	console.log(`Sandbox (bwrap): ${sandboxAvailable() ? "disponível" : "NÃO ENCONTRADO"}`);
}

async function runCommand(root: string, args: Args, rest: string[]) {
	if (rest.length === 0) throw new Error(`falta a tarefa\n\n${HELP.run}`);
	checkUsage("run", args, rest, { flags: ["sandbox", "model", "thinking", "noInput"], max: Infinity });
	summary(await runTask({ ...runOptions(root, args), task: rest.join(" "), sandbox: args.sandbox }));
}

async function stepCommand(root: string, args: Args, rest: string[]) {
	const [sub, ...stepArgs] = rest;
	if (sub === "approve") return stepApprove(root, args, stepArgs);
	if (sub === "run") return stepRun(root, args, stepArgs);
	throw new Error(`subcomando desconhecido "step ${sub ?? ""}" (use step run ou step approve)\n\n${HELP.step}`);
}

/**
 * A aprovação é da pessoa, não do agente: só vale num terminal interativo, com a palavra
 * "aprovar" digitada. É uma trava contra aprovação por engano (um agente rodando o wb não
 * tem terminal), não contra um agente decidido a contorná-la, que poderia simular um com
 * `script`. A garantia de verdade vem da aprovação por PR (ADR 0003).
 */
async function stepApprove(root: string, args: Args, rest: string[]) {
	checkUsage("step approve", args, rest, { flags: [], min: 1 });
	const id = asStep(rest[0]);
	if (!process.stdin.isTTY || !process.stderr.isTTY) {
		throw new Error(`"wb step approve" só roda num terminal interativo: a aprovação é sua, não do agente. Rode no seu terminal: wb step approve ${id}`);
	}
	const rl = createInterface({ input: process.stdin, output: process.stderr });
	let answer: string;
	try {
		answer = (await rl.question(`Revisou ${STEPS[id].output}? Digite "aprovar" para aprovar: `)).trim().toLowerCase();
	} finally {
		rl.close();
	}
	if (answer !== "aprovar") throw new Error("não aprovado");
	const a = approve(root, id);
	console.log(`${a.file} aprovado (sha256 ${a.sha256.slice(0, 12)}…). Se o arquivo mudar, a aprovação deixa de valer.`);
}

async function stepRun(root: string, args: Args, rest: string[]) {
	checkUsage("step run", args, rest, { flags: ["model", "thinking", "noInput"], max: Infinity });
	const id = asStep(rest[0]);
	const r = await runStep({ ...runOptions(root, args), id, note: rest.slice(1).join(" ") || undefined });
	summary(r);
	console.error(r.outputWritten ? `\n${bold(`✓ ${r.output} escrito.`)}` : `\n${bold(`! ${r.output} não foi escrito nesta execução.`)}`);
	if (id === "design" && r.outputWritten) console.error("Revise o design e aprove para liberar o build: wb step approve design");
}

const COMMANDS: Record<string, (root: string, args: Args, rest: string[]) => void | Promise<void>> = {
	run: runCommand,
	step: stepCommand,
	status: statusCommand,
	scope: scopeCommand,
};

async function main() {
	// Flags valem antes ou depois do comando: "wb -C ~/proj status" e "wb status -C ~/proj".
	const args = parseArgs(process.argv.slice(2));
	const [cmd, ...rest] = args.rest;
	if (args.version) return console.log(version());
	if (!cmd) return console.log(HELP.main);
	if (cmd === "help") return console.log(HELP[rest[0] ?? "main"] ?? HELP.main);
	if (args.help) return console.log(HELP[cmd] ?? HELP.main);
	const command = COMMANDS[cmd];
	if (!command) throw new Error(`comando desconhecido "${cmd}"\n\n${HELP.main}`);
	await command(projectRoot(args.project), args, rest);
}

main().catch((err) => {
	console.error(err instanceof Error ? err.message : err);
	process.exit(1);
});
