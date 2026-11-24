// Host da Workbench sobre o SDK do Pi: aplica o perfil antes de cada ação,
// roda o bash no sandbox, redige segredos e audita o que vai para o modelo.
import { execFileSync } from "node:child_process";
import { createHash, randomBytes } from "node:crypto";
import { appendFileSync, mkdirSync, realpathSync } from "node:fs";
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
import { type DetectedCommand, detectCommands } from "./detect.ts";
import { checkPath, matchesAny, redactSecrets } from "./policy.ts";
import { type EffectiveScope, loadProjectConfig, type ProfileName, resolveScope } from "./profiles.ts";
import { sandboxAvailable, wrapCommand } from "./sandbox.ts";

export interface Decision {
	tool: string;
	target: string;
	allowed: boolean;
	reason?: string;
}

export interface RunOptions {
	projectDir: string;
	task: string;
	profile?: ProfileName;
	/** "provider/modelo", ex.: openai-codex/gpt-6-luna */
	model?: string;
	/** Para testes: runtime e modelo já prontos (ex.: provider faux). */
	modelRuntime?: ModelRuntime;
	modelOverride?: Model<Api>;
	auditDir?: string;
	agentDir?: string;
	onText?: (delta: string) => void;
	onDecision?: (d: Decision) => void;
}

export interface RunResult {
	runId: string;
	scope: EffectiveScope;
	commands: DetectedCommand[];
	decisions: Decision[];
	changedFiles: string[];
	finalText: string;
	auditPath: string;
}

export const DEFAULT_MODEL = "openai-codex/gpt-6-luna";

export function defaultAuditDir(): string {
	return path.join(process.env.XDG_STATE_HOME ?? path.join(os.homedir(), ".local", "state"), "workbench", "runs");
}

function gitChanges(root: string): Set<string> | undefined {
	try {
		const out = execFileSync("git", ["status", "--porcelain=v1", "-z", "--untracked-files=all"], { cwd: root, encoding: "utf8" });
		return new Set(out.split("\0").filter(Boolean).map((l) => l.slice(3)));
	} catch {
		return undefined;
	}
}

export function describeScope(scope: EffectiveScope, commands: DetectedCommand[]): string {
	const p = scope.profile;
	return [
		`# Workbench: perfil "${p.name}"`,
		p.description,
		`Leitura: ${scope.read.join(", ")}`,
		`Escrita: ${scope.write.join(", ") || "nenhuma"}`,
		`Sempre negado: ${scope.deny.join(", ")}`,
		p.bash === "sandbox"
			? `Comandos: bash disponível num sandbox SEM rede (não dá para instalar dependências nem acessar a internet). Comandos detectados no projeto: ${commands.map((c) => c.command).join(", ") || "nenhum"}.`
			: "Comandos: nenhum. O bash não está disponível neste perfil.",
		"Chamadas fora desse escopo são bloqueadas pelo host. Não tente contornar o bloqueio: se precisar de algo bloqueado, diga no resumo final o que faltou e por quê.",
		"Ao terminar, escreva um resumo curto em português: o que fez, arquivos alterados e o que ficou pendente.",
	].join("\n");
}

