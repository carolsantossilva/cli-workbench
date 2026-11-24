// Detecta os comandos do projeto em vez de exigir que sejam declarados.
// Olha a raiz e as subpastas até MAX_DEPTH níveis, para cobrir monorepos
// (ex.: apps/mobile/package.json, apps/server/go.mod).
import { existsSync, readdirSync, readFileSync } from "node:fs";
import path from "node:path";

export interface DetectedCommand {
	/** Pronto para o bash, que roda na raiz: em subpasta, vem com "cd <pasta> && ". */
	command: string;
	/** Arquivo que originou o comando, relativo à raiz (ex.: "apps/mobile/package.json"). */
	source: string;
}

const NPM_SCRIPTS = ["test", "lint", "typecheck", "check", "build"];
const MAX_DEPTH = 2;
const SKIP = new Set(["node_modules", "vendor", "dist", "build", "target", "coverage"]);
// Nomes que entram no comando sugerido e no system prompt: só os seguros para o shell, sem aspas.
const SAFE_NAME = /^[A-Za-z0-9._-]+$/;

function commandsIn(dir: string, rel: string): DetectedCommand[] {
	const out: DetectedCommand[] = [];
	const prefix = rel ? `cd ${rel} && ` : "";
	const src = (f: string) => (rel ? `${rel}/${f}` : f);
	const pkg = path.join(dir, "package.json");
	if (existsSync(pkg)) {
		try {
			const scripts = (JSON.parse(readFileSync(pkg, "utf8")).scripts ?? {}) as Record<string, string>;
			for (const s of NPM_SCRIPTS) if (scripts[s]) out.push({ command: prefix + (s === "test" ? "npm test" : `npm run ${s}`), source: src("package.json") });
		} catch {
			// package.json inválido: sem comandos detectados.
		}
	}
	if (existsSync(path.join(dir, "go.mod"))) {
		for (const c of ["go build ./...", "go vet ./...", "go test ./..."]) out.push({ command: prefix + c, source: src("go.mod") });
	}
	if (existsSync(path.join(dir, "Cargo.toml"))) {
		out.push({ command: `${prefix}cargo test`, source: src("Cargo.toml") });
	}
	return out;
}

export function detectCommands(root: string): DetectedCommand[] {
	const out: DetectedCommand[] = [];
	const walk = (dir: string, rel: string, depth: number) => {
		out.push(...commandsIn(dir, rel));
		if (depth >= MAX_DEPTH) return;
		let entries;
		try {
			entries = readdirSync(dir, { withFileTypes: true });
		} catch {
			return;
		}
		// Só pastas de verdade (symlinks não), em ordem estável.
		for (const e of entries.filter((e) => e.isDirectory() && !e.name.startsWith(".") && !SKIP.has(e.name) && SAFE_NAME.test(e.name)).sort((a, b) => a.name.localeCompare(b.name))) {
			walk(path.join(dir, e.name), rel ? `${rel}/${e.name}` : e.name, depth + 1);
		}
	};
	walk(root, "", 0);
	return out;
}
