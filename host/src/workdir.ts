// .workbench/ do projeto: estado das etapas, marcadores de run e auditoria.
// Fica fora do git sozinha (.gitignore próprio com "*"), sem mexer no .gitignore do projeto.
import { existsSync, lstatSync, mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";

const IGNORE = "# Criado pelo wb: estado e auditoria locais, fora do git.\n*\n";

/** Cria .workbench/ (e o .gitignore dela) se preciso e devolve o caminho. Recusa symlink. */
export function workbenchDir(root: string): string {
	const dir = path.join(root, ".workbench");
	const st = lstatSync(dir, { throwIfNoEntry: false });
	// Um .workbench que é symlink levaria as escritas do host (que rodam fora do sandbox) para outro lugar.
	if (st && !st.isDirectory()) throw new Error(`${dir} existe e não é um diretório (symlink?); remova-o para continuar`);
	mkdirSync(dir, { recursive: true });
	const ignore = path.join(dir, ".gitignore");
	if (!existsSync(ignore)) writeFileSync(ignore, IGNORE);
	return dir;
}

export const auditDirFor = (root: string) => path.join(workbenchDir(root), "runs");
