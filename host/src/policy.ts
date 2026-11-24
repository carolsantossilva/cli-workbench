// Checagens puras de caminho e redação de segredos (base: spikes/pi-host/policy.ts).
import { existsSync, realpathSync } from "node:fs";
import path from "node:path";
import type { EffectiveScope } from "./profiles.ts";

// hard: nunca pode ser liberado (sensível, fora da raiz, config/.git).
// soft: só o perfil não cobre; pode virar pergunta ao usuário.
export type Decision = { allow: true; rel: string } | { allow: false; reason: string; hard: boolean; rel?: string };

// Resolve um caminho vindo de uma ferramenta para um caminho relativo à raiz,
// seguindo symlinks do ancestral existente mais próximo.
export function resolveInRoot(root: string, input: string): Decision {
	const realRoot = realpathSync(root);
	const p = input.startsWith("@") ? input.slice(1) : input;
	if (p.startsWith("~")) return { allow: false, hard: true, reason: `caminho na home não é permitido: ${input}` };
	const abs = path.resolve(realRoot, p);
	let probe = abs;
	const tail: string[] = [];
	while (!existsSync(probe) && probe !== path.dirname(probe)) {
		tail.unshift(path.basename(probe));
		probe = path.dirname(probe);
	}
	const real = path.join(realpathSync(probe), ...tail);
	const rel = path.relative(realRoot, real);
	if (rel.startsWith("..") || path.isAbsolute(rel)) return { allow: false, hard: true, reason: `fora da raiz do projeto: ${input}` };
	return { allow: true, rel: rel.split(path.sep).join("/") || "." };
}

export function matchesAny(rel: string, globs: readonly string[]): boolean {
	return globs.some((g) => g === "**" || rel === g || path.posix.matchesGlob(rel, g));
}

export function checkPath(root: string, input: string, mode: "read" | "write", scope: EffectiveScope): Decision {
	const r = resolveInRoot(root, input);
	if (!r.allow) return r;
	if (matchesAny(r.rel, scope.deny)) return { allow: false, hard: true, reason: `arquivo sensível, sempre negado: ${r.rel}` };
	if (mode === "write") {
		if (matchesAny(r.rel, scope.writeDeny)) return { allow: false, hard: true, reason: `escrita proibida: ${r.rel}` };
		return matchesAny(r.rel, scope.write)
			? r
			: { allow: false, hard: false, rel: r.rel, reason: `o perfil "${scope.profile.name}" não permite escrever em ${r.rel}` };
	}
	if (matchesAny(r.rel, [...scope.read, ...scope.write])) return r;
	return { allow: false, hard: false, rel: r.rel, reason: `o perfil "${scope.profile.name}" não permite ler ${r.rel}` };
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
