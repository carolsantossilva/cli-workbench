#!/usr/bin/env node
// wb: CLI do micro MVP do host da Workbench.
import { existsSync, realpathSync } from "node:fs";
import path from "node:path";
import { createInterface, type Interface } from "node:readline/promises";
import { detectCommands } from "./detect.ts";
import { runWorkflow } from "./fluxo.ts";
import { type ApprovalAnswer, type ApprovalRequest, DEFAULT_MODEL, DEFAULT_THINKING, THINKING_LEVELS, type Decision, defaultAuditDir, describeScope, type RunResult, runTask } from "./host.ts";
import { loadProjectConfig, PROFILE_NAMES, type ProfileName, resolveScope } from "./profiles.ts";
import { findDeniedFiles, sandboxAvailable } from "./sandbox.ts";
import { approvalStatus, approve, loadState, preflight, WORKFLOW_IDS, type WorkflowId, WORKFLOWS } from "./workflows.ts";

const USAGE = `uso:
  wb run <projeto> [--perfil leitura|docs|codigo] [opções] <tarefa...>
  wb fluxo <projeto> <etapa> [opções] [texto...]
  wb aprovar <projeto> <etapa>
  wb status <projeto>
  wb escopo <projeto> [--perfil leitura|docs|codigo]

Etapas: ${WORKFLOW_IDS.join(" → ")}
  brainstorm recebe a ideia como texto; nas outras, o texto é um pedido extra.
  build só roda com o design aprovado (wb aprovar <projeto> design) e inalterado.
  Os arquivos de cada etapa ficam em docs/workbench/.

Opções:
  --modelo provider/modelo   padrão ${DEFAULT_MODEL} (login: npx pi auth login openai-codex)
  --raciocinio <nível>       ${THINKING_LEVELS.join("|")} (padrão ${DEFAULT_THINKING})
  --sem-perguntas            não pergunta nada: fora do escopo é negado e o agente decide com premissas

Perfil padrão do run: o de .workbench.json do projeto, senão "leitura".
Auditoria: ${defaultAuditDir()}/<run>.jsonl`;

function parse(argv: string[]) {
	const flags: Record<string, string> = {};
	const rest: string[] = [];
	let semPerguntas = false;
	for (let i = 0; i < argv.length; i++) {
		const a = argv[i];
		if (a === "--perfil" || a === "--modelo" || a === "--raciocinio") flags[a.slice(2)] = argv[++i] ?? "";
		else if (a === "--sem-perguntas") semPerguntas = true;
		else rest.push(a);
	}
	const perfil = flags.perfil as ProfileName | undefined;
	if (perfil && !PROFILE_NAMES.includes(perfil)) throw new Error(`perfil desconhecido "${perfil}" (use ${PROFILE_NAMES.join(", ")})`);
	const raciocinio = flags.raciocinio as (typeof THINKING_LEVELS)[number] | undefined;
	if (raciocinio && !THINKING_LEVELS.includes(raciocinio)) throw new Error(`nível de raciocínio desconhecido "${raciocinio}" (use ${THINKING_LEVELS.join(", ")})`);
	return { perfil, modelo: flags.modelo, raciocinio, semPerguntas, rest };
}

const dim = (s: string) => (process.stderr.isTTY ? `\x1b[2m${s}\x1b[0m` : s);
const bold = (s: string) => (process.stderr.isTTY ? `\x1b[1m${s}\x1b[0m` : s);

function prompts(rl: Interface | undefined) {
	if (!rl) return { ask: undefined, askUser: undefined };
	const ask = async (req: ApprovalRequest): Promise<ApprovalAnswer> => {
		const verbo = req.mode === "write" ? "ESCREVER em" : "LER";
		process.stderr.write(`\n${bold(`? O agente quer ${verbo} ${req.path}`)} ${dim(`(${req.reason})`)}\n`);
		for (;;) {
			const a = (await rl.question("  [s] sim, desta vez   [t] sim, até o fim desta execução   [n] não  > ")).trim().toLowerCase();
			if (a === "s") return "once";
			if (a === "t") return "run";
			if (a === "n" || a === "") return "deny";
		}
	};
	const askUser = async (question: string): Promise<string> => {
		process.stderr.write(`\n${bold("? Pergunta do agente:")} ${question}\n`);
		return rl.question("  > ");
	};
	return { ask, askUser };
}

const onDecision = (d: Decision) =>
	process.stderr.write(dim(`\n[${d.allowed ? (d.by === "usuario" ? "liberado" : "ok") : "BLOQUEADO"}] ${d.tool} ${d.target}${d.allowed ? "" : ` (${d.reason})`}\n`));

