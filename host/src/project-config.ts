// .workbench.json do projeto: só exceções aos sandboxes prontos (sandbox-profiles.ts).
import { existsSync, readFileSync } from "node:fs";
import path from "node:path";
import { recoverInterrupted } from "./guard.ts";
import { SANDBOX_NAMES, type SandboxOverrides } from "./sandbox-profiles.ts";

/** `.workbench.json` opcional na raiz do projeto: só exceções aos sandboxes. */
export type ProjectConfig = SandboxOverrides;

export function loadProjectConfig(root: string): ProjectConfig {
	recoverInterrupted(root);
	const file = path.join(root, ".workbench.json");
	if (!existsSync(file)) return {};
	const cfg = JSON.parse(readFileSync(file, "utf8")) as ProjectConfig;
	if (!cfg || typeof cfg !== "object" || Array.isArray(cfg)) throw new Error(".workbench.json: configuração deve ser um objeto");
	const legacy = ["perfil", "negar", "perfis"].filter((k) => k in cfg);
	if (legacy.length > 0) throw new Error(`.workbench.json: chaves antigas ${legacy.join(", ")}; use sandbox, deny e write`);
	if ("kbAi" in cfg) {
		const kb = cfg.kbAi;
		if (!kb || typeof kb !== "object" || Array.isArray(kb)) throw new Error('.workbench.json: kbAi deve ser um objeto com a chave "wiki"');
		const unknown = Object.keys(kb).filter((k) => k !== "wiki");
		if (unknown.length) throw new Error(`.workbench.json: chaves desconhecidas em kbAi: ${unknown.join(", ")}`);
		if (kb.wiki !== undefined && typeof kb.wiki !== "boolean") throw new Error(".workbench.json: kbAi.wiki deve ser true ou false");
	}
	if (cfg.sandbox && !SANDBOX_NAMES.includes(cfg.sandbox)) {
		throw new Error(`.workbench.json: sandbox desconhecido "${cfg.sandbox}" (use ${SANDBOX_NAMES.join(", ")})`);
	}
	return cfg;
}

