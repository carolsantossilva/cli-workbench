// wb hooks: instala a cópia do review antes do push, tirada da main do remoto, e aponta
// Claude Code e Codex para ela, sem mexer nos outros hooks e configurações, e diz quando
// algo está fora do lugar. O remoto é um repositório bare local: nada vai para a rede.
import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { appendFileSync, existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, renameSync, rmSync, statSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { after, test } from "node:test";
import { type HookSource, hooksStatus, installedHookPath, installHooks } from "../src/hooks.ts";

const tmp = mkdtempSync(path.join(os.tmpdir(), "wb-hooks-test-"));
after(() => rmSync(tmp, { recursive: true, force: true }));
const CLI = path.join(import.meta.dirname, "../src/cli.ts");
const REAL_HOOK = readFileSync(path.join(import.meta.dirname, "../hooks/review-before-push.py"), "utf8");
const REL = "host/hooks/review-before-push.py";

let n = 0;
function home(...agents: string[]): string {
	const h = path.join(tmp, `h${n++}`);
	for (const a of agents) mkdirSync(path.join(h, a), { recursive: true });
	return h;
}
const json = (file: string) => JSON.parse(readFileSync(file, "utf8"));
const backups = (dir: string) => readdirSync(dir).filter((f) => f.includes(".antes-wb-"));
const git = (repo: string, ...args: string[]) => execFileSync("git", ["-C", repo, "-c", "user.name=t", "-c", "user.email=t@t", ...args], { stdio: "pipe" });

/** Um remoto bare com o hook na main e um clone dele, como o checkout de onde o wb roda. */
function repo(content = REAL_HOOK): HookSource & { remote: string; commit: (text: string) => void } {
	const base = path.join(tmp, `r${n++}`);
	const remote = path.join(base, "remote.git");
	const clone = path.join(base, "clone");
	mkdirSync(base);
	git(base, "init", "-q", "--bare", "-b", "main", remote);
	git(base, "init", "-q", "-b", "main", clone);
	git(clone, "remote", "add", "origin", remote);
	const commit = (text: string) => {
		mkdirSync(path.join(clone, path.dirname(REL)), { recursive: true });
		writeFileSync(path.join(clone, REL), text);
		git(clone, "add", "-A");
		git(clone, "commit", "-qm", "hook");
		git(clone, "push", "-q", "origin", "HEAD:main");
	};
	commit(content);
	return { repo: clone, rel: REL, remote, commit };
}

/** Outra pessoa muda o hook na main, sem passar por este clone. */
function updateMainElsewhere(remote: string, text: string) {
	const other = path.join(tmp, `o${n++}`);
	git(tmp, "clone", "-q", remote, other);
	writeFileSync(path.join(other, REL), text);
	git(other, "commit", "-qam", "nova versão");
	git(other, "push", "-q", "origin", "HEAD:main");
}

test("instala a cópia da main e aponta os dois agentes para ela, preservando o resto", () => {
	const source = repo();
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

	const r = installHooks(h, { source });
	const hook = installedHookPath(h);
	assert.equal(r.hook, hook);
	assert.equal(r.from, "main");
	assert.equal(readFileSync(hook, "utf8"), REAL_HOOK);
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

	const s = hooksStatus(h, { source });
	assert.equal(s.copy, "ok");
	assert.equal(s.commit, r.commit);
	assert.equal(s.main, r.commit);
	assert.equal(s.fetched, true);
	assert.deepEqual(s.agents.map((a) => a.state), ["ativo", "ativo"]);
});

test("vale a main, não o checkout: edição local ou branch não entra", () => {
	const source = repo();
	writeFileSync(path.join(source.repo, REL), "# editado no checkout\n");
	git(source.repo, "switch", "-qc", "outra");
	git(source.repo, "commit", "-qam", "na branch");
	const h = home(".claude");
	installHooks(h, { source });
	assert.equal(readFileSync(installedHookPath(h), "utf8"), REAL_HOOK);
});

test("main nova no remoto: status avisa e install atualiza", () => {
	const source = repo();
	const h = home(".claude");
	installHooks(h, { source });
	updateMainElsewhere(source.remote, `${REAL_HOOK}# v2\n`);
	const s = hooksStatus(h, { source });
	assert.equal(s.copy, "desatualizada: a main tem outra versão", "o status busca a main antes de comparar");
	assert.notEqual(s.main, s.commit);
	installHooks(h, { source });
	assert.ok(readFileSync(installedHookPath(h), "utf8").endsWith("# v2\n"));
	assert.equal(hooksStatus(h, { source }).copy, "ok");
});

test("--local instala a versão do checkout, e o status diz que não é a da main", () => {
	const source = repo();
	writeFileSync(path.join(source.repo, REL), `${REAL_HOOK}# testando\n`);
	const h = home(".claude");
	const r = installHooks(h, { source, local: true });
	assert.equal(r.from, "local");
	assert.equal(r.dirty, true, "conteúdo não commitado fica marcado");
	assert.equal(hooksStatus(h, { source }).dirty, true);
	assert.ok(readFileSync(installedHookPath(h), "utf8").endsWith("# testando\n"));
	assert.equal(hooksStatus(h, { source }).copy, "do checkout local, não da main");
});

test("sem rede ou sem o hook na main: install recusa e indica --local; status usa a última main", () => {
	const source = repo();
	const h = home(".claude");
	installHooks(h, { source });
	renameSync(source.remote, `${source.remote}.fora`);
	assert.throws(() => installHooks(h, { source }), /não deu para buscar a main.*--local/);
	const s = hooksStatus(h, { source });
	assert.equal(s.fetched, false);
	assert.equal(s.copy, "ok", "compara com a última main buscada");
	renameSync(`${source.remote}.fora`, source.remote);

	const vazio = repo();
	git(vazio.repo, "rm", "-q", REL);
	git(vazio.repo, "commit", "-qm", "sem hook");
	git(vazio.repo, "push", "-q", "origin", "HEAD:main");
	assert.throws(() => installHooks(home(".claude"), { source: vazio }), /ainda não está na main.*--local/);
});

test("instalar de novo não duplica o hook nem cria outro backup", () => {
	const source = repo();
	const h = home(".claude");
	installHooks(h, { source });
	const before = backups(path.join(h, ".claude")).length;
	const r = installHooks(h, { source });
	assert.equal(r.agents[0].result, "já estava");
	assert.equal(backups(path.join(h, ".claude")).length, before);
	assert.equal(json(path.join(h, ".claude/settings.json")).hooks.PreToolUse.length, 1);
});

test("sem o agente instalado, não cria a pasta dele", () => {
	const source = repo();
	const h = home(".codex");
	const r = installHooks(h, { source });
	assert.deepEqual(r.agents.map((a) => a.result), ["sem o agente", "instalado"]);
	assert.ok(!existsSync(path.join(h, ".claude")));
	assert.equal(hooksStatus(h, { source }).agents[0].state, "sem o agente");
});

test("status: cópia editada, hook ausente ou apontando para outro script", () => {
	const source = repo();
	const h = home(".claude", ".codex");
	assert.equal(hooksStatus(h, { source }).copy, "ausente");
	installHooks(h, { source });
	appendFileSync(installedHookPath(h), "# editado\n");
	writeFileSync(path.join(h, ".codex/hooks.json"), "{}");
	writeFileSync(
		path.join(h, ".claude/settings.json"),
		JSON.stringify({ hooks: { PreToolUse: [{ hooks: [{ type: "command", command: "python3 /old/review-before-push.py claude" }] }] } }),
	);
	const s = hooksStatus(h, { source });
	assert.equal(s.copy, "editada depois de instalada");
	assert.deepEqual(s.agents.map((a) => a.state), ["aponta para outro script", "ausente"]);
});

test("config com JSON inválido: recusa e não mexe no arquivo", () => {
	const source = repo();
	const h = home(".claude");
	const settings = path.join(h, ".claude/settings.json");
	writeFileSync(settings, "{ quebrado");
	assert.throws(() => installHooks(h, { source }), /não é JSON válido/);
	assert.equal(readFileSync(settings, "utf8"), "{ quebrado");
});

test("wb hooks pela CLI: install --local e uso errado (sem rede)", () => {
	const h = home(".claude");
	const wb = (...args: string[]) => spawnSync(process.execPath, [CLI, ...args], { encoding: "utf8", env: { ...process.env, HOME: h, NO_COLOR: "1" } });
	const install = wb("hooks", "install", "--local");
	assert.equal(install.status, 0, install.stderr);
	assert.match(install.stdout, /\(do checkout local, [0-9a-f]{7}\)\nclaude: instalado/);
	assert.match(wb("hooks").stderr, /subcomando desconhecido "hooks "/);
	assert.match(wb("hooks", "status", "x").stderr, /argumento a mais/);
	assert.match(wb("hooks", "status", "--local").stderr, /--local não vale para "wb hooks status"/);
	assert.match(wb("hooks", "status", "-s", "docs-only").stderr, /--sandbox não vale/);
	assert.match(wb("status", "--local").stderr, /--local não vale para "wb status"/);
});

test("home com espaço: o caminho vai entre aspas e o status reconhece", () => {
	const source = repo();
	const h = path.join(tmp, "com espaço", "it's");
	mkdirSync(path.join(h, ".claude"), { recursive: true });
	installHooks(h, { source });
	const command: string = json(path.join(h, ".claude/settings.json")).hooks.PreToolUse[0].hooks[0].command;
	assert.match(command, / '.*com espaço\/it'\\''s\/\.local\/share\/workbench\/hooks\/review-before-push\.py' claude$/);
	const r = spawnSync("sh", ["-c", command], { input: JSON.stringify({ tool_input: { command: "git status" } }), encoding: "utf8" });
	assert.equal(r.status, 0, r.stderr);
	assert.equal(r.stdout, "", "comando que não é push: o hook não diz nada");
	assert.equal(hooksStatus(h, { source }).agents[0].state, "ativo");
});

test("status: instalação antiga sem registro e script removido da main", () => {
	const source = repo();
	const h = home(".claude");
	installHooks(h, { source });
	rmSync(installedHookPath(h).replace(/\.py$/, ".json"));
	assert.equal(hooksStatus(h, { source }).copy, "instalada por uma versão antiga do wb");

	installHooks(h, { source });
	git(source.repo, "rm", "-q", REL);
	git(source.repo, "commit", "-qm", "tira o hook");
	git(source.repo, "push", "-q", "origin", "HEAD:main");
	assert.equal(hooksStatus(h, { source }).copy, "o script não está mais na main");
});

test("clone de uma branch só: o fetch ainda atualiza origin/main", () => {
	const source = repo();
	const single = path.join(tmp, `s${n++}`);
	git(source.repo, "push", "-q", "origin", "HEAD:outra");
	git(tmp, "clone", "-q", "--single-branch", "-b", "outra", source.remote, single);
	updateMainElsewhere(source.remote, `${REAL_HOOK}# v3\n`);
	const h = home(".claude");
	installHooks(h, { source: { repo: single, rel: REL } });
	assert.ok(readFileSync(installedHookPath(h), "utf8").endsWith("# v3\n"));
});
