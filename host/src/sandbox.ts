// Sandbox do bash via bubblewrap: sem rede, home escondida, só o projeto gravável,
// arquivos sensíveis do projeto mascarados e ambiente limpo.
import { existsSync, readdirSync, realpathSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { matchesAny } from "./policy.ts";

// Ferramentas instaladas na home que os comandos do projeto costumam precisar (só leitura).
const HOME_TOOLCHAINS = [".nvm", ".volta", ".bun", "go", ".cargo", ".rustup", ".local/share/pnpm"];
const SKIP_DIRS = new Set(["node_modules", ".git", ".venv", "vendor", "dist", "build", ".next"]);

export function sandboxAvailable(): boolean {
	return ["/usr/bin/bwrap", "/bin/bwrap", "/usr/local/bin/bwrap"].some((p) => existsSync(p));
}

export function quote(s: string): string {
	return `'${s.replace(/'/g, `'\\''`)}'`;
}

/** Arquivos do projeto que casam com a lista de negação (até 6 níveis, pulando dependências). */
export function findDeniedFiles(root: string, deny: readonly string[], maxDepth = 6): string[] {
	const out: string[] = [];
	const walk = (dir: string, rel: string, depth: number) => {
		if (depth > maxDepth) return;
		let entries;
		try {
			entries = readdirSync(dir, { withFileTypes: true });
		} catch {
			return;
		}
		for (const e of entries) {
			const r = rel ? `${rel}/${e.name}` : e.name;
			if (e.isDirectory()) {
				if (!SKIP_DIRS.has(e.name)) walk(path.join(dir, e.name), r, depth + 1);
			} else if (matchesAny(r, deny)) {
				out.push(path.join(dir, e.name));
			}
		}
	};
	walk(root, "", 0);
	return out;
}

export function buildSandboxArgs(root: string, cwd: string, deny: readonly string[], writable = true): string[] {
	const realRoot = realpathSync(root);
	const home = os.homedir();
	const args = [
		"--ro-bind", "/", "/",
		"--dev", "/dev",
		"--proc", "/proc",
		"--tmpfs", "/tmp",
		"--tmpfs", "/run",
		"--tmpfs", home,
	];
	for (const t of HOME_TOOLCHAINS) {
		const p = path.join(home, t);
		if (existsSync(p)) args.push("--ro-bind", p, p);
	}
	args.push(writable ? "--bind" : "--ro-bind", realRoot, realRoot);
	for (const f of findDeniedFiles(realRoot, deny)) args.push("--ro-bind", "/dev/null", f);
	args.push(
		"--unshare-net",
		"--unshare-pid",
		"--die-with-parent",
		"--new-session",
		"--clearenv",
		"--setenv", "PATH", process.env.PATH ?? "/usr/bin:/bin",
		"--setenv", "HOME", home,
		"--setenv", "LANG", process.env.LANG ?? "C.UTF-8",
		"--setenv", "TERM", "dumb",
		"--setenv", "GOCACHE", "/tmp/go-build",
		"--setenv", "GOPROXY", "off",
		"--setenv", "npm_config_cache", "/tmp/npm-cache",
		"--chdir", cwd,
	);
	return args;
}

/** Reescreve um comando para rodar dentro do sandbox. */
export function wrapCommand(command: string, root: string, cwd: string, deny: readonly string[], writable = true): string {
	const args = buildSandboxArgs(root, cwd, deny, writable).map(quote).join(" ");
	return `exec bwrap ${args} -- /bin/bash -c ${quote(command)}`;
}
