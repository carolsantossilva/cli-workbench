import { existsSync, readFileSync, readdirSync, realpathSync, statSync } from "node:fs";
import os from "node:os";
import path from "node:path";

export interface WikiRecord { path: string; title: string; description: string; quality: string }
export interface WikiOptions { env?: NodeJS.ProcessEnv; home?: string }

export function resolveWikiDir(options: WikiOptions = {}): string {
	const env = options.env ?? process.env;
	const home = options.home ?? os.homedir();
	let configured = env.WB_WIKI_DIR;
	if (configured === undefined) {
		const file = path.join(home, ".config/workbench/config.json");
		if (existsSync(file)) {
			const config = JSON.parse(readFileSync(file, "utf8")) as { kbAi?: unknown };
			if (config.kbAi !== undefined && (!config.kbAi || typeof config.kbAi !== "object" || Array.isArray(config.kbAi))) throw new Error("configuração kbAi inválida");
			const kbAi = config.kbAi as { wikiDir?: unknown } | undefined;
			if (kbAi?.wikiDir !== undefined) {
				if (typeof kbAi.wikiDir !== "string" || !kbAi.wikiDir) throw new Error("configuração kbAi.wikiDir inválida");
				configured = kbAi.wikiDir;
			}
		}
	}
	if (configured !== undefined && !configured) throw new Error("configuração WB_WIKI_DIR inválida: valor vazio");
	const selected = configured === undefined ? path.join(home, "developer-carol/kb-ai/wiki") : configured.replace(/^~(?=\/|$)/, home);
	let real: string;
	try { real = realpathSync(selected); } catch { throw new Error(`wiki indisponível: diretório não encontrado: ${selected}`); }
	if (!statSync(real).isDirectory()) throw new Error(`wiki indisponível: não é um diretório: ${selected}`);
	if (path.basename(real) !== "wiki") throw new Error(`wiki indisponível: configure o diretório wiki/, não o clone: ${selected}`);
	return real;
}

function pageFiles(root: string): string[] {
	const result: string[] = [];
	const visited = new Set<string>();
	const visit = (dir: string) => {
		let canonical: string;
		try { canonical = realpathSync(dir); } catch { return; }
		if (!inside(root, canonical) || visited.has(canonical)) return;
		visited.add(canonical);
		for (const name of readdirSync(canonical).sort()) {
			const file = path.join(canonical, name);
			let real: string;
			let st;
			try { real = realpathSync(file); st = statSync(real); } catch { continue; }
			if (!inside(root, real)) continue;
			if (st.isDirectory()) { visit(real); continue; }
			if (name.endsWith(".md") && st.isFile()) result.push(real);
		}
	};
	visit(root);
	return [...new Set(result)].sort((a, b) => relative(root, a).localeCompare(relative(root, b)));
}
function inside(root: string, file: string) { return file === root || file.startsWith(root + path.sep); }
function relative(root: string, file: string) { return path.relative(root, file).split(path.sep).join("/"); }

function validPage(root: string, requested: string): string {
	if (!requested || path.isAbsolute(requested) || requested.split(/[\\/]/).includes("..")) throw new Error("página indisponível");
	const candidate = path.resolve(root, requested);
	try {
		const real = realpathSync(candidate);
		if (!inside(root, real) || !real.endsWith(".md") || !statSync(real).isFile()) throw new Error();
		return real;
	} catch { throw new Error("página indisponível"); }
}

function metadata(content: string) {
	const match = content.match(/^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)/);
	const values: Record<string, string> = {};
	if (!match) return { values, body: content };
	for (const line of match[1].split(/\r?\n/)) {
		const pair = line.match(/^\s*(title|description|status|stale_after)\s*:\s*(.*?)\s*$/);
		if (!pair) continue;
		let value = pair[2].trim();
		const flow = value.match(/^\{\s*(?:en|value)\s*:\s*(['"]?)(.*?)\1\s*\}$/);
		if (flow) value = flow[2];
		else value = value.replace(/^(['"])(.*)\1$/, "$2");
		values[pair[1]] = value;
	}
	return { values, body: content.slice(match[0].length) };
}

export function wikiSearch(root: string, query: string, now = new Date()): WikiRecord[] {
	const canonical = realpathSync(root);
	const needle = query.toLocaleLowerCase();
	return pageFiles(canonical).flatMap((file) => {
		let text: string;
		try { text = readFileSync(file, "utf8"); } catch { return []; }
		if (!text.toLocaleLowerCase().includes(needle)) return [];
		const { values } = metadata(text);
		const quality: string[] = [];
		if ("stale_after" in values) {
			const parsed = Date.parse(values.stale_after);
			const valid = /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$/.test(values.stale_after) && Number.isFinite(parsed) && new Date(parsed).toISOString().replace(".000Z", "Z") === values.stale_after;
			if (!valid) quality.push("stale_after inválido");
			else if (parsed < now.getTime()) quality.push(`stale (venceu ${values.stale_after.slice(0, 10)})`);
		}
		if (values.status?.toLowerCase() === "draft") quality.push("draft");
		if (/⚠️\s*Divergência/i.test(text)) quality.push("divergência");
		return [{ path: relative(canonical, file), title: values.title || "(ausente)", description: values.description || "(ausente)", quality: quality.join(", ") || "ok" }];
	});
}

export function showWikiPage(root: string, requested: string): string {
	const canonical = realpathSync(root);
	return readFileSync(validPage(canonical, requested), "utf8");
}

export function wikiIndexes(root: string): { path: string; content: string }[] {
	const canonical = realpathSync(root);
	const indexes = pageFiles(canonical).filter((f) => path.basename(f) === "index.md");
	// O índice raiz é o ponto de entrada: vem antes dos índices das pastas.
	indexes.sort((a, b) => Number(relative(canonical, b) === "index.md") - Number(relative(canonical, a) === "index.md"));
	return indexes.map((f) => ({ path: relative(canonical, f), content: readFileSync(f, "utf8") }));
}

export function formatWikiRecord(record: WikiRecord): string {
	return `path: ${record.path}\ntitle: ${record.title}\ndescription: ${record.description}\nquality: ${record.quality}`;
}
