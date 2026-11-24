#!/usr/bin/env node
// wb: CLI do micro MVP do host da Workbench.
// Nomes de comandos, flags e chaves em inglês, seguindo as convenções de CLI (clig.dev,
// GNU, POSIX); mensagens e ajuda em português.
import { existsSync, readFileSync, realpathSync } from "node:fs";
import path from "node:path";
import { createInterface, type Interface } from "node:readline/promises";
import { detectCommands } from "./detect.ts";
import { type ApprovalAnswer, type ApprovalRequest, DEFAULT_MODEL, DEFAULT_THINKING, type Decision, defaultAuditDir, describeScope, type RunResult, runTask, THINKING_LEVELS } from "./host.ts";
import { loadProjectConfig, resolveScope, SANDBOX_NAMES } from "./profiles.ts";
import { findDeniedFiles, sandboxAvailable } from "./sandbox.ts";
import { parseArgs } from "./args.ts";
import { runStep } from "./step.ts";
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
  step approve <etapa>         aprova a saída de uma etapa (hoje: design) pelo sha256
  status                       mostra em que etapa o projeto está e o próximo passo
  scope                        mostra o que o agente pode ler, escrever e rodar
  help [comando]               ajuda de um comando

${COMMON_OPTIONS}
  -V, --version              mostra a versão

Auditoria: ${defaultAuditDir()}/<run>.jsonl
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
// Texto vindo do modelo (caminhos, comandos, perguntas) não pode trazer sequências ANSI
// ou quebras de linha que falsifiquem o que aparece no terminal.
const safe = (s: string) => s.replace(/[\u0000-\u0008\u000b-\u001f\u007f-\u009f\u200e\u200f\u202a-\u202e\u2066-\u2069]/g, (c) => `\\x${c.charCodeAt(0).toString(16).padStart(2, "0")}`);
const oneLine = (s: string) => safe(s).replace(/\n/g, "\\n");
const dim = (s: string) => (color ? `\x1b[2m${s}\x1b[0m` : s);
const bold = (s: string) => (color ? `\x1b[1m${s}\x1b[0m` : s);

function prompts(rl: Interface | undefined) {
	if (!rl) return { ask: undefined, askUser: undefined };
	const ask = async (req: ApprovalRequest): Promise<ApprovalAnswer> => {
		const verbo = req.mode === "write" ? "ESCREVER em" : "LER";
		process.stderr.write(`\n${bold(`? O agente quer ${verbo} ${oneLine(req.path)}`)} ${dim(`(${oneLine(req.reason)})`)}\n`);
		for (;;) {
			const a = (await rl.question("  [s] sim, desta vez   [t] sim, até o fim desta execução   [n] não  > ")).trim().toLowerCase();
			if (a === "s") return "once";
			if (a === "t") return "run";
			if (a === "n" || a === "") return "deny";
		}
	};
	const askUser = async (question: string): Promise<string> => {
		process.stderr.write(`\n${bold("? Pergunta do agente:")} ${safe(question)}\n`);
		return rl.question("  > ");
	};
	return { ask, askUser };
}

const onDecision = (d: Decision) =>
	process.stderr.write(dim(`\n[${d.allowed ? (d.by === "user" ? "liberado" : "ok") : "BLOQUEADO"}] ${oneLine(d.tool)} ${oneLine(d.target)}${d.allowed ? "" : ` (${oneLine(d.reason ?? "")})`}\n`));

function summary(r: RunResult) {
	const blocked = r.decisions.filter((d) => !d.allowed).length;
	console.error(dim(`\n— ${r.scope.profile.name} · ${r.decisions.length} ações, ${blocked} bloqueadas`));
	console.error(dim(`— arquivos alterados: ${r.changedFiles.join(", ") || "nenhum (ou projeto sem git)"}`));
	console.error(dim(`— auditoria: ${r.auditPath}`));
}

function asStep(id: string | undefined): StepId {
	if (!id || !STEP_IDS.includes(id as StepId)) throw new Error(`etapa desconhecida "${id ?? ""}" (use ${STEP_IDS.join(", ")})`);
	return id as StepId;
}

function status(root: string) {
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

async function main() {
	// Flags valem antes ou depois do comando: "wb -C ~/proj status" e "wb status -C ~/proj".
	const args = parseArgs(process.argv.slice(2));
	const [cmd, ...rest] = args.rest;
	args.rest = rest;
	if (args.version) return console.log(version());
	if (!cmd) return console.log(HELP.main);
	if (cmd === "help") return console.log(HELP[rest[0] ?? "main"] ?? HELP.main);
	if (args.help) return console.log(HELP[cmd] ?? HELP.main);
	const root = realpathSync(args.project);

	switch (cmd) {
		case "scope": {
			const scope = resolveScope(args.sandbox, loadProjectConfig(root));
			console.log(describeScope(scope, detectCommands(root)));
			const masked = findDeniedFiles(root, scope.deny).map((f) => f.slice(root.length + 1));
			console.log(`\nArquivos sensíveis encontrados (negados e mascarados no sandbox): ${masked.join(", ") || "nenhum"}`);
			console.log(`Sandbox (bwrap): ${sandboxAvailable() ? "disponível" : "NÃO ENCONTRADO"}`);
			return;
		}
		case "status":
			return status(root);
		case "run":
		case "step": {
			let sub = "run";
			let rest = args.rest;
			if (cmd === "step") {
				[sub, ...rest] = args.rest;
				if (sub === "approve") {
					const a = approve(root, asStep(rest[0]));
					console.log(`${a.file} aprovado (sha256 ${a.sha256.slice(0, 12)}…). Se o arquivo mudar, a aprovação deixa de valer.`);
					return;
				}
				if (sub !== "run") throw new Error(`subcomando desconhecido "step ${sub ?? ""}" (use step run ou step approve)\n\n${HELP.step}`);
			}
			const noInput = args.noInput || ["1", "true"].includes(process.env.WB_NO_INPUT ?? "");
			const interactive = !noInput && process.stdin.isTTY && process.stderr.isTTY;
			const rl = interactive ? createInterface({ input: process.stdin, output: process.stderr }) : undefined;
			// Com o readline ativo, Ctrl-C não vira sinal; repassa para o host limpar o run.
			rl?.on("SIGINT", () => process.kill(process.pid, "SIGINT"));
			const { ask, askUser } = prompts(rl);
			const common = { projectDir: root, model: args.model, thinkingLevel: args.thinking, ask, askUser, onText: (t: string) => process.stdout.write(t), onDecision };
			try {
				if (cmd === "run") {
					if (rest.length === 0) throw new Error(`falta a tarefa\n\n${HELP.run}`);
					summary(await runTask({ ...common, task: rest.join(" "), sandbox: args.sandbox }));
				} else {
					const id = asStep(rest[0]);
					const r = await runStep({ ...common, id, note: rest.slice(1).join(" ") || undefined });
					summary(r);
					console.error(r.outputWritten ? `\n${bold(`✓ ${r.output} escrito.`)}` : `\n${bold(`! ${r.output} não foi escrito nesta execução.`)}`);
					if (id === "design" && r.outputWritten) console.error("Revise o design e aprove para liberar o build: wb step approve design");
				}
			} finally {
				rl?.close();
			}
			return;
		}
		default:
			throw new Error(`comando desconhecido "${cmd}"\n\n${HELP.main}`);
	}
}

main().catch((err) => {
	console.error(err instanceof Error ? err.message : err);
	process.exit(1);
});
