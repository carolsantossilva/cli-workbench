// O que sai das ferramentas e vai para o modelo: sem segredos e sem arquivos negados.
import path from "node:path";
import { matchesAny, redactSecrets } from "./policy.ts";
import { findDeniedFiles } from "./sandbox.ts";

/** Redige segredos em cada string de um valor JSON, sem serializar. */
export function redactDeep<T>(value: T): { value: T; found: string[] } {
	const found: string[] = [];
	const walk = (v: unknown): unknown => {
		if (typeof v === "string") {
			const r = redactSecrets(v);
			found.push(...r.found);
			return r.text;
		}
		if (Array.isArray(v)) return v.map(walk);
		if (v && typeof v === "object") return Object.fromEntries(Object.entries(v).map(([k, x]) => [k, walk(x)]));
		return v;
	};
	const out = walk(value) as T;
	return { value: found.length > 0 ? out : value, found };
}

/** Tira de uma saída de grep/find/ls as linhas de arquivos negados, inclusive linhas de contexto (arquivo-12-texto). */
export function dropDeniedLines(text: string, root: string, deny: readonly string[]): string {
	const denied = findDeniedFiles(root, deny).map((f) => path.relative(root, f).split(path.sep).join("/"));
	const lines = text.split("\n");
	const kept = lines.filter((raw) => {
		const line = raw.trim().replace(/^\.\//, "");
		const file = line.split(":")[0];
		if (file && matchesAny(file, deny)) return false;
		return !denied.some((d) => line === d || line.startsWith(`${d}:`) || line.startsWith(`${d}-`));
	});
	return kept.length === lines.length ? text : kept.join("\n");
}
