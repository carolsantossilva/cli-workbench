// Extensão do Pi que aplica a Workbench dentro da sessão: decide cada chamada de
// ferramenta, limpa os resultados e audita tudo o que vai para o provedor.
import { createHash } from "node:crypto";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { Type } from "typebox";
import type { Gate } from "./gate.ts";
import { redactSecrets } from "./policy.ts";
import type { EffectiveScope } from "./profiles.ts";
import { dropDeniedLines, redactDeep } from "./redact.ts";
import type { Audit } from "./usage.ts";

export interface ExtensionOptions {
	root: string;
	scope: EffectiveScope;
	gate: Gate;
	audit: Audit;
	/** Habilita a ferramenta ask_user. */
	askUser?: (question: string) => Promise<string>;
}

export function workbenchExtension(o: ExtensionOptions): (pi: ExtensionAPI) => void {
	return (pi) => {
		if (o.askUser) registerAskUser(pi, o.askUser, o);
		pi.on("tool_call", async (event) => decideToolCall(event.toolName, event.input as Record<string, unknown>, o));
		pi.on("tool_result", async (event) => cleanToolResult(event.toolName, event.content, o));
		auditProviderTraffic(pi, o.audit);
	};
}

function registerAskUser(pi: ExtensionAPI, askUser: (question: string) => Promise<string>, { gate, audit }: ExtensionOptions) {
	pi.registerTool({
		name: "ask_user",
		label: "Perguntar ao usuário",
		description: "Faz uma pergunta ao usuário e espera a resposta. Use para decisões que só o usuário pode tomar; uma pergunta por chamada.",
		parameters: Type.Object({ question: Type.String({ description: "A pergunta, curta e concreta, com uma recomendação se houver" }) }),
		async execute(_id, params) {
			const resposta = (await gate.serial(() => askUser(params.question))).trim();
			audit("question", { bytes: Buffer.byteLength(resposta) });
			return { content: [{ type: "text", text: resposta || "(sem resposta: decida com uma premissa explícita)" }], details: {} };
		},
	});
}

/** Toda chamada passa por aqui antes de acontecer; ferramenta desconhecida é negada. */
function decideToolCall(tool: string, input: Record<string, unknown>, { scope, gate, askUser }: ExtensionOptions) {
	const target = String(input.path ?? input.command ?? input.question ?? ".");
	switch (tool) {
		case "read":
		case "grep":
		case "find":
		case "ls":
			return gate.checkFile(tool, String(input.path ?? "."), "read");
		case "write":
		case "edit":
			return gate.checkFile(tool, String(input.path ?? ""), "write");
		case "ask_user":
			return askUser ? gate.decide("ask_user", target, true) : gate.decide("ask_user", target, false, "sem usuário para responder", "rule");
		case "bash":
			return scope.profile.bash !== "none"
				? gate.decide("bash", target, true, "sandbox")
				: gate.decide("bash", target, false, `o sandbox "${scope.profile.name}" não roda comandos`, "rule");
		default:
			return gate.decide(tool, target, false, "ferramenta fora do contrato da Workbench", "rule");
	}
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
