// Guarda dos caminhos que o bash não pode criar. Os que já existem são montados só
// leitura no sandbox (sandbox.ts); os que ainda não existem (.workbench.json e
// qualquer .git) não têm como ser montados, então o host tira uma foto antes do run
// e põe de lado o que aparecer. A foto fica em .workbench/ (só leitura para o bash),
// para que um run interrompido (erro, Ctrl-C, kill) seja tratado no próximo uso.
import { existsSync, readdirSync, readFileSync, renameSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";
import { workbenchDir } from "./workdir.ts";

// Uma foto por run, para que dois runs simultâneos no mesmo projeto não apaguem a do outro.
const markerPath = (root: string, runId: string) => path.join(root, ".workbench", `run-${runId}.json`);
const CONFIG = ".workbench.json";

interface Snapshot {
	runId: string;
	pid: number;
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
	const snap: Snapshot = { runId, pid: process.pid, configExisted: existsSync(path.join(root, CONFIG)), gits: findGitEntries(root) };
	workbenchDir(root);
	writeFileSync(markerPath(root, runId), JSON.stringify(snap) + "\n");
	return snap;
}

const SAFE_ID = /^[\w-]+$/;

/** Põe de lado o que o run criou nos caminhos protegidos e apaga a foto (a lida, se veio de recoverInterrupted). */
export function endGuard(root: string, snap: Snapshot, marker = markerPath(root, snap.runId)): Quarantined[] {
	const moved: Quarantined[] = [];
	const id = SAFE_ID.test(snap.runId) ? snap.runId : "desconhecido";
	const aside = (rel: string) => {
		let to = `${rel}.rejected-${id}`;
		for (let n = 2; existsSync(path.join(root, to)); n++) to = `${rel}.rejected-${id}-${n}`;
		renameSync(path.join(root, rel), path.join(root, to));
		moved.push({ path: rel, movedTo: to });
	};
	if (!snap.configExisted && existsSync(path.join(root, CONFIG))) aside(CONFIG);
	const known = new Set(snap.gits);
	for (const g of findGitEntries(root)) if (!known.has(g)) aside(g);
	rmSync(marker, { force: true });
	return moved;
}

function alive(pid: number): boolean {
	try {
		process.kill(pid, 0);
		return true;
	} catch (err) {
		return (err as NodeJS.ErrnoException).code === "EPERM";
	}
}

/**
 * Se um run anterior morreu sem limpar (kill -9, queda), faz agora a limpeza pendente.
 * Um run ainda vivo (outro terminal) não é tocado. O que for movido é avisado, e dá
 * para desfazer renomeando de volta, caso tenha sido criado por você depois da queda.
 */
export function recoverInterrupted(root: string): Quarantined[] {
	const dir = path.join(root, ".workbench");
	if (!existsSync(dir)) return [];
	const moved: Quarantined[] = [];
	for (const name of readdirSync(dir).filter((n) => /^(run|execucao)-.+\.json$/.test(n))) {
		const marker = path.join(dir, name);
		let snap: Snapshot;
		try {
			snap = JSON.parse(readFileSync(marker, "utf8")) as Snapshot;
			if (typeof snap.runId !== "string" || !Array.isArray(snap.gits)) throw new Error("foto inválida");
		} catch {
			rmSync(marker, { force: true });
			continue;
		}
		if (snap.pid && snap.pid !== process.pid && alive(snap.pid)) continue;
		// Apaga o arquivo efetivamente lido (inclui os execucao-*.json de versões anteriores).
		for (const q of endGuard(root, snap, marker)) {
			moved.push(q);
			process.stderr.write(`[workbench] o run ${snap.runId} foi interrompido; ${q.path} apareceu depois do início dele e foi movido para ${q.movedTo}. Se foi você que criou, renomeie de volta.\n`);
		}
	}
	return moved;
}
