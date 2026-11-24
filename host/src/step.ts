// Roda uma etapa de workflow: confere entradas e aprovação, monta o escopo da
// etapa, executa e registra o resultado no estado do projeto.
import { existsSync, mkdirSync, realpathSync, writeFileSync } from "node:fs";
import path from "node:path";
import { type RunOptions, type RunResult, runTask } from "./host.ts";
import { loadProjectConfig } from "./profiles.ts";
import {
	type ApprovalStatus,
	approvalStatus,
	fileHash,
	IDEA,
	loadState,
	preflight,
	saveState,
	type StepId,
	STEPS,
	stepPrompt,
	stepScope,
} from "./workflows.ts";

export interface StepRunOptions extends Omit<RunOptions, "task" | "scope" | "sandbox"> {
	id: StepId;
	/** brainstorm: a ideia (vira docs/workbench/idea.md). Outras etapas: pedido extra do usuário. */
	note?: string;
}

export interface StepRunResult extends RunResult {
	output: string;
	outputWritten: boolean;
	approval: ApprovalStatus;
}

export async function runStep(o: StepRunOptions): Promise<StepRunResult> {
	const root = realpathSync(o.projectDir);
	const def = STEPS[o.id];
	if (!def) throw new Error(`etapa desconhecida "${o.id}"`);

	if (o.id === "brainstorm" && o.note) {
		mkdirSync(path.join(root, path.dirname(IDEA)), { recursive: true });
		writeFileSync(path.join(root, IDEA), `# Ideia\n\n${o.note.trim()}\n`);
	}
	const blocker = preflight(root, def);
	if (blocker) throw new Error(`não dá para rodar "${o.id}": ${blocker}`);

	const present = [...def.inputs, ...def.optionalInputs].filter((f) => existsSync(path.join(root, f)));
	let task = stepPrompt(def, present);
	if (o.note && o.id !== "brainstorm") task += `\n\nPedido do usuário para esta etapa: ${o.note.trim()}`;

	const before = fileHash(root, def.output);
	const result = await runTask({
		...o,
		projectDir: root,
		task,
		scope: stepScope(def, loadProjectConfig(root)),
		askUser: def.asksUser ? o.askUser : undefined,
	});
	const after = fileHash(root, def.output);
	const outputWritten = Boolean(after && after !== before);

	const state = loadState(root);
	state.runs[o.id] = { runId: result.runId, at: new Date().toISOString(), outputWritten };
	saveState(root, state);

	return { ...result, output: def.output, outputWritten, approval: approvalStatus(root, o.id, state) };
}
