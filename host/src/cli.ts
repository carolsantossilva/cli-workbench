#!/usr/bin/env node
// wb: CLI do micro MVP do host da Workbench.
import { realpathSync } from "node:fs";
import { createInterface } from "node:readline/promises";
import { detectCommands } from "./detect.ts";
import { type ApprovalAnswer, type ApprovalRequest, DEFAULT_MODEL, defaultAuditDir, describeScope, runTask } from "./host.ts";
import { loadProjectConfig, PROFILE_NAMES, type ProfileName, resolveScope } from "./profiles.ts";
import { findDeniedFiles, sandboxAvailable } from "./sandbox.ts";

const USAGE = `uso:
  wb run <projeto> [--perfil leitura|docs|codigo] [--modelo provider/modelo] [--sem-perguntas] <tarefa...>
  wb escopo <projeto> [--perfil leitura|docs|codigo]

Perfil padrão: o de .workbench.json do projeto, senão "leitura".
Fora do perfil, a Workbench pergunta antes de ler ou escrever (só em terminal interativo).
Com --sem-perguntas, ou sem terminal, tudo fora do perfil é negado.
Modelo padrão: ${DEFAULT_MODEL} (faça login antes com: npx pi auth login openai-codex)
Auditoria: ${defaultAuditDir()}/<run>.jsonl`;

function parse(argv: string[]) {
	const flags: Record<string, string> = {};
	const rest: string[] = [];
	let semPerguntas = false;
	for (let i = 0; i < argv.length; i++) {
		const a = argv[i];
		if (a === "--perfil" || a === "--modelo") flags[a.slice(2)] = argv[++i] ?? "";
		else if (a === "--sem-perguntas") semPerguntas = true;
		else rest.push(a);
	}
	const perfil = flags.perfil as ProfileName | undefined;
	if (perfil && !PROFILE_NAMES.includes(perfil)) throw new Error(`perfil desconhecido "${perfil}" (use ${PROFILE_NAMES.join(", ")})`);
	return { perfil, modelo: flags.modelo, semPerguntas, rest };
}

async function main() {
	const [cmd, ...argv] = process.argv.slice(2);
	if (!cmd || cmd === "-h" || cmd === "--help") return console.log(USAGE);
	const { perfil, modelo, semPerguntas, rest } = parse(argv);
	const [projeto, ...tarefa] = rest;
	if (!projeto) throw new Error(USAGE);

	if (cmd === "escopo") {
		const root = realpathSync(projeto);
		const scope = resolveScope(perfil, loadProjectConfig(root));
		console.log(describeScope(scope, detectCommands(root)));
		const masked = findDeniedFiles(root, scope.deny).map((f) => f.slice(root.length + 1));
		console.log(`\nArquivos sensíveis encontrados (negados e mascarados no sandbox): ${masked.join(", ") || "nenhum"}`);
		if (scope.profile.bash === "sandbox") console.log(`Sandbox (bwrap): ${sandboxAvailable() ? "disponível" : "NÃO ENCONTRADO"}`);
		return;
	}
	if (cmd !== "run") throw new Error(`comando desconhecido "${cmd}"\n\n${USAGE}`);
	if (tarefa.length === 0) throw new Error("falta a tarefa");

	const dim = (s: string) => (process.stderr.isTTY ? `\x1b[2m${s}\x1b[0m` : s);
	const bold = (s: string) => (process.stderr.isTTY ? `\x1b[1m${s}\x1b[0m` : s);
	const interactive = !semPerguntas && process.stdin.isTTY && process.stderr.isTTY;
	const rl = interactive ? createInterface({ input: process.stdin, output: process.stderr }) : undefined;
	const ask = rl
		? async (req: ApprovalRequest): Promise<ApprovalAnswer> => {
				const verbo = req.mode === "write" ? "ESCREVER em" : "LER";
				process.stderr.write(`\n${bold(`? O agente quer ${verbo} ${req.path}`)} ${dim(`(${req.reason})`)}\n`);
				for (;;) {
					const a = (await rl.question("  [s] sim, desta vez   [t] sim, até o fim desta execução   [n] não  > ")).trim().toLowerCase();
					if (a === "s") return "once";
					if (a === "t") return "run";
					if (a === "n" || a === "") return "deny";
				}
			}
		: undefined;
	const result = await runTask({
		ask,
		projectDir: projeto,
		task: tarefa.join(" "),
		profile: perfil,
		model: modelo,
		onText: (t) => process.stdout.write(t),
		onDecision: (d) =>
			process.stderr.write(dim(`\n[${d.allowed ? (d.by === "usuario" ? "liberado" : "ok") : "BLOQUEADO"}] ${d.tool} ${d.target}${d.allowed ? "" : ` (${d.reason})`}\n`)),
	}).finally(() => rl?.close());
	const blocked = result.decisions.filter((d) => !d.allowed).length;
	console.error(dim(`\n— perfil ${result.scope.profile.name} · ${result.decisions.length} ações, ${blocked} bloqueadas`));
	console.error(dim(`— arquivos alterados: ${result.changedFiles.join(", ") || "nenhum (ou projeto sem git)"}`));
	console.error(dim(`— auditoria: ${result.auditPath}`));
}

main().catch((err) => {
	console.error(err instanceof Error ? err.message : err);
	process.exit(1);
});
