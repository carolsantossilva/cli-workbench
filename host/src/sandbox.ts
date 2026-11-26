// Sandbox do bash via bubblewrap: sem rede, home escondida, só o projeto gravável,
// arquivos sensíveis do projeto mascarados e ambiente limpo.
import { existsSync, readdirSync, realpathSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { workbenchDir } from "./workdir.ts";
import { assertProjectRoot, matchesAny } from "./policy.ts";
import { ALWAYS_DENY } from "./sandbox-profiles.ts";

// Ferramentas instaladas na home que os comandos do projeto costumam precisar (só leitura).
const HOME_TOOLCHAINS = [".nvm", ".volta", ".bun", "go", ".cargo", ".rustup", ".local/share/pnpm"];
// Credenciais que ficam dentro dessas pastas (ex.: ~/.cargo/credentials.toml do `cargo login`),
// mascaradas com /dev/null. A busca vai só até 3 níveis: as toolchains têm milhares de arquivos.
const TOOLCHAIN_DENY = [...ALWAYS_DENY, "**/credentials", "**/credentials.toml", "**/credentials*"];
// Só dependências instaladas e o próprio .git ficam de fora da busca de arquivos sensíveis.
const SKIP_DIRS = new Set(["node_modules", ".git"]);
// Caminhos que o bash nunca pode alterar: com eles, um comando no sandbox poderia
// plantar hooks/fsmonitor que o git do usuário (ou do host) executa fora do sandbox,
// mudar a configuração da Workbench ou forjar a aprovação do design.
export const PROTECTED = [".git", ".workbench", ".workbench.json"];

export function sandboxAvailable(): boolean {
	return ["/usr/bin/bwrap", "/bin/bwrap", "/usr/local/bin/bwrap"].some((p) => existsSync(p));
}

export function quote(s: string): string {
	return `'${s.replace(/'/g, `'\\''`)}'`;
}

/** Arquivos do projeto que casam com a lista de negação (até 12 níveis, pulando node_modules e .git). */
export function findDeniedFiles(root: string, deny: readonly string[], maxDepth = 12): string[] {
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
			if (e.name === ".workbench" && e.isDirectory() && matchesAny(`${r}/x`, deny)) {
				// Estado e auditoria do wb: a pasta inteira entra na lista (mascarada de uma vez no
				// sandbox), sem descer nela; um item por arquivo cresceria com cada run.
				out.push(path.join(dir, e.name));
			} else if (e.name === ".git") {
				// .git aninhado, pasta ou arquivo (submódulo/worktree: "gitdir: ..."), entra na
				// lista para ser montado só leitura; o da raiz é tratado em PROTECTED.
				if (rel) out.push(path.join(dir, e.name));
			} else if (e.isDirectory()) {
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
	assertProjectRoot(root);
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
		if (!existsSync(p)) continue;
		args.push("--ro-bind", p, p);
		for (const f of findDeniedFiles(p, TOOLCHAIN_DENY, 3)) args.push("--ro-bind", "/dev/null", f);
	}
	args.push(writable ? "--bind" : "--ro-bind", realRoot, realRoot);
	if (writable) {
		// .workbench precisa existir para ser montado só leitura (senão o bash poderia criá-lo).
		workbenchDir(realRoot);
		for (const p of PROTECTED) {
			const abs = path.join(realRoot, p);
			if (existsSync(abs)) args.push("--ro-bind", abs, abs);
		}
	}
	for (const f of findDeniedFiles(realRoot, deny)) {
		// .git aninhados (submódulos, repositórios dentro do projeto) entram como só leitura.
		if (path.basename(f) === ".git") {
			if (writable) args.push("--ro-bind", f, f);
		} else if (path.basename(f) === ".workbench") {
			// Some com o conteúdo: no lugar, uma pasta vazia e só leitura.
			args.push("--tmpfs", f, "--remount-ro", f);
		} else args.push("--ro-bind", "/dev/null", f);
	}
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
