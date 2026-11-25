// wb hooks: instala a cópia do review antes do push e aponta Claude Code e Codex para ela,
// sem mexer nos outros hooks e configurações, e diz quando algo está fora do lugar.
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { appendFileSync, existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { after, test } from "node:test";
import { hooksStatus, installedHookPath, installHooks } from "../src/hooks.ts";

const tmp = mkdtempSync(path.join(os.tmpdir(), "wb-hooks-test-"));
after(() => rmSync(tmp, { recursive: true, force: true }));
const CLI = path.join(import.meta.dirname, "../src/cli.ts");

let n = 0;
function home(...agents: string[]): string {
	const h = path.join(tmp, `h${n++}`);
	for (const a of agents) mkdirSync(path.join(h, a), { recursive: true });
	return h;
}
const json = (file: string) => JSON.parse(readFileSync(file, "utf8"));
const backups = (dir: string) => readdirSync(dir).filter((f) => f.includes(".antes-wb-"));

test("instala a cópia e aponta os dois agentes para ela, preservando o resto", () => {
	const h = home(".claude", ".codex");
	const settings = path.join(h, ".claude/settings.json");
	writeFileSync(
		settings,
		JSON.stringify({
			model: "opus",
			hooks: {
				PreToolUse: [
					{ matcher: "^Bash$", hooks: [{ type: "command", command: "/usr/bin/python3 /old/agent-hooks/review-before-push.py claude", timeout: 900 }] },
					{ matcher: "^Edit$", hooks: [{ type: "command", command: "outro-hook" }] },
				],
				Stop: [{ hooks: [{ type: "command", command: "aviso" }] }],
			},
		}),
	);

	const r = installHooks(h);
	const hook = installedHookPath(h);
	assert.equal(r.hook, hook);
	assert.equal(statSync(hook).mode & 0o777, 0o700);
	assert.deepEqual(r.agents.map((a) => [a.name, a.result]), [["claude", "instalado"], ["codex", "instalado"]]);
	assert.ok(r.agents[0].backup && existsSync(r.agents[0].backup), "backup do settings.json que existia");
	assert.equal(r.agents[1].backup, undefined, "o hooks.json do Codex não existia: sem backup");

	const cfg = json(settings);
	assert.equal(cfg.model, "opus");
	assert.deepEqual(cfg.hooks.Stop, [{ hooks: [{ type: "command", command: "aviso" }] }]);
	const commands = cfg.hooks.PreToolUse.flatMap((g: { hooks: { command: string }[] }) => g.hooks.map((x) => x.command));
	assert.ok(commands.includes("outro-hook"), "outros hooks ficam");
	assert.ok(!commands.some((c: string) => c.includes("/old/")), "o review antigo sai");
	assert.equal(commands.filter((c: string) => c.endsWith(`${hook} claude`)).length, 1);
	assert.ok(json(path.join(h, ".codex/hooks.json")).hooks.PreToolUse[0].hooks[0].command.endsWith(`${hook} codex`));

	assert.deepEqual(hooksStatus(h), {
		hook,
		copy: "ok",
		agents: [
			{ name: "claude", state: "ativo", commands: [commands.find((c: string) => c.endsWith("claude"))] },
			{ name: "codex", state: "ativo", commands: [json(path.join(h, ".codex/hooks.json")).hooks.PreToolUse[0].hooks[0].command] },
		],
	});
});

test("instalar de novo não duplica o hook nem cria outro backup", () => {
	const h = home(".claude");
	installHooks(h);
	writeFileSync(path.join(h, ".claude/x"), "");
	const before = backups(path.join(h, ".claude")).length;
	const r = installHooks(h);
	assert.equal(r.agents[0].result, "já estava");
	assert.equal(backups(path.join(h, ".claude")).length, before);
	assert.equal(json(path.join(h, ".claude/settings.json")).hooks.PreToolUse.length, 1);
});

test("sem o agente instalado, não cria a pasta dele", () => {
	const h = home(".codex");
	const r = installHooks(h);
	assert.deepEqual(r.agents.map((a) => a.result), ["sem o agente", "instalado"]);
	assert.ok(!existsSync(path.join(h, ".claude")));
	assert.equal(hooksStatus(h).agents[0].state, "sem o agente");
});

test("status: cópia alterada, hook ausente ou apontando para outro script", () => {
	const h = home(".claude", ".codex");
	assert.equal(hooksStatus(h).copy, "ausente");
	installHooks(h);
	appendFileSync(installedHookPath(h), "# editado\n");
	writeFileSync(path.join(h, ".codex/hooks.json"), "{}");
	writeFileSync(
		path.join(h, ".claude/settings.json"),
		JSON.stringify({ hooks: { PreToolUse: [{ hooks: [{ type: "command", command: "python3 /old/review-before-push.py claude" }] }] } }),
	);
	const s = hooksStatus(h);
	assert.equal(s.copy, "diferente desta versão do wb");
	assert.deepEqual(s.agents.map((a) => a.state), ["aponta para outro script", "ausente"]);
});

test("config com JSON inválido: recusa e não mexe no arquivo", () => {
	const h = home(".claude");
	const settings = path.join(h, ".claude/settings.json");
	writeFileSync(settings, "{ quebrado");
	assert.throws(() => installHooks(h), /não é JSON válido/);
	assert.equal(readFileSync(settings, "utf8"), "{ quebrado");
});

test("wb hooks pela CLI: install, status e uso errado", () => {
	const h = home(".claude");
	const wb = (...args: string[]) => spawnSync(process.execPath, [CLI, ...args], { encoding: "utf8", env: { ...process.env, HOME: h, NO_COLOR: "1" } });
	const install = wb("hooks", "install");
	assert.equal(install.status, 0, install.stderr);
	assert.match(install.stdout, /claude: instalado/);
	const status = wb("hooks", "status");
	assert.match(status.stdout, /\(ok\)\nclaude: ativo\ncodex: sem o agente/);
	assert.match(wb("hooks").stderr, /subcomando desconhecido "hooks "/);
	assert.match(wb("hooks", "status", "x").stderr, /argumento a mais/);
	assert.match(wb("hooks", "status", "-s", "docs-only").stderr, /--sandbox não vale/);
});

test("home com espaço: o caminho vai entre aspas e o status reconhece", () => {
	const h = path.join(tmp, "com espaço", "it's");
	mkdirSync(path.join(h, ".claude"), { recursive: true });
	installHooks(h);
	const command: string = json(path.join(h, ".claude/settings.json")).hooks.PreToolUse[0].hooks[0].command;
	assert.match(command, / '.*com espaço\/it'\\''s\/\.local\/share\/workbench\/hooks\/review-before-push\.py' claude$/);
	const r = spawnSync("sh", ["-c", `${command.replace(/ claude$/, "")} claude`], { input: JSON.stringify({ tool_input: { command: "git status" } }), encoding: "utf8" });
	assert.equal(r.status, 0, r.stderr);
	assert.equal(r.stdout, "", "comando que não é push: o hook não diz nada");
	assert.equal(hooksStatus(h).agents[0].state, "ativo");
});
