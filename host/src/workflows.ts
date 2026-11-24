// Workflows: etapas com entradas, saída e escopo próprios, encadeadas por arquivos
// em docs/workbench/. O design precisa ser aprovado (por hash) antes do build.
import { createHash } from "node:crypto";
import { existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";
import { ALWAYS_DENY, type EffectiveScope, type ProjectConfig, WRITE_DENY } from "./profiles.ts";

export type StepId = "brainstorm" | "grill-me" | "design" | "build" | "review";
export const STEP_IDS: StepId[] = ["brainstorm", "grill-me", "design", "build", "review"];

export const DIR = "docs/workbench";
export const IDEA = `${DIR}/idea.md`;

export interface StepDef {
	id: StepId;
	goal: string;
	/** Precisam existir antes de rodar. */
	inputs: string[];
	/** Lidos se existirem. */
	optionalInputs: string[];
	output: string;
	write: string[];
	bash: "none" | "sandbox" | "sandbox-ro";
	/** O agente pode fazer perguntas ao usuário durante a etapa. */
	asksUser: boolean;
	/** Etapa cuja saída precisa estar aprovada (e inalterada) antes desta. */
	requiresApproved?: StepId;
	instructions: string[];
}

// Não use a palavra "evidência" nas instruções: com o gpt-6-luna ela dispara uma recusa
// espúria ("voting systems or elections infrastructure") em quase toda execução.
// Medido em 2026-11-24: frase com "evidência" recusou 6/6, a mesma sem ela 0/6.
export const STEPS: Record<StepId, StepDef> = {
	brainstorm: {
		id: "brainstorm",
		goal: "transformar a ideia em entendimento compartilhado: propósito, usuários, resultados, limites",
		inputs: [IDEA],
		optionalInputs: [],
		output: `${DIR}/brief.md`,
		write: [`${DIR}/brief.md`],
		bash: "none",
		asksUser: true,
		instructions: [
			"Leia a ideia e o contexto relevante do projeto.",
			"Faça poucas perguntas focadas (ferramenta `ask_user`) só quando a resposta mudar o resultado; senão, declare premissas.",
			`Escreva ${DIR}/brief.md com: propósito, usuários, resultados esperados, fora do escopo, restrições e perguntas em aberto.`,
			"Preserve a incerteza em vez de inventar requisitos. Não escreva especificação funcional nem código.",
		],
	},
	"grill-me": {
		id: "grill-me",
		goal: "desafiar o brief: ambiguidades, premissas, riscos e casos extremos",
		inputs: [`${DIR}/brief.md`],
		optionalInputs: [`${DIR}/decisions.md`],
		output: `${DIR}/decisions.md`,
		write: [`${DIR}/decisions.md`],
		bash: "none",
		asksUser: true,
		instructions: [
			"Leia o brief e procure termos ambíguos, premissas não testadas, casos extremos e riscos que mudariam escopo ou arquitetura.",
			"Pergunte ao usuário com a ferramenta `ask_user`: uma pergunta concreta por vez, com uma recomendação quando tiver uma. Faça no máximo 7 perguntas.",
			`Registre em ${DIR}/decisions.md cada decisão (D1, D2…): pergunta, resposta, opções rejeitadas e consequência. Liste o que ficou em aberto.`,
			"Não reescreva o brief como se uma premissa estivesse aprovada.",
		],
	},
	design: {
		id: "design",
		goal: "especificar o próximo incremento o bastante para construir e verificar",
		inputs: [`${DIR}/brief.md`],
		optionalInputs: [`${DIR}/decisions.md`],
		output: `${DIR}/technical-design.md`,
		write: [`${DIR}/technical-design.md`],
		bash: "none",
		asksUser: false,
		instructions: [
			"Leia o brief, as decisões e o código existente que importa.",
			`Escreva ${DIR}/technical-design.md com: escopo do incremento, componentes e responsabilidades, fluxo de dados, comportamento em erro, testes que provam que funciona, e decisões tomadas.`,
			"Compare alternativas só onde mudariam o design. Marque claramente escolhas em aberto e premissas.",
			"Não escreva código. O design será aprovado pelo usuário antes do build.",
		],
	},
	build: {
		id: "build",
		goal: "implementar o design aprovado em lotes pequenos, com testes",
		inputs: [`${DIR}/technical-design.md`],
		optionalInputs: [`${DIR}/plan.md`, `${DIR}/decisions.md`],
		output: `${DIR}/plan.md`,
		write: ["**"],
		bash: "sandbox",
		asksUser: false,
		requiresApproved: "design",
		instructions: [
			"Leia o design aprovado e o estado atual do código antes de editar; preserve mudanças existentes.",
			"O usuário aprovou o design: escolhas marcadas nele como em aberto seguem a recomendação do próprio design (ou a opção mais simples, se não houver), registradas no plano como premissa. Não pare por causa delas.",
			`Escreva primeiro ${DIR}/plan.md com lotes pequenos em ordem de dependência, cada um com a verificação que prova que funcionou. Se o plano já existir, continue do primeiro lote não concluído.`,
			"Implemente lote a lote, com teste antes da mudança de comportamento. Rode os testes de verdade e marque no plano cada lote concluído com o comando rodado e a saída que mostra que funcionou.",
			"Pare se algo contradisser o design: registre no plano o que foi observado e a decisão necessária.",
		],
	},
	review: {
		id: "review",
		goal: "avaliar a mudança contra o design, o plano e os testes",
		inputs: [`${DIR}/technical-design.md`],
		optionalInputs: [`${DIR}/plan.md`, `${DIR}/decisions.md`],
		output: `${DIR}/review.md`,
		write: [`${DIR}/review.md`],
		bash: "sandbox-ro",
		asksUser: false,
		instructions: [
			"Veja o que mudou (`git diff`, `git status`) e compare com o design e o plano.",
			"Rode os testes do projeto; o sandbox desta etapa é só leitura, então nada do projeto pode ser alterado.",
			`Escreva ${DIR}/review.md com achados acionáveis: gravidade, arquivo, impacto concreto e como foi confirmado (comando, saída ou trecho de código). Separe defeitos verificados de limitações e premissas. Diga quais testes rodou e o resultado real.`,
			"Não altere código.",
		],
	},
};

export function stepScope(def: StepDef, cfg: ProjectConfig): EffectiveScope {
	return {
		profile: {
			name: `step ${def.id}`,
			description: `Etapa "${def.id}" da Workbench: ${def.goal}.`,
			read: ["**"],
			write: def.write,
			bash: def.bash,
		},
		read: ["**"],
		write: def.write,
		deny: [...ALWAYS_DENY, ...(cfg.deny ?? [])],
		writeDeny: WRITE_DENY,
	};
}

export function stepPrompt(def: StepDef, present: string[]): string {
	return [
		`# Etapa ${def.id}`,
		`Objetivo: ${def.goal}.`,
		`Entradas disponíveis: ${present.join(", ") || "nenhuma"}.`,
		`Saída: ${def.output}.`,
		"",
		...def.instructions.map((i) => `- ${i}`),
	].join("\n");
}

// ── Estado: aprovações por hash em .workbench/state.json ──

export interface Approval {
	file: string;
	sha256: string;
	at: string;
}
export interface State {
	approvals: Partial<Record<StepId, Approval>>;
	runs: Partial<Record<StepId, { runId: string; at: string; outputWritten: boolean }>>;
}

const statePath = (root: string) => path.join(root, ".workbench", "state.json");

export function loadState(root: string): State {
	const p = statePath(root);
	migrateLegacyState(root);
	if (!existsSync(p)) return { approvals: {}, runs: {} };
	const s = JSON.parse(readFileSync(p, "utf8")) as Partial<State>;
	return { approvals: s.approvals ?? {}, runs: s.runs ?? {} };
}

/** .workbench/estado.json (aprovacoes/execucoes) da versão em português vira state.json, uma vez. */
function migrateLegacyState(root: string): void {
	const legacy = path.join(root, ".workbench", "estado.json");
	if (!existsSync(legacy) || existsSync(statePath(root))) return;
	type Legacy = { aprovacoes?: State["approvals"]; execucoes?: Partial<Record<StepId, { runId: string; at: string; saida: boolean }>> };
	let old: Legacy;
	try {
		old = JSON.parse(readFileSync(legacy, "utf8")) as Legacy;
	} catch {
		return; // corrompido: fica onde está, sem migrar e sem travar o resto
	}
	const runs: State["runs"] = {};
	for (const [id, r] of Object.entries(old.execucoes ?? {})) if (r) runs[id as StepId] = { runId: r.runId, at: r.at, outputWritten: r.saida };
	saveState(root, { approvals: old.aprovacoes ?? {}, runs });
	rmSync(legacy);
}

export function saveState(root: string, state: State): void {
	mkdirSync(path.dirname(statePath(root)), { recursive: true });
	writeFileSync(statePath(root), JSON.stringify(state, null, 2) + "\n");
}

export function fileHash(root: string, rel: string): string | undefined {
	const p = path.join(root, rel);
	return existsSync(p) ? createHash("sha256").update(readFileSync(p)).digest("hex") : undefined;
}

export type ApprovalStatus = "approved" | "changed-after" | "pending" | "missing";

export function approvalStatus(root: string, id: StepId, state = loadState(root)): ApprovalStatus {
	const file = STEPS[id].output;
	const hash = fileHash(root, file);
	if (!hash) return "missing";
	const a = state.approvals[id];
	if (!a || a.file !== file) return "pending";
	return a.sha256 === hash ? "approved" : "changed-after";
}

export function approve(root: string, id: StepId): Approval {
	const file = STEPS[id].output;
	const sha256 = fileHash(root, file);
	if (!sha256) throw new Error(`não há ${file} para aprovar; rode "wb step run ${id}" antes`);
	const state = loadState(root);
	const a = { file, sha256, at: new Date().toISOString() };
	state.approvals[id] = a;
	saveState(root, state);
	return a;
}

/** Erro legível se a etapa não pode rodar agora (entrada faltando ou aprovação pendente). */
export function preflight(root: string, def: StepDef): string | undefined {
	const missing = def.inputs.filter((f) => !existsSync(path.join(root, f)));
	if (missing.length > 0) {
		const from = STEP_IDS.find((w) => missing.includes(STEPS[w].output));
		return `falta ${missing.join(", ")}${from ? `; rode "wb step run ${from}" antes` : def.id === "brainstorm" ? '; passe a ideia: wb step run brainstorm "sua ideia"' : ""}`;
	}
	if (def.requiresApproved) {
		const st = approvalStatus(root, def.requiresApproved);
		if (st !== "approved") {
			const file = STEPS[def.requiresApproved].output;
			return st === "changed-after"
				? `${file} mudou depois da aprovação; revise e aprove de novo: wb step approve ${def.requiresApproved}`
				: `${file} ainda não foi aprovado; revise e aprove: wb step approve ${def.requiresApproved}`;
		}
	}
	return undefined;
}
