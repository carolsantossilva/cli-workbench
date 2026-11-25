// Checagens puras de caminho e redação de segredos (base: spikes/pi-host/policy.ts).
import { lstatSync, realpathSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import type { EffectiveScope } from "./sandbox-profiles.ts";

// hard: nunca pode ser liberado (sensível, fora da raiz, config/.git).
// soft: só o perfil não cobre; pode virar pergunta ao usuário.
export type Decision = { allow: true; rel: string } | { allow: false; reason: string; hard: boolean; rel?: string };

/**
 * A raiz do projeto não pode ser a home nem um diretório acima dela: o sandbox esconde a
 * home, mas monta a raiz de volta, e com ela ~/.ssh, ~/.aws e ~/.config ficariam visíveis.
 */
export function assertProjectRoot(root: string): void {
	const realRoot = realpathSync(root);
	let home: string;
	try {
		home = realpathSync(os.homedir());
	} catch {
		return;
	}
	const rel = path.relative(realRoot, home);
	if (rel === "" || (!rel.startsWith("..") && !path.isAbsolute(rel))) {
		throw new Error(`o projeto não pode ser a home nem um diretório que a contém (${realRoot}); use a pasta do projeto`);
	}
}

// Resolve um caminho vindo de uma ferramenta para um caminho relativo à raiz,
// seguindo symlinks do ancestral existente mais próximo.
export function resolveInRoot(root: string, input: string): Decision {
	const realRoot = realpathSync(root);
	const p = input.startsWith("@") ? input.slice(1) : input;
	if (p.startsWith("~")) return { allow: false, hard: true, reason: `caminho na home não é permitido: ${input}` };
	const abs = path.resolve(realRoot, p);
	// lstat (não existsSync): um symlink quebrado "existe" e precisa ser resolvido, senão
	// uma escrita seguiria o link para fora da raiz ou para .workbench/.git.
	const present = (p: string) => {
		try {
			lstatSync(p);
			return true;
		} catch {
			return false;
		}
	};
	let probe = abs;
	const tail: string[] = [];
	while (!present(probe) && probe !== path.dirname(probe)) {
		tail.unshift(path.basename(probe));
		probe = path.dirname(probe);
	}
	let resolved: string;
	try {
		resolved = realpathSync(probe);
	} catch {
		return { allow: false, hard: true, reason: `symlink quebrado não é permitido: ${input}` };
	}
	const real = path.join(resolved, ...tail);
	const rel = path.relative(realRoot, real);
	if (rel.startsWith("..") || path.isAbsolute(rel)) return { allow: false, hard: true, reason: `fora da raiz do projeto: ${input}` };
	return { allow: true, rel: rel.split(path.sep).join("/") || "." };
}

// Glob → RegExp com semântica "dot: true": `*` e `**` também casam nomes que começam
// com ponto. (path.matchesGlob do Node não tem essa opção e ignora .config/, .ssh/ etc.)
const globCache = new Map<string, RegExp>();
export function globToRegExp(glob: string): RegExp {
	let re = globCache.get(glob);
	if (re) return re;
	let src = "";
	for (let i = 0; i < glob.length; i++) {
		const c = glob[i];
		if (c === "*" && glob[i + 1] === "*") {
			const atStart = i === 0 || glob[i - 1] === "/";
			const slashAfter = glob[i + 2] === "/";
			if (atStart && slashAfter) {
				src += "(?:.*/)?";
				i += 2;
			} else if (atStart && i + 2 === glob.length && i > 0) {
				src = src.slice(0, -1) + "(?:/.*)?";
				i += 1;
			} else {
				src += ".*";
				i += 1;
			}
		} else if (c === "*") src += "[^/]*";
		else if (c === "?") src += "[^/]";
		else src += c.replace(/[.+^${}()|[\]\\]/g, "\\$&");
	}
	re = new RegExp(`^${src}$`);
	globCache.set(glob, re);
	return re;
}

export function matchesAny(rel: string, globs: readonly string[]): boolean {
	return globs.some((g) => rel === g || globToRegExp(g).test(rel));
}

export function checkPath(root: string, input: string, mode: "read" | "write", scope: EffectiveScope): Decision {
	const r = resolveInRoot(root, input);
	if (!r.allow) return r;
	if (matchesAny(r.rel, scope.deny)) return { allow: false, hard: true, reason: `arquivo sensível, sempre negado: ${r.rel}` };
	if (mode === "write") {
		if (matchesAny(r.rel, scope.writeDeny)) return { allow: false, hard: true, reason: `escrita proibida: ${r.rel}` };
		return matchesAny(r.rel, scope.write)
			? r
			: { allow: false, hard: false, rel: r.rel, reason: `o sandbox "${scope.profile.name}" não permite escrever em ${r.rel}` };
	}
	if (matchesAny(r.rel, [...scope.read, ...scope.write])) return r;
	return { allow: false, hard: false, rel: r.rel, reason: `o sandbox "${scope.profile.name}" não permite ler ${r.rel}` };
}

const SECRET_PATTERNS: readonly [string, RegExp][] = [
	["private-key", /-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----/g],
	["anthropic-key", /\bsk-ant-[A-Za-z0-9_-]{20,}/g],
	["openai-key", /\bsk-(?:proj-)?[A-Za-z0-9_-]{20,}/g],
	["github-token", /\bgh[pousr]_[A-Za-z0-9]{30,}/g],
	["github-pat", /\bgithub_pat_[A-Za-z0-9_]{40,}/g],
	["aws-access-key", /\bAKIA[0-9A-Z]{16}\b/g],
	["google-api-key", /\bAIza[0-9A-Za-z_-]{35}\b/g],
	["slack-token", /\bxox[abposr]-[A-Za-z0-9-]{10,}/g],
];

export function redactSecrets(text: string): { text: string; found: string[] } {
	const found: string[] = [];
	let out = text;
	for (const [kind, re] of SECRET_PATTERNS) {
		out = out.replace(re, () => {
			found.push(kind);
			return `[REDACTED:${kind}]`;
		});
	}
	return { text: out, found };
}
