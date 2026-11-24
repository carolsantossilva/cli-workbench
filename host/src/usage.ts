// Auditoria do run (um JSONL por execução) e contagem de tokens das respostas do modelo.
import { randomBytes } from "node:crypto";
import { appendInside } from "./workdir.ts";

export type Audit = (event: string, data: Record<string, unknown>) => void;

export function newRunId(): string {
	return `${new Date().toISOString().replace(/[-:]/g, "").replace(/\..+/, "")}-${randomBytes(3).toString("hex")}`;
}

/** Cada evento vira uma linha JSON em auditPath, que nunca é seguido como symlink. */
export function openAudit(auditPath: string, runId: string): Audit {
	return (event, data) => appendInside(auditPath, JSON.stringify({ ts: new Date().toISOString(), runId, event, ...data }) + "\n");
}

/** Tokens somados das respostas do modelo, como o Pi reporta (input não inclui o cache). */
export interface TokenUsage {
	input: number;
	output: number;
	cacheRead: number;
	cacheWrite: number;
	/** Parte de output; só alguns provedores informam. */
	reasoning: number;
	total: number;
	/** Custo em dólar: cobrado pelo provedor quando ele informa, senão estimado pelo catálogo do Pi. */
	cost: number;
	costFromProvider: boolean;
	responses: number;
}

/** O usage de uma mensagem do Pi (pi-ai), só com o que a Workbench usa. */
interface PiUsage {
	input: number;
	output: number;
	cacheRead: number;
	cacheWrite: number;
	reasoning?: number;
	totalTokens: number;
	cost?: { total: number; totalFromProvider?: boolean };
}

interface AssistantMessageLike {
	role?: string;
	provider?: string;
	model?: string;
	stopReason?: string;
	usage?: PiUsage;
}

export const emptyUsage = (): TokenUsage => ({ input: 0, output: 0, cacheRead: 0, cacheWrite: 0, reasoning: 0, total: 0, cost: 0, costFromProvider: true, responses: 0 });

function addUsage(sum: TokenUsage, u: PiUsage): void {
	sum.input += u.input;
	sum.output += u.output;
	sum.cacheRead += u.cacheRead;
	sum.cacheWrite += u.cacheWrite;
	sum.reasoning += u.reasoning ?? 0;
	sum.total += u.totalTokens;
	sum.cost += u.cost?.total ?? 0;
	sum.costFromProvider &&= Boolean(u.cost?.totalFromProvider);
	sum.responses++;
}

/** Soma o uso de uma resposta do modelo ao total e grava o evento "usage". Ignora o que não for resposta. */
export function recordUsage(sum: TokenUsage, message: unknown, audit: Audit): void {
	const m = message as AssistantMessageLike;
	if (m.role !== "assistant" || !m.usage) return;
	const u = m.usage;
	addUsage(sum, u);
	audit("usage", {
		model: `${m.provider}/${m.model}`,
		stopReason: m.stopReason,
		input: u.input,
		output: u.output,
		cacheRead: u.cacheRead,
		cacheWrite: u.cacheWrite,
		reasoning: u.reasoning,
		total: u.totalTokens,
		cost: u.cost?.total,
		costFromProvider: Boolean(u.cost?.totalFromProvider),
	});
}
