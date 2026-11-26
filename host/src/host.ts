// Host da Workbench sobre o SDK do Pi: monta uma sessão com o escopo do sandbox, roda a
// tarefa e devolve o que aconteceu. As regras ficam em gate.ts (o que pode), extension.ts
// (como o Pi as aplica), redact.ts (o que sai para o modelo) e usage.ts (auditoria e tokens).
import { execFileSync } from "node:child_process";
import { mkdirSync, realpathSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import type { Api, Model } from "@earendil-works/pi-ai";
import {
	type AgentSession,
	createAgentSession,
	createBashToolDefinition,
	DefaultResourceLoader,
	type ExtensionAPI,
	ModelRuntime,
	SessionManager,
	SettingsManager,
} from "@earendil-works/pi-coding-agent";
import { type DetectedCommand, detectCommands } from "./detect.ts";
import { workbenchExtension } from "./extension.ts";
import { type ApprovalAnswer, type ApprovalRequest, createGate, type Decision } from "./gate.ts";
import { beginGuard, endGuard, recoverInterrupted, type Snapshot } from "./guard.ts";
import { assertProjectRoot, redactSecrets } from "./policy.ts";
import { type EffectiveScope, type SandboxName, resolveScope } from "./sandbox-profiles.ts";
import { loadProjectConfig, type ProjectConfig } from "./project-config.ts";
import { looksLikeRefusal, rewritePrompt } from "./refusal.ts";
import { sandboxAvailable, wrapCommand } from "./sandbox.ts";
import { type Audit, emptyUsage, newRunId, openAudit, recordUsage, type TokenUsage } from "./usage.ts";
import { auditDirFor } from "./workdir.ts";

export type { ApprovalAnswer, ApprovalRequest, Decision } from "./gate.ts";
export type { TokenUsage } from "./usage.ts";

export interface RunOptions {
	projectDir: string;
	task: string;
	sandbox?: SandboxName;
	/** "provider/modelo", ex.: openai-codex/gpt-6-luna */
	model?: string;
	/** Esforço de raciocínio (padrão xhigh); o Pi ajusta ao que o modelo suporta. */
	thinkingLevel?: ThinkingLevel;
	/** Para testes: runtime e modelo já prontos (ex.: provider faux). */
	modelRuntime?: ModelRuntime;
	modelOverride?: Model<Api>;
	auditDir?: string;
	agentDir?: string;
	onText?: (delta: string) => void;
	onDecision?: (d: Decision) => void;
	/** Pergunta ao usuário sobre ações fora do sandbox. Sem isso, tudo fora do sandbox é negado. */
	ask?: (req: ApprovalRequest) => Promise<ApprovalAnswer>;
	/**
	 * Escopo pronto no lugar do sandbox, ou montado a partir do `.workbench.json` (ex.: de
	 * um workflow). A função roda depois da limpeza de um run interrompido, para que a
	 * auditoria registre o que foi posto de lado e uma configuração plantada não seja lida.
	 */
	scope?: EffectiveScope | ((cfg: ProjectConfig) => EffectiveScope);
	/** Habilita a ferramenta `ask_user`: o agente faz uma pergunta e recebe a resposta do usuário. */
	askUser?: (question: string) => Promise<string>;
	/** Quantas vezes pedir para continuar após uma recusa (padrão 1). */
	refusalRetries?: number;
}

export interface RunResult {
	runId: string;
	scope: EffectiveScope;
	commands: DetectedCommand[];
	decisions: Decision[];
	changedFiles: string[];
	finalText: string;
	auditPath: string;
	usage: TokenUsage;
}

export const DEFAULT_MODEL = "openai-codex/gpt-6-luna";
// Mesmo conjunto do ThinkingLevel do Pi (pi-agent-core não é dependência direta).
export type ThinkingLevel = "off" | "minimal" | "low" | "medium" | "high" | "xhigh" | "max";
export const DEFAULT_THINKING: ThinkingLevel = "xhigh";
export const THINKING_LEVELS: ThinkingLevel[] = ["off", "minimal", "low", "medium", "high", "xhigh", "max"];

function throwIfModelError(last: unknown): void {
	const m = last as { role?: string; stopReason?: string; errorMessage?: string } | undefined;
	if (m?.role === "assistant" && m.stopReason === "error") throw new Error(`erro do modelo: ${m.errorMessage ?? "desconhecido"}`);
}

function gitChanges(root: string): Set<string> | undefined {
	try {
		// Sem fsmonitor e sem hooks: o git do host não executa nada que o projeto configure.
		const out = execFileSync("git", ["-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null", "status", "--porcelain=v1", "-z", "--untracked-files=all"], { cwd: root, encoding: "utf8" });
		const items = out.split("\0").filter(Boolean);
		const files = new Set<string>();
		for (let i = 0; i < items.length; i++) {
			files.add(items[i].slice(3));
			// Em renomeação/cópia, o item seguinte é o caminho antigo, sem prefixo de status.
			if (/^[RC]|^.[RC]/.test(items[i])) files.add(items[++i]);
		}
		return files;
	} catch {
		return undefined;
	}
}

export function describeScope(scope: EffectiveScope, commands: DetectedCommand[], interactive = false): string {
	const p = scope.profile;
	return [
		`# Workbench: sandbox "${p.name}"`,
		p.description,
		`Leitura: ${scope.read.join(", ")}`,
		`Escrita: ${scope.write.join(", ") || "nenhuma"}`,
		`Sempre negado: ${scope.deny.join(", ")}`,
		p.bash !== "none"
			? `Comandos: bash disponível num sandbox SEM rede (não dá para instalar dependências nem acessar a internet)${p.bash === "sandbox-ro" ? ", com o projeto SÓ LEITURA" : ""}. Comandos detectados no projeto: ${commands.map((c) => c.command).join(", ") || "nenhum"}.`
			: "Comandos: nenhum. O bash não está disponível neste sandbox.",
		interactive
			? "Se a tarefa exigir ler ou escrever um arquivo fora desse escopo, faça a chamada mesmo assim: o host pergunta ao usuário, que pode liberar ou negar. Arquivos sensíveis, caminhos fora da raiz, .git e a configuração da Workbench nunca são liberados. Se uma chamada for negada, não tente contornar: siga sem ela e diga no resumo final o que faltou."
			: "Chamadas fora desse escopo são bloqueadas pelo host. Não tente contornar o bloqueio: se precisar de algo bloqueado, diga no resumo final o que faltou e por quê.",
		"Ao terminar, escreva um resumo curto em português: o que fez, arquivos alterados e o que ficou pendente.",
	].join("\n");
}

export async function runTask(opts: RunOptions): Promise<RunResult> {
	const root = realpathSync(opts.projectDir);
	assertProjectRoot(root);
	const recovered = recoverInterrupted(root);
	const config = loadProjectConfig(root);
	const scope = typeof opts.scope === "function" ? opts.scope(config) : (opts.scope ?? resolveScope(opts.sandbox, config));
	const commands = detectCommands(root);
	if (scope.profile.bash !== "none" && !sandboxAvailable()) {
		throw new Error(`"${scope.profile.name}" precisa do bubblewrap (bwrap) instalado para rodar comandos em sandbox`);
	}

	const runId = newRunId();
	const auditDir = opts.auditDir ?? auditDirFor(root);
	mkdirSync(auditDir, { recursive: true });
	const auditPath = path.join(auditDir, `${runId}.jsonl`);
	const audit = openAudit(auditPath, runId);
	const gate = createGate({ root, scope, audit, ask: opts.ask, onDecision: opts.onDecision });

	const { modelRuntime, model } = await resolveModel(opts);
	const agentDir = opts.agentDir ?? path.join(os.homedir(), ".pi", "agent");
	const settingsManager = SettingsManager.inMemory({ compaction: { enabled: true } });
	const extension = workbenchExtension({ root, scope, gate, audit, askUser: opts.askUser, wikiEnabled: config.kbAi?.wiki === true });
	const resourceLoader = await loadResources(root, agentDir, settingsManager, extension, describeScope(scope, commands, Boolean(opts.ask)));

	const before = gitChanges(root);
	const guard = beginGuard(root, runId);
	for (const q of recovered) audit("violation", { ...q, action: "created in an interrupted run; moved aside" });
	const rewritten = rewritePrompt(opts.task);
	audit("start", { projectDir: root, profile: scope.profile.name, interactive: Boolean(opts.ask), model: `${model.provider}/${model.id}`, task: redactSecrets(rewritten.text).text, write: scope.write, commands: commands.map((c) => c.command) });

	const { session } = await createAgentSession({
		cwd: root,
		agentDir,
		model,
		thinkingLevel: opts.thinkingLevel ?? DEFAULT_THINKING,
		modelRuntime,
		resourceLoader,
		settingsManager,
		...sessionTools(root, scope, opts, config.kbAi?.wiki === true),
		sessionManager: SessionManager.inMemory(root),
	});
	await session.bindExtensions?.({});
	if (rewritten.count > 0) {
		audit("prompt_rewrite", { count: rewritten.count, reason: "word that triggers spurious refusals" });
		opts.onText?.(`[workbench: troquei ${rewritten.count} palavra(s) que fazem o modelo recusar; veja "prompt_rewrite" na auditoria]\n`);
	}
	audit("thinking", { requested: opts.thinkingLevel ?? DEFAULT_THINKING, effective: session.thinkingLevel });

	const usage = emptyUsage();
	const unsubscribe = session.subscribe((event) => {
		if (event.type === "message_update" && event.assistantMessageEvent.type === "text_delta") opts.onText?.(event.assistantMessageEvent.delta);
		if (event.type === "message_end") recordUsage(usage, event.message, audit);
	});
	const signals = interruptOnSignal(session);
	let finalText = "";
	try {
		finalText = await promptWithRetries(session, rewritten.text, opts, audit);
	} finally {
		signals.release();
		unsubscribe();
		session.dispose();
		quarantineProtected(root, guard, audit, opts.onText);
	}
	if (signals.interrupted()) {
		audit("end", { interrupted: true, usage });
		throw new Error("execução interrompida");
	}

	const after = gitChanges(root);
	const changedFiles = before && after ? [...after].filter((f) => !before.has(f)).sort() : [];
	audit("end", { changedFiles, blocked: gate.decisions.filter((d) => !d.allowed).length, usage });
	return { runId, scope, commands, decisions: gate.decisions, changedFiles, finalText, auditPath, usage };
}

async function resolveModel(opts: RunOptions): Promise<{ modelRuntime: ModelRuntime; model: Model<Api> }> {
	const modelRuntime = opts.modelRuntime ?? (await ModelRuntime.create());
	if (opts.modelOverride) return { modelRuntime, model: opts.modelOverride };
	const ref = opts.model ?? DEFAULT_MODEL;
	const [provider, ...rest] = ref.split("/");
	const model = modelRuntime.getModel(provider, rest.join("/"));
	if (!model) throw new Error(`modelo não encontrado: ${ref}`);
	return { modelRuntime, model };
}

/** Sem extensões, skills ou prompts do usuário: só a extensão da Workbench e a descrição do escopo. */
async function loadResources(root: string, agentDir: string, settingsManager: SettingsManager, extension: (pi: ExtensionAPI) => void, systemPrompt: string) {
	const resourceLoader = new DefaultResourceLoader({
		cwd: root,
		agentDir,
		settingsManager,
		extensionFactories: [extension],
		noExtensions: true,
		noSkills: true,
		noPromptTemplates: true,
		noThemes: true,
		appendSystemPrompt: [systemPrompt],
	});
	await resourceLoader.reload();
	return resourceLoader;
}

/** A allowlist de ferramentas da sessão; o "bash" é a definição que roda no sandbox. */
function sessionTools(root: string, scope: EffectiveScope, opts: RunOptions, wiki: boolean) {
	const tools = ["read", "grep", "find", "ls"];
	// Com aprovação interativa, até o sandbox read-only recebe write/edit: cada escrita vira pergunta.
	if (scope.write.length > 0 || opts.ask) tools.push("write", "edit");
	if (scope.profile.bash !== "none") tools.push("bash");
	if (opts.askUser) tools.push("ask_user");
	if (wiki) tools.push("wiki");
	const customTools =
		scope.profile.bash !== "none"
			? [
					createBashToolDefinition(root, {
						exposeSessionEnvironment: false,
						spawnHook: (c) => ({ ...c, command: wrapCommand(c.command, root, c.cwd, scope.deny, scope.profile.bash === "sandbox") }),
					}),
				]
			: [];
	return { tools, customTools: customTools as never };
}

/** Recusas espúrias acontecem de forma intermitente: uma retomada simples, sem argumentar. */
async function promptWithRetries(session: AgentSession, task: string, opts: RunOptions, audit: Audit): Promise<string> {
	await session.prompt(task);
	throwIfModelError(session.messages.at(-1));
	let finalText = session.getLastAssistantText() ?? "";
	for (let retry = 0; retry < (opts.refusalRetries ?? 1) && looksLikeRefusal(finalText); retry++) {
		audit("refusal", { retry: retry + 1, text: finalText.slice(0, 200) });
		opts.onText?.("\n[o modelo recusou; pedindo para continuar de onde parou]\n");
		await session.prompt("Continue a tarefa de onde parou.");
		throwIfModelError(session.messages.at(-1));
		finalText = session.getLastAssistantText() ?? "";
	}
	return finalText;
}

/** Ctrl-C/SIGTERM interrompem o modelo, mas deixam o finally do run limpar o que ele criou. */
function interruptOnSignal(session: AgentSession) {
	let interrupted = false;
	const onSignal = () => {
		interrupted = true;
		void session.abort();
	};
	process.once("SIGINT", onSignal);
	process.once("SIGTERM", onSignal);
	return {
		interrupted: () => interrupted,
		release: () => {
			process.off("SIGINT", onSignal);
			process.off("SIGTERM", onSignal);
		},
	};
}

/** .workbench.json e .git criados durante o run (só o bash conseguiria) não valem: vão para o lado. */
function quarantineProtected(root: string, guard: Snapshot, audit: Audit, onText?: (t: string) => void) {
	for (const q of endGuard(root, guard)) {
		audit("violation", { ...q, action: "created during the run; moved aside" });
		onText?.(`\n[workbench: ${q.path} criado durante a execução foi ignorado e movido para ${q.movedTo}]\n`);
	}
}
