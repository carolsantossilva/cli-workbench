// Detecta os comandos do projeto em vez de exigir que sejam declarados.
import { existsSync, readFileSync } from "node:fs";
import path from "node:path";

export interface DetectedCommand {
	command: string;
	source: string;
}

const NPM_SCRIPTS = ["test", "lint", "typecheck", "check", "build"];

export function detectCommands(root: string): DetectedCommand[] {
	const out: DetectedCommand[] = [];
	const pkg = path.join(root, "package.json");
	if (existsSync(pkg)) {
		try {
			const scripts = (JSON.parse(readFileSync(pkg, "utf8")).scripts ?? {}) as Record<string, string>;
			for (const s of NPM_SCRIPTS) if (scripts[s]) out.push({ command: s === "test" ? "npm test" : `npm run ${s}`, source: "package.json" });
		} catch {
			// package.json inválido: sem comandos detectados.
		}
	}
	if (existsSync(path.join(root, "go.mod"))) {
		out.push({ command: "go build ./...", source: "go.mod" }, { command: "go vet ./...", source: "go.mod" }, { command: "go test ./...", source: "go.mod" });
	}
	if (existsSync(path.join(root, "Cargo.toml"))) {
		out.push({ command: "cargo test", source: "Cargo.toml" });
	}
	return out;
}
