// Hooks que o wb instala na máquina para os agentes usados fora dele (Claude Code e Codex).
// Hoje: o review antes do push. Os agentes rodam uma cópia local do script, tirada da main
// do remoto: o que vale é o que passou por PR, não o checkout (de qualquer branch). A cópia
// só muda com um novo "wb hooks install"; "wb hooks status" avisa quando a main mudou.
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { existsSync, mkdirSync, readFileSync, renameSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

export const REVIEW_HOOK = "review-before-push.py";
const STATUS_MESSAGE = "Revisando o commit antes do push";
const MAIN = "refs/remotes/origin/main";

type AgentName = "claude" | "codex";
interface Agent {
	name: AgentName;
	dir: string;
	config: string;
}

interface HookCommand {
	type?: string;
	command?: string;
	[k: string]: unknown;
}
interface HookGroup {
	matcher?: string;
	hooks?: HookCommand[];
	[k: string]: unknown;
}
interface HookConfig {
	hooks?: { PreToolUse?: HookGroup[]; [k: string]: unknown };
	[k: string]: unknown;
}

/** Onde o script vive no git: o repo do wb e o caminho dentro dele. */
export interface HookSource {
	repo: string;
	rel: string;
}

/** O que foi instalado: de onde veio, de qual commit e o hash, para notar edições na cópia. */
interface Installed {
	from: "main" | "local";
	commit: string;
	/** --local com mudanças não commitadas: o conteúdo não é o do commit. */
	dirty?: boolean;
	sha256: string;
}

// Sem prompt de credencial: um remoto que pede senha falha na hora em vez de esperar o timeout.
const GIT_ENV = { ...process.env, GIT_TERMINAL_PROMPT: "0" };

function git(repo: string, ...args: string[]): string {
	return execFileSync("git", ["-C", repo, ...args], { encoding: "utf8", stdio: ["ignore", "pipe", "pipe"], timeout: 60_000, env: GIT_ENV }).trim();
}

/** O repo de onde este wb roda. */
export function defaultSource(): HookSource {
	const file = fileURLToPath(new URL(`../hooks/${REVIEW_HOOK}`, import.meta.url));
	const repo = git(path.dirname(file), "rev-parse", "--show-toplevel");
	return { repo, rel: path.relative(repo, file).split(path.sep).join("/") };
}

function fetchMain(repo: string): boolean {
	try {
		// Refspec explícito: atualiza origin/main mesmo num clone de uma branch só.
		git(repo, "fetch", "-q", "origin", `+refs/heads/main:${MAIN}`);
		return true;
	} catch {
		return false;
	}
}

/** O script na última main buscada; undefined se ele ainda não está lá. */
function mainVersion({ repo, rel }: HookSource): { commit: string; content: Buffer } | undefined {
	let commit: string;
	try {
		commit = git(repo, "rev-parse", "--verify", `${MAIN}^{commit}`);
	} catch {
		return undefined;
	}
	try {
		const content = execFileSync("git", ["-C", repo, "show", `${commit}:${rel}`], { stdio: ["ignore", "pipe", "pipe"], timeout: 60_000 });
		return { commit, content };
	} catch {
		return undefined;
	}
}

export function installedHookPath(home = os.homedir()): string {
	return path.join(home, ".local/share/workbench/hooks", REVIEW_HOOK);
}
const metaPath = (home: string) => installedHookPath(home).replace(/\.py$/, ".json");

function readInstalled(home: string): Installed | undefined {
	try {
		return JSON.parse(readFileSync(metaPath(home), "utf8")) as Installed;
	} catch {
		return undefined;
	}
}

function agents(home: string): Agent[] {
	return [
		{ name: "claude", dir: path.join(home, ".claude"), config: path.join(home, ".claude/settings.json") },
		{ name: "codex", dir: path.join(home, ".codex"), config: path.join(home, ".codex/hooks.json") },
	];
}

function python(): string {
	return existsSync("/usr/bin/python3") ? "/usr/bin/python3" : "python3";
}

/** Aspas simples quando o caminho tem espaço ou outro caractere do shell. */
const shellQuote = (s: string) => (/^[\w@%+=:,./-]+$/.test(s) ? s : `'${s.replaceAll("'", `'\\''`)}'`);

/** O comando que cada agente roda: o caminho da cópia sempre entre aspas se preciso. */
export function hookCommand(home: string, agent: AgentName): string {
	return `${python()} ${shellQuote(installedHookPath(home))} ${agent}`;
}

const sha256 = (data: Buffer | string) => createHash("sha256").update(data).digest("hex");

/** Algum hook de review antes do push, deste wb ou de uma instalação antiga. */
const isReviewHook = (h: HookCommand) => typeof h.command === "string" && h.command.includes(REVIEW_HOOK);

/** A config do agente; `raw` evita ler de novo um arquivo já lido. */
function readConfig(file: string, raw = existsSync(file) ? readFileSync(file, "utf8") : undefined): HookConfig {
	if (raw === undefined || raw.trim() === "") return {};
	try {
		return JSON.parse(raw) as HookConfig;
	} catch (e) {
		throw new Error(`${file} não é JSON válido; corrija antes de instalar (${(e as Error).message})`);
	}
}

/** O PreToolUse com um único hook de review: o deste wb. Os outros hooks ficam como estão. */
function withReviewHook(cfg: HookConfig, command: string): HookConfig {
	const groups = (cfg.hooks?.PreToolUse ?? [])
		.map((g) => ({ ...g, hooks: (g.hooks ?? []).filter((h) => !isReviewHook(h)) }))
		.filter((g) => g.hooks.length > 0);
	groups.push({ matcher: "^Bash$", hooks: [{ type: "command", command, timeout: 900, statusMessage: STATUS_MESSAGE }] });
	return { ...cfg, hooks: { ...cfg.hooks, PreToolUse: groups } };
}

function writeAtomic(file: string, data: string | Buffer, mode: number) {
	const tmp = `${file}.wb-${process.pid}.tmp`;
	writeFileSync(tmp, data, { mode });
	renameSync(tmp, file);
}

export interface InstallOptions {
	/** Instala a versão do checkout, não a da main (para testar uma mudança no hook). */
	local?: boolean;
	source?: HookSource;
}

type AgentInstallResult = "instalado" | "já estava" | "sem o agente";

export interface InstallResult {
	hook: string;
	from: "main" | "local";
	commit: string;
	dirty?: boolean;
	agents: { name: AgentName; config: string; result: AgentInstallResult; backup?: string }[];
}

function scriptToInstall(source: HookSource, local: boolean): { content: Buffer; from: Installed["from"]; commit: string; dirty?: boolean } {
	if (local) {
		const dirty = git(source.repo, "status", "--porcelain", "--", source.rel) !== "";
		return { content: readFileSync(path.join(source.repo, source.rel)), from: "local", commit: git(source.repo, "rev-parse", "HEAD"), dirty };
	}
	if (!fetchMain(source.repo)) throw new Error(`não deu para buscar a main de origin em ${source.repo}; sem rede, use "wb hooks install --local"`);
	const main = mainVersion(source);
	if (!main) throw new Error(`${source.rel} ainda não está na main de origin; para testar antes do merge, use "wb hooks install --local"`);
	return { ...main, from: "main" };
}

export function installHooks(home = os.homedir(), { local = false, source = defaultSource() }: InstallOptions = {}): InstallResult {
	const script = scriptToInstall(source, local);
	const hook = installedHookPath(home);
	mkdirSync(path.dirname(hook), { recursive: true, mode: 0o700 });
	writeAtomic(hook, script.content, 0o700);
	const installed: Installed = { from: script.from, commit: script.commit, dirty: script.dirty, sha256: sha256(script.content) };
	writeAtomic(metaPath(home), `${JSON.stringify(installed, null, 2)}\n`, 0o600);

	const result: InstallResult = { hook, from: script.from, commit: script.commit, dirty: script.dirty, agents: [] };
	for (const a of agents(home)) result.agents.push({ name: a.name, config: a.config, ...configureAgent(a, hookCommand(home, a.name)) });
	return result;
}

/** Aponta o PreToolUse do agente para a cópia; backup da config anterior quando ela muda. */
function configureAgent(agent: Agent, command: string): { result: AgentInstallResult; backup?: string } {
	if (!existsSync(agent.dir)) return { result: "sem o agente" };
	const before = existsSync(agent.config) ? readFileSync(agent.config, "utf8") : undefined;
	const current = readConfig(agent.config, before);
	const cfg = withReviewHook(current, command);
	if (before !== undefined && JSON.stringify(current) === JSON.stringify(cfg)) return { result: "já estava" };
	let backup: string | undefined;
	if (before !== undefined) {
		backup = `${agent.config}.antes-wb-${new Date().toISOString().replace(/[:.]/g, "-")}`;
		writeFileSync(backup, before, { mode: 0o600 });
	}
	writeAtomic(agent.config, `${JSON.stringify(cfg, null, 2)}\n`, 0o600);
	return { result: "instalado", backup };
}

export type CopyState =
	| "ok"
	| "ausente"
	| "instalada por uma versão antiga do wb"
	| "editada depois de instalada"
	| "desatualizada: a main tem outra versão"
	| "o script não está mais na main"
	| "do checkout local, não da main";

export interface HookStatus {
	hook: string;
	copy: CopyState;
	/** Commit de onde a cópia veio. */
	commit?: string;
	dirty?: boolean;
	/** Commit da main usada na comparação. */
	main?: string;
	/** false: sem acesso ao remoto, comparado com a última main buscada. */
	fetched: boolean;
	agents: { name: AgentName; state: "ativo" | "ausente" | "aponta para outro script" | "sem o agente"; commands: string[] }[];
}

type AgentState = HookStatus["agents"][number]["state"];

export function hooksStatus(home = os.homedir(), { source = defaultSource() }: { source?: HookSource } = {}): HookStatus {
	const hook = installedHookPath(home);
	const installed = readInstalled(home);
	const fetched = fetchMain(source.repo);
	const main = mainVersion(source);
	return {
		hook,
		copy: copyState(hook, installed, main?.content),
		commit: installed?.commit,
		dirty: installed?.dirty,
		main: main?.commit,
		fetched,
		agents: agents(home).map((a) => ({ name: a.name, ...agentStatus(a, hookCommand(home, a.name)) })),
	};
}

/** Compara a cópia instalada com o que foi registrado na instalação e com a main. */
function copyState(hook: string, installed: Installed | undefined, mainContent: Buffer | undefined): CopyState {
	if (!existsSync(hook)) return "ausente";
	if (!installed) return "instalada por uma versão antiga do wb";
	if (sha256(readFileSync(hook)) !== installed.sha256) return "editada depois de instalada";
	if (installed.from === "local") return "do checkout local, não da main";
	if (!mainContent) return "o script não está mais na main";
	if (sha256(mainContent) !== installed.sha256) return "desatualizada: a main tem outra versão";
	return "ok";
}

/** Os hooks de review na config do agente e se todos apontam para a cópia. */
function agentStatus(agent: Agent, command: string): { state: AgentState; commands: string[] } {
	if (!existsSync(agent.dir)) return { state: "sem o agente", commands: [] };
	const commands = (readConfig(agent.config).hooks?.PreToolUse ?? [])
		.flatMap((g) => g.hooks ?? [])
		.filter(isReviewHook)
		.map((h) => String(h.command));
	if (commands.length === 0) return { state: "ausente", commands };
	return { state: commands.every((c) => c === command) ? "ativo" : "aponta para outro script", commands };
}
