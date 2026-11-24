// Decide se cada ação do agente acontece: pelo sandbox, por regra fixa ou perguntando
// ao usuário. Toda decisão é registrada na auditoria e devolvida ao Pi.
import type { Audit } from "./usage.ts";
import { checkPath } from "./policy.ts";
import type { EffectiveScope } from "./profiles.ts";

export interface Decision {
	tool: string;
	target: string;
	allowed: boolean;
	reason?: string;
	/** Quem decidiu: o sandbox, uma regra fixa, ou o usuário ao ser perguntado. */
	by?: "sandbox" | "rule" | "user";
}

export interface ApprovalRequest {
	tool: string;
	mode: "read" | "write";
	path: string;
	reason: string;
}

/** once: só esta chamada. run: este arquivo, até o fim da execução. deny: nega. */
export type ApprovalAnswer = "once" | "run" | "deny";

/** O que o Pi recebe de um hook tool_call: nada (segue) ou um bloqueio com motivo. */
export type GateResult = { block: true; reason: string } | undefined;

export interface GateOptions {
	root: string;
	scope: EffectiveScope;
	audit: Audit;
	ask?: (req: ApprovalRequest) => Promise<ApprovalAnswer>;
	onDecision?: (d: Decision) => void;
}

export function createGate({ root, scope, audit, ask, onDecision }: GateOptions) {
	const decisions: Decision[] = [];
	// Liberações "até o fim da execução", por arquivo exato e modo.
	const granted = { read: new Set<string>(), write: new Set<string>() };

	// Perguntas em série (aprovações e ask_user na mesma fila): chamadas paralelas do
	// modelo não abrem dois prompts juntos, e um terminal só atende uma pergunta por vez.
	let promptQueue: Promise<unknown> = Promise.resolve();
	const serial = <T>(f: () => Promise<T>): Promise<T> => {
		const next = promptQueue.then(f);
		promptQueue = next.catch(() => {});
		return next;
	};

	const decide = (tool: string, target: string, allowed: boolean, reason?: string, by: Decision["by"] = "sandbox"): GateResult => {
		const d: Decision = { tool, target, allowed, reason, by };
		decisions.push(d);
		audit("decision", { ...d });
		onDecision?.(d);
		return allowed ? undefined : { block: true, reason: `Bloqueado pela Workbench: ${reason}` };
	};

	const askApproval = (req: ApprovalRequest): Promise<ApprovalAnswer> =>
		ask ? serial(() => ask(req)).catch(() => "deny" as const) : Promise.resolve("deny");

	/** Leitura ou escrita de um caminho: o escopo libera, uma regra fixa nega, ou o usuário decide. */
	async function checkFile(tool: string, input: string, mode: "read" | "write"): Promise<GateResult> {
		const d = checkPath(root, input, mode, scope);
		if (d.allow) return decide(tool, input, true);
		if (d.hard || !d.rel) return decide(tool, input, false, d.reason, "rule");
		if (granted[mode].has(d.rel)) return decide(tool, input, true, "liberado pelo usuário nesta execução", "user");
		const answer = await askApproval({ tool, mode, path: d.rel, reason: d.reason });
		audit("approval", { tool, mode, path: d.rel, answer });
		if (answer === "deny") return decide(tool, input, false, `${d.reason} (negado pelo usuário)`, "user");
		if (answer === "run") {
			granted[mode].add(d.rel);
			// Quem pode escrever um arquivo também pode lê-lo de volta.
			if (mode === "write") granted.read.add(d.rel);
		}
		return decide(tool, input, true, answer === "run" ? "liberado pelo usuário nesta execução" : "liberado pelo usuário uma vez", "user");
	}

	return { decisions, decide, checkFile, serial };
}

export type Gate = ReturnType<typeof createGate>;
