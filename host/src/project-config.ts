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
	const legacy = ["perfil", "negar", "perfis"].filter((k) => k in cfg);
	if (legacy.length > 0) throw new Error(`.workbench.json: chaves antigas ${legacy.join(", ")}; use sandbox, deny e write`);
	if (cfg.sandbox && !SANDBOX_NAMES.includes(cfg.sandbox)) {
		throw new Error(`.workbench.json: sandbox desconhecido "${cfg.sandbox}" (use ${SANDBOX_NAMES.join(", ")})`);
	}
	return cfg;
}

