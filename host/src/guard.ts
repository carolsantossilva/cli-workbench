// Guarda dos caminhos que o bash não pode criar. Os que já existem são montados só
// leitura no sandbox (sandbox.ts); os que ainda não existem (.workbench.json e
// qualquer .git) não têm como ser montados, então o host tira uma foto antes do run
// e põe de lado o que aparecer. A foto fica em .workbench/ (só leitura para o bash),
// para que um run interrompido (erro, Ctrl-C, kill) seja tratado no próximo uso.
import { existsSync, mkdirSync, readdirSync, readFileSync, renameSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";

const MARKER = path.join(".workbench", "execucao-ativa.json");
const CONFIG = ".workbench.json";

interface Snapshot {
	runId: string;
	configExisted: boolean;
	gits: string[];
}

export interface Quarantined {
	path: string;
	movedTo: string;
}

/** Todos os .git (pasta ou arquivo) do projeto, relativos à raiz. */
export function findGitEntries(root: string, maxDepth = 12): string[] {
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
			if (e.name === ".git") out.push(r);
			else if (e.isDirectory() && e.name !== "node_modules") walk(path.join(dir, e.name), r, depth + 1);
		}
	};
	walk(root, "", 0);
	return out;
}

export function beginGuard(root: string, runId: string): Snapshot {
	const snap: Snapshot = { runId, configExisted: existsSync(path.join(root, CONFIG)), gits: findGitEntries(root) };
	mkdirSync(path.join(root, ".workbench"), { recursive: true });
	writeFileSync(path.join(root, MARKER), JSON.stringify(snap) + "\n");
	return snap;
}

/** Põe de lado o que o run criou nos caminhos protegidos e apaga a foto. */
export function endGuard(root: string, snap: Snapshot): Quarantined[] {
	const moved: Quarantined[] = [];
	const aside = (rel: string) => {
		const to = `${rel}.rejeitado-${snap.runId}`;
		renameSync(path.join(root, rel), path.join(root, to));
		moved.push({ path: rel, movedTo: to });
	};
	if (!snap.configExisted && existsSync(path.join(root, CONFIG))) aside(CONFIG);
	const known = new Set(snap.gits);
	for (const g of findGitEntries(root)) if (!known.has(g)) aside(g);
	rmSync(path.join(root, MARKER), { force: true });
	return moved;
}

/** Se um run anterior foi interrompido, faz agora a limpeza que ficou pendente. */
export function recoverInterrupted(root: string): Quarantined[] {
	const marker = path.join(root, MARKER);
	if (!existsSync(marker)) return [];
	let snap: Snapshot;
	try {
		snap = JSON.parse(readFileSync(marker, "utf8")) as Snapshot;
	} catch {
		rmSync(marker, { force: true });
		return [];
	}
	return endGuard(root, snap);
}