export async function runTask(opts: RunOptions): Promise<RunResult> {
	const root = realpathSync(opts.projectDir);
	const scope = resolveScope(opts.profile, loadProjectConfig(root));
	const commands = detectCommands(root);
	if (scope.profile.bash === "sandbox" && !sandboxAvailable()) {
		throw new Error('o perfil "codigo" precisa do bubblewrap (bwrap) instalado para rodar comandos em sandbox');
	}

	const runId = `${new Date().toISOString().replace(/[-:]/g, "").replace(/\..+/, "")}-${randomBytes(3).toString("hex")}`;
	const auditDir = opts.auditDir ?? defaultAuditDir();
	mkdirSync(auditDir, { recursive: true });
	const auditPath = path.join(auditDir, `${runId}.jsonl`);
	const audit = (event: string, data: Record<string, unknown>) =>
		appendFileSync(auditPath, JSON.stringify({ ts: new Date().toISOString(), runId, event, ...data }) + "\n");

	const decisions: Decision[] = [];
	const decide = (tool: string, target: string, allowed: boolean, reason?: string) => {
		const d = { tool, target, allowed, reason };
		decisions.push(d);
		audit("decision", { ...d });
		opts.onDecision?.(d);
		return allowed ? undefined : { block: true, reason: `Bloqueado pela Workbench: ${reason}` };
	};

	function extension(pi: ExtensionAPI) {
		pi.on("tool_call", async (event) => {
			const input = event.input as Record<string, unknown>;
			const target = String(input.path ?? input.command ?? ".");
			switch (event.toolName) {
				case "read":
				case "grep":
				case "find":
				case "ls": {
					const d = checkPath(root, String(input.path ?? "."), "read", scope);
					return decide(event.toolName, target, d.allow, d.allow ? undefined : d.reason);
				}
				case "write":
				case "edit": {
					const d = checkPath(root, String(input.path ?? ""), "write", scope);
					return decide(event.toolName, target, d.allow, d.allow ? undefined : d.reason);
				}
				case "bash":
					return scope.profile.bash === "sandbox"
						? decide("bash", target, true, "sandbox")
						: decide("bash", target, false, `o perfil "${scope.profile.name}" não roda comandos`);
				default:
					return decide(event.toolName, target, false, "ferramenta fora do contrato da Workbench");
			}
		});

		// Antes de o resultado entrar no transcript: tira linhas de arquivos negados
		// (grep/find/ls varrem diretórios) e redige segredos.
		pi.on("tool_result", async (event) => {
			let changed = false;
			const content = event.content.map((block) => {
				if (block.type !== "text") return block;
				let text = block.text;
				if (["grep", "find", "ls"].includes(event.toolName)) {
					const kept = text.split("\n").filter((line) => {
						const file = line.split(":")[0].trim().replace(/^\.\//, "");
						return !(file && matchesAny(file, scope.deny));
					});
					if (kept.length !== text.split("\n").length) text = kept.join("\n");
				}
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

		pi.on("context", async (event, ctx) => {
			const body = JSON.stringify(event.messages);
			const r = redactSecrets(body);
			send("context", body, r.found, ctx.model);
			return r.found.length > 0 ? { messages: JSON.parse(r.text) } : undefined;
		});

		pi.on("before_provider_request", async (event, ctx) => {
			const body = JSON.stringify(event.payload ?? null);
			const r = redactSecrets(body);
			send("payload", body, r.found, ctx.model);
			return r.found.length > 0 ? JSON.parse(r.text) : undefined;
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
		appendSystemPrompt: [describeScope(scope, commands)],
	});
	await resourceLoader.reload();

	const tools = ["read", "grep", "find", "ls"];
	if (scope.write.length > 0) tools.push("write", "edit");
	// `tools` é uma allowlist; o "bash" listado aqui é a definição sandboxed de customTools.
	if (scope.profile.bash === "sandbox") tools.push("bash");
	const customTools =
		scope.profile.bash === "sandbox"
			? [
					createBashToolDefinition(root, {
						exposeSessionEnvironment: false,
						spawnHook: (c) => ({ ...c, command: wrapCommand(c.command, root, c.cwd, scope.deny) }),
					}),
				]
			: [];

	const before = gitChanges(root);
	audit("start", { projectDir: root, profile: scope.profile.name, model: `${model.provider}/${model.id}`, task: opts.task, write: scope.write, commands: commands.map((c) => c.command) });

	const { session } = await createAgentSession({
		cwd: root,
		agentDir,
		model,
		modelRuntime,
		resourceLoader,
		settingsManager,
		tools,
		customTools: customTools as never,
		sessionManager: SessionManager.inMemory(root),
	});
	await session.bindExtensions?.({});
	const unsubscribe = session.subscribe((event) => {
		if (event.type === "message_update" && event.assistantMessageEvent.type === "text_delta") opts.onText?.(event.assistantMessageEvent.delta);
	});

	let finalText = "";
	try {
		await session.prompt(opts.task);
		finalText = session.getLastAssistantText() ?? "";
	} finally {
		unsubscribe();
		session.dispose();
	}

	const after = gitChanges(root);
	const changedFiles = before && after ? [...after].filter((f) => !before.has(f)).sort() : [];
	audit("end", { changedFiles, blocked: decisions.filter((d) => !d.allowed).length });

	return { runId, scope, commands, decisions, changedFiles, finalText, auditPath };
}
