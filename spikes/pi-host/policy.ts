// Pure policy checks. The Go core owns the contract; these only apply the
// already-resolved run context (readPaths/writePaths/allowedCommands) at call time.
import { existsSync, realpathSync } from "node:fs";
import path from "node:path";

export type Decision = { allow: true; rel: string } | { allow: false; reason: string };

// Resolve a tool-supplied path to a root-relative POSIX path, following symlinks
// of the nearest existing ancestor so a link cannot smuggle a write out of root.
export function resolveInRoot(root: string, input: string): Decision {
	const realRoot = realpathSync(root);
	let p = input.startsWith("@") ? input.slice(1) : input;
	if (p.startsWith("~")) return { allow: false, reason: `home-relative path not allowed: ${input}` };
	const abs = path.resolve(realRoot, p);
	let probe = abs;
	const tail: string[] = [];
	while (!existsSync(probe) && probe !== path.dirname(probe)) {
		tail.unshift(path.basename(probe));
		probe = path.dirname(probe);
	}
	const real = path.join(realpathSync(probe), ...tail);
	const rel = path.relative(realRoot, real);
	if (rel.startsWith("..") || path.isAbsolute(rel)) {
		return { allow: false, reason: `outside workspace root: ${input}` };
	}
	return { allow: true, rel: rel.split(path.sep).join("/") || "." };
}

export function matchesAny(rel: string, globs: readonly string[]): boolean {
	return globs.some((g) => rel === g || path.posix.matchesGlob(rel, g));
}

// A directory may be listed when it is a prefix of some declared glob, so the
// agent can navigate to its inputs; only entry names are exposed that way.
export function isAncestorOfAny(rel: string, globs: readonly string[]): boolean {
	if (rel === ".") return globs.length > 0;
	return globs.some((g) => g.startsWith(rel + "/"));
}

export function checkPath(
	root: string,
	input: string,
	mode: "read" | "write" | "list",
	scope: { readPaths: readonly string[]; writePaths: readonly string[] },
): Decision {
	const r = resolveInRoot(root, input);
	if (!r.allow) return r;
	if (mode === "write") {
		return matchesAny(r.rel, scope.writePaths)
			? r
			: { allow: false, reason: `write outside writePaths: ${r.rel}` };
	}
	// Anything the workflow may write it may also read back.
	const readable = [...scope.readPaths, ...scope.writePaths];
	if (matchesAny(r.rel, readable)) return r;
	if (mode === "list" && isAncestorOfAny(r.rel, readable)) return r;
	return { allow: false, reason: `${mode} outside readPaths: ${r.rel}` };
}

const SECRET_PATTERNS: readonly [string, RegExp][] = [
	["private-key", /-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----/g],
	["anthropic-key", /\bsk-ant-[A-Za-z0-9_-]{20,}/g],
	["openai-key", /\bsk-(?:proj-)?[A-Za-z0-9_-]{20,}/g],
	["github-token", /\bgh[pousr]_[A-Za-z0-9]{30,}/g],
	["aws-access-key", /\bAKIA[0-9A-Z]{16}\b/g],
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