function summary(r: RunResult) {
	const blocked = r.decisions.filter((d) => !d.allowed).length;
	console.error(dim(`\n— ${r.scope.profile.name} · ${r.decisions.length} ações, ${blocked} bloqueadas`));
	console.error(dim(`— arquivos alterados: ${r.changedFiles.join(", ") || "nenhum (ou projeto sem git)"}`));
	console.error(dim(`— auditoria: ${r.auditPath}`));
}

function asWorkflow(id: string | undefined): WorkflowId {
	if (!id || !WORKFLOW_IDS.includes(id as WorkflowId)) throw new Error(`etapa desconhecida "${id ?? ""}" (use ${WORKFLOW_IDS.join(", ")})`);
	return id as WorkflowId;
}

function status(root: string) {
	const state = loadState(root);
	const labels: Record<string, string> = { aprovado: "aprovado", "alterado-depois": "ALTERADO depois da aprovação", pendente: "aguardando aprovação", "sem-arquivo": "" };
	for (const id of WORKFLOW_IDS) {
		const def = WORKFLOWS[id];
		const has = existsSync(path.join(root, def.output));
		const run = state.execucoes[id];
		const blocker = preflight(root, def);
		const parts = [has ? `${def.output}` : dim("sem saída")];
		if (id === "design" && has) parts.push(labels[approvalStatus(root, id, state)]);
		if (run) parts.push(dim(`última execução ${run.at.slice(0, 16).replace("T", " ")}`));
		if (!has && blocker) parts.push(dim(`(${blocker})`));
		console.log(`${has ? "●" : "○"} ${id.padEnd(10)} ${parts.filter(Boolean).join(" · ")}`);
	}
	const next = WORKFLOW_IDS.find((id) => !existsSync(path.join(root, WORKFLOWS[id].output)));
	if (approvalStatus(root, "design", state) === "pendente" || approvalStatus(root, "design", state) === "alterado-depois") {
		console.log(`\nPróximo passo: revise ${WORKFLOWS.design.output} e rode "wb aprovar ${root} design".`);
	} else if (next) {
		console.log(`\nPróximo passo: wb fluxo ${root} ${next}${next === "brainstorm" ? ' "sua ideia"' : ""}`);
	}
}

async function main() {
	const [cmd, ...argv] = process.argv.slice(2);
	if (!cmd || cmd === "-h" || cmd === "--help") return console.log(USAGE);
	const { perfil, modelo, raciocinio, semPerguntas, rest } = parse(argv);
	const [projeto, ...args] = rest;
	if (!projeto) throw new Error(USAGE);
	const root = realpathSync(projeto);

	switch (cmd) {
		case "escopo": {
			const scope = resolveScope(perfil, loadProjectConfig(root));
			console.log(describeScope(scope, detectCommands(root)));
			const masked = findDeniedFiles(root, scope.deny).map((f) => f.slice(root.length + 1));
			console.log(`\nArquivos sensíveis encontrados (negados e mascarados no sandbox): ${masked.join(", ") || "nenhum"}`);
			console.log(`Sandbox (bwrap): ${sandboxAvailable() ? "disponível" : "NÃO ENCONTRADO"}`);
			return;
		}
		case "status":
			return status(root);
		case "aprovar": {
			const id = asWorkflow(args[0]);
			const a = approve(root, id);
			console.log(`${a.file} aprovado (sha256 ${a.sha256.slice(0, 12)}…). Se o arquivo mudar, a aprovação deixa de valer.`);
			return;
		}
		case "run":
		case "fluxo": {
			const interactive = !semPerguntas && process.stdin.isTTY && process.stderr.isTTY;
			const rl = interactive ? createInterface({ input: process.stdin, output: process.stderr }) : undefined;
			const { ask, askUser } = prompts(rl);
			const common = { projectDir: root, model: modelo, thinkingLevel: raciocinio, ask, askUser, onText: (t: string) => process.stdout.write(t), onDecision };
			try {
				if (cmd === "run") {
					if (args.length === 0) throw new Error("falta a tarefa");
					summary(await runTask({ ...common, task: args.join(" "), profile: perfil }));
				} else {
					const id = asWorkflow(args[0]);
					const r = await runWorkflow({ ...common, id, note: args.slice(1).join(" ") || undefined });
					summary(r);
					console.error(r.outputWritten ? `\n${bold(`✓ ${r.output} escrito.`)}` : `\n${bold(`! ${r.output} não foi escrito nesta execução.`)}`);
					if (id === "design" && r.outputWritten) console.error(`Revise o design e aprove para liberar o build: wb aprovar ${root} design`);
				}
			} finally {
				rl?.close();
			}
			return;
		}
		default:
			throw new Error(`comando desconhecido "${cmd}"\n\n${USAGE}`);
	}
}

main().catch((err) => {
	console.error(err instanceof Error ? err.message : err);
	process.exit(1);
});
