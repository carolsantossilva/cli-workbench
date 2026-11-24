// Host da Workbench sobre o SDK do Pi: aplica o perfil antes de cada ação,
// roda o bash no sandbox, redige segredos e audita o que vai para o modelo.
import { execFileSync } from "node:child_process";
import { createHash, randomBytes } from "node:crypto";
import { appendFileSync, mkdirSync, realpathSync } from "node:fs";
import { beginGuard, endGuard, type Quarantined, recoverInterrupted } from "./guard.ts";
import os from "node:os";
import path from "node:path";
import type { Api, Model } from "@earendil-works/pi-ai";
import {
	createAgentSession,
	createBashToolDefinition,
	DefaultResourceLoader,
	type ExtensionAPI,
	ModelRuntime,
	SessionManager,
	SettingsManager,
} from "@earendil-works/pi-coding-agent";
import { Type } from "typebox";
import { type DetectedCommand, detectCommands } from "./detect.ts";
import { checkPath, matchesAny, redactSecrets } from "./policy.ts";
import { type EffectiveScope, loadProjectConfig, type SandboxName, resolveScope } from "./profiles.ts";
import { findDeniedFiles, sandboxAvailable, wrapCommand } from "./sandbox.ts";
import { auditDirFor } from "./workdir.ts";

export interface Decision {
	tool: string;
	target: string;
	allowed: boolean;
	reason?: string;
	/** Quem decidiu: o sandbox, uma regra fixa, ou o usuário ao ser perguntado. */
	by?: "sandbox" | "rule" | "user";
}

export interface ApprovalRequest {
	tool: string;
	mode: "read" | "write";
	path: string;
	reason: string;
}

/** once: só esta chamada. run: este arquivo, até o fim da execução. deny: nega. */
export type ApprovalAnswer = "once" | "run" | "deny";

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
	/** Escopo pronto (ex.: de um workflow) no lugar do sandbox. */
	scope?: EffectiveScope;
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

/** Tokens somados das respostas do modelo, como o Pi reporta (input não inclui o cache). */
export interface TokenUsage {
	input: number;
	output: number;
	cacheRead: number;
	cacheWrite: number;
	/** Parte de output; só alguns provedores informam. */
	reasoning: number;
	total: number;
	/** Custo em dólar: cobrado pelo provedor quando ele informa, senão estimado pelo catálogo do Pi. */
	cost: number;
	costFromProvider: boolean;
	responses: number;
}

interface PiUsage {
	input: number;
	output: number;
	cacheRead: number;
	cacheWrite: number;
	reasoning?: number;
	totalTokens: number;
	cost?: { total: number; totalFromProvider?: boolean };
}

const emptyUsage = (): TokenUsage => ({ input: 0, output: 0, cacheRead: 0, cacheWrite: 0, reasoning: 0, total: 0, cost: 0, costFromProvider: true, responses: 0 });

function addUsage(sum: TokenUsage, u: PiUsage): void {
	sum.input += u.input;
	sum.output += u.output;
	sum.cacheRead += u.cacheRead;
	sum.cacheWrite += u.cacheWrite;
	sum.reasoning += u.reasoning ?? 0;
	sum.total += u.totalTokens;
	sum.cost += u.cost?.total ?? 0;
	sum.costFromProvider &&= Boolean(u.cost?.totalFromProvider);
	sum.responses++;
}

export const DEFAULT_MODEL = "openai-codex/gpt-6-luna";
// Mesmo conjunto do ThinkingLevel do Pi (pi-agent-core não é dependência direta).
export type ThinkingLevel = "off" | "minimal" | "low" | "medium" | "high" | "xhigh" | "max";
export const DEFAULT_THINKING: ThinkingLevel = "xhigh";
export const THINKING_LEVELS: ThinkingLevel[] = ["off", "minimal", "low", "medium", "high", "xhigh", "max"];

const REFUSAL = /^\s*(n[ãa]o posso ajudar|n[ãa]o posso atender|I can(?:no|')t assist|I can(?:no|')t help|I'm sorry, but I can(?:no|')t)/i;
export function looksLikeRefusal(text: string): boolean {
	return text.length < 300 && REFUSAL.test(text);
}

// Palavras do texto do usuário que disparam recusa espúria no gpt-6-luna ("voting systems
// or elections infrastructure"), trocadas antes de enviar. Medido em 2026-11-24, 6 execuções
// cada: "evidência" recusou 6/6; "prova", "comprovação" e "evidencia" sem acento, 0/6.
// Palavras dentro de caminhos (docs/evidência.md) ficam como estão.
const PROMPT_REWRITES: { from: RegExp; to: string }[] = [{ from: /(?<![\p{L}\p{N}/._-])evidência(s?)(?![\p{L}\p{N}/_-]|\.[\p{L}\p{N}])/giu, to: "prova" }];

