// Extensão do Pi que aplica a Workbench dentro da sessão: decide cada chamada de
// ferramenta, limpa os resultados e audita tudo o que vai para o provedor.
import { createHash } from "node:crypto";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import type { Gate } from "./gate.ts";
import { redactSecrets } from "./policy.ts";
import { dropDeniedLines, redactDeep } from "./redact.ts";
import type { Audit } from "./usage.ts";
import { registerTools, type ToolContext, toolSpec } from "./tools.ts";

export interface ExtensionOptions extends ToolContext {
	root: string;
	gate: Gate;
	audit: Audit;
}

export function workbenchExtension(o: ExtensionOptions): (pi: ExtensionAPI) => void {
	return (pi) => {
		registerTools(pi, o, o);
		pi.on("tool_call", async (event) => decideToolCall(event.toolName, event.input as Record<string, unknown>, o));
		pi.on("tool_result", async (event) => cleanToolResult(event.toolName, event.content, o));
		auditProviderTraffic(pi, o.audit);
	};
}

/** Toda chamada passa por aqui antes de acontecer; ferramenta desconhecida é negada. */
export function decideToolCall(tool: string, input: Record<string, unknown>, o: ExtensionOptions) {
	const target = String(input.path ?? input.command ?? input.question ?? input.query ?? input.action ?? ".");
	const spec = toolSpec(tool);
	return spec ? spec.decide(tool, input, target, o, o.gate) : o.gate.decide(tool, target, false, "ferramenta fora do contrato da Workbench", "rule");
}

/**
 * Antes de o resultado entrar no transcript: tira linhas de arquivos negados
 * (grep/find/ls varrem diretórios) e redige segredos.
 */
function cleanToolResult(tool: string, blocks: { type: string; text?: string }[], { root, scope, audit }: ExtensionOptions) {
	let changed = false;
	const content = blocks.map((block) => {
		if (block.type !== "text" || block.text === undefined) return block;
		let text = block.text;
		if (["grep", "find", "ls"].includes(tool)) text = dropDeniedLines(text, root, scope.deny);
		const r = redactSecrets(text);
		if (r.found.length > 0) audit("redact", { tool, kinds: r.found });
		if (r.text === block.text) return block;
		changed = true;
		return { ...block, text: r.text };
	});
	return changed ? { content: content as never, structuredContent: undefined } : undefined;
}

/**
 * Auditoria de cada envio ao provedor (modelo, tamanho e hash, nunca o conteúdo) e
 * redação de segredos no contexto e no payload. A redação percorre cada string, não o
 * JSON serializado: assim os padrões com \b funcionam depois de quebras de linha e uma
 * chave nunca casa entre duas mensagens.
 */
function auditProviderTraffic(pi: ExtensionAPI, audit: Audit) {
	const send = (source: string, body: string, found: string[], model?: { provider?: string; id?: string }) =>
		audit("send", {
			source,
			provider: model?.provider ?? null,
			model: model?.id ?? null,
			bytes: Buffer.byteLength(body),
			sha256: createHash("sha256").update(body).digest("hex"),
			secretsRedacted: found,
		});

	pi.on("context", async (event, ctx) => {
		const r = redactDeep(event.messages);
		send("context", JSON.stringify(r.value), r.found, ctx.model);
		return r.found.length > 0 ? { messages: r.value } : undefined;
	});

	pi.on("before_provider_request", async (event, ctx) => {
		const r = redactDeep(event.payload ?? null);
		send("payload", JSON.stringify(r.value), r.found, ctx.model);
		return r.found.length > 0 ? r.value : undefined;
	});
}
