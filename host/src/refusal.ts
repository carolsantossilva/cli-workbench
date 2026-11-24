// Recusas espúrias do gpt-6-luna: detectar a recusa e evitar as palavras que a disparam.

const REFUSAL = /^\s*(n[ãa]o posso ajudar|n[ãa]o posso atender|I can(?:no|')t assist|I can(?:no|')t help|I'm sorry, but I can(?:no|')t)/i;
export function looksLikeRefusal(text: string): boolean {
	return text.length < 300 && REFUSAL.test(text);
}

// Palavras do texto do usuário que disparam recusa espúria no gpt-6-luna ("voting systems
// or elections infrastructure"), trocadas antes de enviar. Medido em 2026-11-24, 6 execuções
// cada: "evidência" recusou 6/6; "prova", "comprovação" e "evidencia" sem acento, 0/6.
// Palavras dentro de caminhos (docs/evidência.md) ficam como estão.
const PROMPT_REWRITES: { from: RegExp; to: string }[] = [{ from: /(?<![\p{L}\p{N}/._-])evidência(s?)(?![\p{L}\p{N}/_-]|\.[\p{L}\p{N}])/giu, to: "prova" }];

function matchCase(original: string, replacement: string): string {
	if (original === original.toUpperCase()) return replacement.toUpperCase();
	if (original[0] === original[0].toUpperCase()) return replacement[0].toUpperCase() + replacement.slice(1);
	return replacement;
}

/** Troca no texto do usuário as palavras que disparam recusa espúria. */
export function rewritePrompt(text: string): { text: string; count: number } {
	let count = 0;
	for (const { from, to } of PROMPT_REWRITES) {
		text = text.replace(from, (m: string, plural: string) => (count++, matchCase(m, to + (plural ? "s" : ""))));
	}
	return { text, count };
}
