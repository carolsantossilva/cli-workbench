// Texto vindo do modelo (caminhos, comandos, perguntas) é escapado antes de ir para o
// terminal: sequências ANSI, controles C0/C1, controles bidirecionais, separadores de
// linha Unicode e caracteres de largura zero poderiam falsificar o que a pessoa vê
// num pedido de aprovação.
const UNSAFE = /[\u0000-\u0008\u000b-\u001f\u007f-\u009f\u061c\u200b-\u200f\u2028\u2029\u202a-\u202e\u2060\u2066-\u2069\ufeff]/g;

const escape = (c: string) => {
	const code = c.codePointAt(0) ?? 0;
	return code <= 0xff ? `\\x${code.toString(16).padStart(2, "0")}` : `\\u{${code.toString(16)}}`;
};

/** Escapa caracteres que alteram a exibição; mantém \t e \n. */
export const safe = (s: string): string => s.replace(UNSAFE, escape);

/** Como safe, e também escapa \n: para caminhos, comandos e motivos numa linha só. */
export const oneLine = (s: string): string => safe(s).replace(/\n/g, "\\n");
