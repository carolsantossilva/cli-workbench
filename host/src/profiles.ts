// Perfis de permissão: cada execução escolhe um, e o projeto só declara exceções.
import { existsSync, readFileSync } from "node:fs";
import path from "node:path";

export type ProfileName = "leitura" | "docs" | "codigo";

export interface Profile {
	name: ProfileName;
	description: string;
	read: string[];
	write: string[];
	/** none: sem bash. sandbox: bash livre dentro do bwrap, sem rede e sem a home. */
	bash: "none" | "sandbox";
}

// Nunca lidos nem escritos, em qualquer perfil.
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
];

// Podem ser lidos, nunca escritos pelo agente.
export const WRITE_DENY = ["**/.git/**", ".git", "**/.workbench/**", ".workbench.json"];

const BUILTIN: Record<ProfileName, Profile> = {
	leitura: {
		name: "leitura",
		description: "Lê o projeto e responde. Não escreve nada e não roda comandos.",
		read: ["**"],
		write: [],
		bash: "none",
	},
	docs: {
		name: "docs",
		description: "Lê o projeto e escreve só documentação (Markdown e docs/). Não roda comandos.",
		read: ["**"],
		write: ["**/*.md", "docs/**"],
		bash: "none",
	},
	codigo: {
		name: "codigo",
		description:
			"Lê e escreve código no projeto. Pode rodar comandos num sandbox sem rede, que só enxerga o projeto e as ferramentas instaladas.",
		read: ["**"],
		write: ["**"],
		bash: "sandbox",
	},
};

export const PROFILE_NAMES = Object.keys(BUILTIN) as ProfileName[];

/** `.workbench.json` opcional na raiz do projeto: só exceções aos perfis. */
export interface ProjectConfig {
	perfil?: ProfileName;
	negar?: string[];
	perfis?: Partial<Record<ProfileName, { escrita?: string[] }>>;
}

export function loadProjectConfig(root: string): ProjectConfig {
	const file = path.join(root, ".workbench.json");
	if (!existsSync(file)) return {};
	const cfg = JSON.parse(readFileSync(file, "utf8")) as ProjectConfig;
	if (cfg.perfil && !PROFILE_NAMES.includes(cfg.perfil)) {
		throw new Error(`.workbench.json: perfil desconhecido "${cfg.perfil}" (use ${PROFILE_NAMES.join(", ")})`);
	}
	return cfg;
}

export interface EffectiveScope {
	profile: Profile;
	read: string[];
	write: string[];
	deny: string[];
	writeDeny: string[];
}

export function resolveScope(name: ProfileName | undefined, cfg: ProjectConfig): EffectiveScope {
	const chosen = name ?? cfg.perfil ?? "leitura";
	const profile = BUILTIN[chosen];
	if (!profile) throw new Error(`perfil desconhecido "${chosen}" (use ${PROFILE_NAMES.join(", ")})`);
	return {
		profile,
		read: profile.read,
		write: [...profile.write, ...(cfg.perfis?.[chosen]?.escrita ?? [])],
		deny: [...ALWAYS_DENY, ...(cfg.negar ?? [])],
		writeDeny: WRITE_DENY,
	};
}
