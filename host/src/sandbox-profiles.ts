// Os sandboxes prontos (perfis de permissão): cada execução escolhe um, e o projeto só
// declara exceções no .workbench.json (project-config.ts). O sandbox.ts aplica isso no bwrap.
// Nomes seguem o --sandbox do Codex CLI onde há equivalente (read-only, workspace-write).
export type SandboxName = "read-only" | "docs-only" | "workspace-write";

export interface SandboxProfile {
	/** Um sandbox pronto ou "step <etapa>". */
	name: string;
	description: string;
	read: string[];
	write: string[];
	/** none: sem bash. sandbox: bash livre no bwrap, sem rede e sem a home. sandbox-ro: idem, com o projeto só leitura. */
	bash: "none" | "sandbox" | "sandbox-ro";
}

// Nunca lidos nem escritos, em qualquer sandbox.
export const ALWAYS_DENY = [
	"**/.env",
	"**/.env.*",
	"**/*.pem",
	"**/*.key",
	"**/id_rsa*",
	"**/id_ed25519*",
	"**/.npmrc",
	"**/.netrc",
	"**/auth.json",
	"**/credentials*.json",
	// Estado e auditoria do wb: o agente não precisa, e trechos de runs anteriores não devem voltar ao modelo.
	"**/.workbench/**",
];

// Podem ser lidos, nunca escritos pelo agente.
export const WRITE_DENY = ["**/.git/**", ".git", "**/.workbench/**", ".workbench.json"];

const BUILTIN: Record<SandboxName, SandboxProfile> = {
	"read-only": {
		name: "read-only",
		description: "Lê o projeto e responde. Não escreve nada e não roda comandos.",
		read: ["**"],
		write: [],
		bash: "none",
	},
	"docs-only": {
		name: "docs-only",
		description: "Lê o projeto e escreve só documentação (Markdown e docs/). Não roda comandos.",
		read: ["**"],
		write: ["**/*.md", "docs/**"],
		bash: "none",
	},
	"workspace-write": {
		name: "workspace-write",
		description:
			"Lê e escreve código no projeto. Pode rodar comandos num sandbox sem rede, que só enxerga o projeto e as ferramentas instaladas.",
		read: ["**"],
		write: ["**"],
		bash: "sandbox",
	},
};

export const SANDBOX_NAMES = Object.keys(BUILTIN) as SandboxName[];

/** Ajustes do projeto aos sandboxes prontos (o .workbench.json os lê em project-config.ts). */
export interface SandboxOverrides {
	/** Sandbox padrão do `wb run` neste projeto. */
	sandbox?: SandboxName;
	/** Globs negados além dos sempre negados. */
	deny?: string[];
	/** Globs graváveis a mais, por sandbox. */
	write?: Partial<Record<SandboxName, string[]>>;
	/** Opt-in explícito para a ferramenta de consulta da wiki do kb-ai. */
	kbAi?: { wiki?: boolean };
}

export interface EffectiveScope {
	profile: SandboxProfile;
	read: string[];
	write: string[];
	deny: string[];
	writeDeny: string[];
	/** Ferramenta wiki: consulta só leitura da wiki do kb-ai, fora da raiz do projeto. */
	wiki: boolean;
}

export function resolveScope(name: SandboxName | undefined, cfg: SandboxOverrides): EffectiveScope {
	const chosen = name ?? cfg.sandbox ?? "read-only";
	const profile = BUILTIN[chosen];
	if (!profile) throw new Error(`sandbox desconhecido "${chosen}" (use ${SANDBOX_NAMES.join(", ")})`);
	return {
		profile,
		read: profile.read,
		write: [...profile.write, ...(cfg.write?.[chosen] ?? [])],
		deny: [...ALWAYS_DENY, ...(cfg.deny ?? [])],
		writeDeny: WRITE_DENY,
		wiki: cfg.kbAi?.wiki === true,
	};
}