function matchCase(original: string, replacement: string): string {
	if (original === original.toUpperCase()) return replacement.toUpperCase();
	if (original[0] === original[0].toUpperCase()) return replacement[0].toUpperCase() + replacement.slice(1);
	return replacement;
}

/** Troca no texto do usuário as palavras que disparam recusa espúria. */
export function rewritePrompt(text: string): { text: string; count: number } {
	let count = 0;
	for (const { from, to } of PROMPT_REWRITES) {
		text = text.replace(from, (m: string, plural: string) => (count++, matchCase(m, to + (plural ? "s" : ""))));
	}
	return { text, count };
}

/** Redige segredos em cada string de um valor JSON, sem serializar. */
export function redactDeep<T>(value: T): { value: T; found: string[] } {
	const found: string[] = [];
	const walk = (v: unknown): unknown => {
		if (typeof v === "string") {
			const r = redactSecrets(v);
			found.push(...r.found);
			return r.text;
		}
		if (Array.isArray(v)) return v.map(walk);
		if (v && typeof v === "object") return Object.fromEntries(Object.entries(v).map(([k, x]) => [k, walk(x)]));
		return v;
	};
	const out = walk(value) as T;
	return { value: found.length > 0 ? out : value, found };
}

/** Tira de uma saída de grep/find/ls as linhas de arquivos negados, inclusive linhas de contexto (arquivo-12-texto). */
export function dropDeniedLines(text: string, root: string, deny: readonly string[]): string {
	const denied = findDeniedFiles(root, deny).map((f) => path.relative(root, f).split(path.sep).join("/"));
	const lines = text.split("\n");
	const kept = lines.filter((raw) => {
		const line = raw.trim().replace(/^\.\//, "");
		const file = line.split(":")[0];
		if (file && matchesAny(file, deny)) return false;
		return !denied.some((d) => line === d || line.startsWith(`${d}:`) || line.startsWith(`${d}-`));
	});
	return kept.length === lines.length ? text : kept.join("\n");
}

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
	const recovered = recoverInterrupted(root);
	const scope = opts.scope ?? resolveScope(opts.sandbox, loadProjectConfig(root));
	const commands = detectCommands(root);
	if (scope.profile.bash !== "none" && !sandboxAvailable()) {
		throw new Error(`"${scope.profile.name}" precisa do bubblewrap (bwrap) instalado para rodar comandos em sandbox`);
	}

	const runId = `${new Date().toISOString().replace(/[-:]/g, "").replace(/\..+/, "")}-${randomBytes(3).toString("hex")}`;
	const auditDir = opts.auditDir ?? auditDirFor(root);
	mkdirSync(auditDir, { recursive: true });
	const auditPath = path.join(auditDir, `${runId}.jsonl`);
	const audit = (event: string, data: Record<string, unknown>) =>
		appendFileSync(auditPath, JSON.stringify({ ts: new Date().toISOString(), runId, event, ...data }) + "\n");

	const decisions: Decision[] = [];
	const decide = (tool: string, target: string, allowed: boolean, reason?: string, by: Decision["by"] = "sandbox") => {
		const d: Decision = { tool, target, allowed, reason, by };
		decisions.push(d);
		audit("decision", { ...d });
		opts.onDecision?.(d);
		return allowed ? undefined : { block: true, reason: `Bloqueado pela Workbench: ${reason}` };
	};

	// Liberações "até o fim da execução", por arquivo exato e modo.
	const granted = { read: new Set<string>(), write: new Set<string>() };
	// Perguntas em série (aprovações e ask_user na mesma fila): chamadas paralelas do
	// modelo não abrem dois prompts juntos, e um terminal só atende uma pergunta por vez.
	let promptQueue: Promise<unknown> = Promise.resolve();
	const serial = <T>(f: () => Promise<T>): Promise<T> => {
		const next = promptQueue.then(f);
		promptQueue = next.catch(() => {});
		return next;
	};
	const askUser = (req: ApprovalRequest): Promise<ApprovalAnswer> => {
		const ask = opts.ask;
		if (!ask) return Promise.resolve("deny");
		return serial(() => ask(req)).catch(() => "deny" as const);
	};

	async function gate(tool: string, input: string, mode: "read" | "write") {
		const d = checkPath(root, input, mode, scope);
		if (d.allow) return decide(tool, input, true);
		if (d.hard || !d.rel) return decide(tool, input, false, d.reason, "rule");
		if (granted[mode].has(d.rel)) return decide(tool, input, true, "liberado pelo usuário nesta execução", "user");
		const answer = await askUser({ tool, mode, path: d.rel, reason: d.reason });
		audit("approval", { tool, mode, path: d.rel, answer });
		if (answer === "deny") return decide(tool, input, false, `${d.reason} (negado pelo usuário)`, "user");
		if (answer === "run") {
			granted[mode].add(d.rel);
			// Quem pode escrever um arquivo também pode lê-lo de volta.
			if (mode === "write") granted.read.add(d.rel);
		}
		return decide(tool, input, true, answer === "run" ? "liberado pelo usuário nesta execução" : "liberado pelo usuário uma vez", "user");
	}

	function extension(pi: ExtensionAPI) {
		const askUserFn = opts.askUser;
		if (askUserFn) {
			pi.registerTool({
				name: "ask_user",
				label: "Perguntar ao usuário",
				description: "Faz uma pergunta ao usuário e espera a resposta. Use para decisões que só o usuário pode tomar; uma pergunta por chamada.",
				parameters: Type.Object({ question: Type.String({ description: "A pergunta, curta e concreta, com uma recomendação se houver" }) }),
				async execute(_id, params) {
					const resposta = (await serial(() => askUserFn(params.question))).trim();
					audit("question", { bytes: Buffer.byteLength(resposta) });
					return { content: [{ type: "text", text: resposta || "(sem resposta: decida com uma premissa explícita)" }], details: {} };
				},
			});
		}
		pi.on("tool_call", async (event) => {
			const input = event.input as Record<string, unknown>;
			const target = String(input.path ?? input.command ?? input.question ?? ".");
			switch (event.toolName) {
				case "read":
				case "grep":
				case "find":
				case "ls":
					return gate(event.toolName, String(input.path ?? "."), "read");
				case "write":
				case "edit":
					return gate(event.toolName, String(input.path ?? ""), "write");
				case "ask_user":
					return opts.askUser ? decide("ask_user", target, true) : decide("ask_user", target, false, "sem usuário para responder", "rule");
				case "bash":
					return scope.profile.bash !== "none"
						? decide("bash", target, true, "sandbox")
						: decide("bash", target, false, `o sandbox "${scope.profile.name}" não roda comandos`, "rule");
				default:
					return decide(event.toolName, target, false, "ferramenta fora do contrato da Workbench", "rule");
			}
		});

		// Antes de o resultado entrar no transcript: tira linhas de arquivos negados
		// (grep/find/ls varrem diretórios) e redige segredos.
		pi.on("tool_result", async (event) => {
			let changed = false;
			const content = event.content.map((block) => {
				if (block.type !== "text") return block;
				let text = block.text;
				if (["grep", "find", "ls"].includes(event.toolName)) text = dropDeniedLines(text, root, scope.deny);
				const r = redactSecrets(text);
				if (r.found.length > 0) audit("redact", { tool: event.toolName, kinds: r.found });
				if (r.text === block.text) return block;
				changed = true;
				return { ...block, text: r.text };
			});
			return changed ? { content, structuredContent: undefined } : undefined;
		});

		// Auditoria de cada envio ao provedor: modelo, tamanho e hash, nunca o conteúdo.
		const send = (source: string, body: string, found: string[], model?: { provider?: string; id?: string }) =>
			audit("send", {
				source,
				provider: model?.provider ?? null,
				model: model?.id ?? null,
				bytes: Buffer.byteLength(body),
				sha256: createHash("sha256").update(body).digest("hex"),
				secretsRedacted: found,
			});

		// A redação percorre cada string (não o JSON serializado): assim os padrões com \b
		// funcionam depois de quebras de linha e uma chave nunca casa entre duas mensagens.
		pi.on("context", async (event, ctx) => {
			const r = redactDeep(event.messages);
			send("context", JSON.stringify(r.value), r.found, ctx.model);
			return r.found.length > 0 ? { messages: r.value } : undefined;
		});

		pi.on("before_provider_request", async (event, ctx) => {
			const r = redactDeep(event.payload ?? null);
			send("payload", JSON.stringify(r.value), r.found, ctx.model);
			return r.found.length > 0 ? r.value : undefined;
		});
	}

	const agentDir = opts.agentDir ?? path.join(os.homedir(), ".pi", "agent");
	const modelRuntime = opts.modelRuntime ?? (await ModelRuntime.create());
	let model = opts.modelOverride;
	if (!model) {
		const ref = opts.model ?? DEFAULT_MODEL;
		const [provider, ...rest] = ref.split("/");
		model = modelRuntime.getModel(provider, rest.join("/"));
		if (!model) throw new Error(`modelo não encontrado: ${ref}`);
	}

	const settingsManager = SettingsManager.inMemory({ compaction: { enabled: true } });
	const resourceLoader = new DefaultResourceLoader({
		cwd: root,
		agentDir,
		settingsManager,
		extensionFactories: [extension],
		noExtensions: true,
		noSkills: true,
		noPromptTemplates: true,
		noThemes: true,
		appendSystemPrompt: [describeScope(scope, commands, Boolean(opts.ask))],
	});
	await resourceLoader.reload();

	const tools = ["read", "grep", "find", "ls"];
	// Com aprovação interativa, até o sandbox read-only recebe write/edit: cada escrita vira pergunta.
	if (scope.write.length > 0 || opts.ask) tools.push("write", "edit");
	// `tools` é uma allowlist; o "bash" listado aqui é a definição sandboxed de customTools.
	if (scope.profile.bash !== "none") tools.push("bash");
	if (opts.askUser) tools.push("ask_user");
	const customTools =
		scope.profile.bash !== "none"
			? [
					createBashToolDefinition(root, {
						exposeSessionEnvironment: false,
						spawnHook: (c) => ({ ...c, command: wrapCommand(c.command, root, c.cwd, scope.deny, scope.profile.bash === "sandbox") }),
					}),
				]
			: [];

	const before = gitChanges(root);
	const guard = beginGuard(root, runId);
	for (const q of recovered) audit("violation", { ...q, action: "created in an interrupted run; moved aside" });
	const rewritten = rewritePrompt(opts.task);
	const task = rewritten.text;
	audit("start", { projectDir: root, profile: scope.profile.name, interactive: Boolean(opts.ask), model: `${model.provider}/${model.id}`, task: redactSecrets(task).text, write: scope.write, commands: commands.map((c) => c.command) });

	const { session } = await createAgentSession({
		cwd: root,
		agentDir,
		model,
		thinkingLevel: opts.thinkingLevel ?? DEFAULT_THINKING,
		modelRuntime,
		resourceLoader,
		settingsManager,
		tools,
		customTools: customTools as never,
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
		if (event.type === "message_end") {
			const m = event.message as { role?: string; provider?: string; model?: string; stopReason?: string; usage?: PiUsage };
			if (m.role !== "assistant" || !m.usage) return;
			addUsage(usage, m.usage);
			const u = m.usage;
			audit("usage", {
				model: `${m.provider}/${m.model}`,
				stopReason: m.stopReason,
				input: u.input,
				output: u.output,
				cacheRead: u.cacheRead,
				cacheWrite: u.cacheWrite,
				reasoning: u.reasoning,
				total: u.totalTokens,
				cost: u.cost?.total,
				costFromProvider: Boolean(u.cost?.totalFromProvider),
			});
		}
	});

	// Ctrl-C/SIGTERM interrompem o modelo, mas deixam o finally limpar o que o run criou.
	let interrupted = false;
	const onSignal = () => {
		interrupted = true;
		void session.abort();
	};
	process.once("SIGINT", onSignal);
	process.once("SIGTERM", onSignal);

	let finalText = "";
	let quarantined: Quarantined[] = [];
	try {
		await session.prompt(task);
		throwIfModelError(session.messages.at(-1));
		finalText = session.getLastAssistantText() ?? "";
		// Recusas espúrias acontecem de forma intermitente: uma retomada simples, sem argumentar.
		for (let retry = 0; retry < (opts.refusalRetries ?? 1) && looksLikeRefusal(finalText); retry++) {
			audit("refusal", { retry: retry + 1, text: finalText.slice(0, 200) });
			opts.onText?.("\n[o modelo recusou; pedindo para continuar de onde parou]\n");
			await session.prompt("Continue a tarefa de onde parou.");
			throwIfModelError(session.messages.at(-1));
			finalText = session.getLastAssistantText() ?? "";
		}
	} finally {
		process.off("SIGINT", onSignal);
		process.off("SIGTERM", onSignal);
		unsubscribe();
		session.dispose();
		// .workbench.json e .git criados durante o run (só o bash conseguiria) não valem.
		quarantined = endGuard(root, guard);
		for (const q of quarantined) {
			audit("violation", { ...q, action: "created during the run; moved aside" });
			opts.onText?.(`\n[workbench: ${q.path} criado durante a execução foi ignorado e movido para ${q.movedTo}]\n`);
		}
	}
	if (interrupted) {
		audit("end", { interrupted: true, usage });
		throw new Error("execução interrompida");
	}

	const after = gitChanges(root);
	const changedFiles = before && after ? [...after].filter((f) => !before.has(f)).sort() : [];
	audit("end", { changedFiles, blocked: decisions.filter((d) => !d.allowed).length, usage });

	return { runId, scope, commands, decisions, changedFiles, finalText, auditPath, usage };
}
