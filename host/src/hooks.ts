// Hooks que o wb instala na máquina para os agentes usados fora dele (Claude Code e Codex).
// Hoje: o review antes do push. O script fica numa cópia fora do repo: editar o checkout
// (de qualquer branch) não muda a guarda que está valendo; só um novo "wb hooks install".
import { createHash } from "node:crypto";
import { chmodSync, copyFileSync, existsSync, mkdirSync, readFileSync, renameSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

export const REVIEW_HOOK = "review-before-push.py";
const SOURCE = fileURLToPath(new URL(`../hooks/${REVIEW_HOOK}`, import.meta.url));
const STATUS_MESSAGE = "Revisando o commit antes do push";

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

export function installedHookPath(home = os.homedir()): string {
	return path.join(home, ".local/share/workbench/hooks", REVIEW_HOOK);
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

const sha256 = (file: string) => createHash("sha256").update(readFileSync(file)).digest("hex");

/** Algum hook de review antes do push, deste wb ou de uma instalação antiga. */
const isReviewHook = (h: HookCommand) => typeof h.command === "string" && h.command.includes(REVIEW_HOOK);

function readConfig(file: string): HookConfig {
	if (!existsSync(file)) return {};
	const raw = readFileSync(file, "utf8");
	if (raw.trim() === "") return {};
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

function writeAtomic(file: string, text: string, mode?: number) {
	const tmp = `${file}.wb-${process.pid}.tmp`;
	writeFileSync(tmp, text, { mode });
	renameSync(tmp, file);
}

export interface InstallResult {
	hook: string;
	agents: { name: AgentName; config: string; result: "instalado" | "já estava" | "sem o agente"; backup?: string }[];
}

export function installHooks(home = os.homedir()): InstallResult {
	const hook = installedHookPath(home);
	mkdirSync(path.dirname(hook), { recursive: true, mode: 0o700 });
	const tmp = `${hook}.wb-${process.pid}.tmp`;
	copyFileSync(SOURCE, tmp);
	chmodSync(tmp, 0o700);
	renameSync(tmp, hook);

	const result: InstallResult = { hook, agents: [] };
	for (const a of agents(home)) {
		if (!existsSync(a.dir)) {
			result.agents.push({ name: a.name, config: a.config, result: "sem o agente" });
			continue;
		}
		const before = existsSync(a.config) ? readFileSync(a.config, "utf8") : undefined;
		const cfg = withReviewHook(readConfig(a.config), `${python()} ${hook} ${a.name}`);
		const after = `${JSON.stringify(cfg, null, 2)}\n`;
		if (before !== undefined && JSON.stringify(JSON.parse(before.trim() || "{}")) === JSON.stringify(cfg)) {
			result.agents.push({ name: a.name, config: a.config, result: "já estava" });
			continue;
		}
		let backup: string | undefined;
		if (before !== undefined) {
			backup = `${a.config}.antes-wb-${new Date().toISOString().replace(/[:.]/g, "-")}`;
			writeFileSync(backup, before, { mode: 0o600 });
		}
		writeAtomic(a.config, after, 0o600);
		result.agents.push({ name: a.name, config: a.config, result: "instalado", backup });
	}
	return result;
}

export interface HookStatus {
	hook: string;
	copy: "ok" | "ausente" | "diferente desta versão do wb";
	agents: { name: AgentName; state: "ativo" | "ausente" | "aponta para outro script" | "sem o agente"; commands: string[] }[];
}

export function hooksStatus(home = os.homedir()): HookStatus {
	const hook = installedHookPath(home);
	const copy = !existsSync(hook) ? "ausente" : sha256(hook) === sha256(SOURCE) ? "ok" : "diferente desta versão do wb";
	return {
		hook,
		copy,
		agents: agents(home).map((a) => {
			if (!existsSync(a.dir)) return { name: a.name, state: "sem o agente", commands: [] };
			const commands = (readConfig(a.config).hooks?.PreToolUse ?? [])
				.flatMap((g) => g.hooks ?? [])
				.filter(isReviewHook)
				.map((h) => String(h.command));
			const ours = commands.filter((c) => c.split(" ").includes(hook) && c.endsWith(` ${a.name}`));
			const state = commands.length === 0 ? "ausente" : ours.length === commands.length ? "ativo" : "aponta para outro script";
			return { name: a.name, state, commands };
		}),
	};
}
