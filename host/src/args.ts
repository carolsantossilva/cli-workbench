// Parser de argumentos do wb: flags em qualquer posição (clig.dev), "--flag=valor" ou
// "--flag valor", e "--" encerra as opções (POSIX).
import { THINKING_LEVELS, type ThinkingLevel } from "./host.ts";
import { SANDBOX_NAMES, type SandboxName } from "./profiles.ts";

export interface Args {
	project: string;
	sandbox?: SandboxName;
	model?: string;
	thinking?: ThinkingLevel;
	noInput: boolean;
	help: boolean;
	version: boolean;
	rest: string[];
}

export function parseArgs(argv: string[], cwd = process.cwd()): Args {
	const a: Args = { project: cwd, noInput: false, help: false, version: false, rest: [] };
	const value = (i: number, flag: string) => {
		const v = argv[i + 1];
		if (v === undefined || v.startsWith("-")) throw new Error(`${flag} precisa de um valor`);
		return v;
	};
	for (let i = 0; i < argv.length; i++) {
		let arg = argv[i];
		if (arg === "--") {
			a.rest.push(...argv.slice(i + 1));
			break;
		}
		const eq = arg.startsWith("--") ? arg.indexOf("=") : -1;
		let inline: string | undefined;
		if (eq > 0) {
			inline = arg.slice(eq + 1);
			arg = arg.slice(0, eq);
		}
		const take = (flag: string) => {
			const v = inline ?? value(i++, flag);
			if (v === "") throw new Error(`${flag} precisa de um valor`);
			return v;
		};
		switch (arg) {
			case "-C":
			case "--project":
				a.project = take(arg);
				break;
			case "-s":
			case "--sandbox":
				a.sandbox = take(arg) as SandboxName;
				break;
			case "-m":
			case "--model":
				a.model = take(arg);
				break;
			case "--thinking":
				a.thinking = take(arg) as ThinkingLevel;
				break;
			case "--no-input":
				a.noInput = true;
				break;
			case "-h":
			case "--help":
				a.help = true;
				break;
			case "-V":
			case "--version":
				a.version = true;
				break;
			default:
				if (arg.startsWith("-") && arg !== "-") throw new Error(`opção desconhecida: ${arg} (veja wb --help)`);
				a.rest.push(argv[i]);
		}
	}
	if (a.sandbox && !SANDBOX_NAMES.includes(a.sandbox)) throw new Error(`sandbox desconhecido "${a.sandbox}" (use ${SANDBOX_NAMES.join(", ")})`);
	if (a.thinking && !THINKING_LEVELS.includes(a.thinking)) throw new Error(`nível de raciocínio desconhecido "${a.thinking}" (use ${THINKING_LEVELS.join(", ")})`);
	return a;
}

