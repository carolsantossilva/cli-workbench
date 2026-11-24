// .workbench/ do projeto: estado das etapas, marcadores de run e auditoria.
// Fica fora do git sozinha (.gitignore próprio com "*"), sem mexer no .gitignore do projeto.
//
// O host escreve aqui por fora do sandbox, e a pasta pode vir de um repositório clonado.
// Por isso nada dentro dela é seguido como symlink: as pastas são conferidas com lstat
// e os arquivos são abertos com O_NOFOLLOW (um link no último componente vira erro).
import { closeSync, constants, lstatSync, mkdirSync, openSync, writeSync } from "node:fs";
import path from "node:path";

const IGNORE = "# Criado pelo wb: estado e auditoria locais, fora do git.\n*\n";

const symlinkError = (p: string) => new Error(`${p} é um symlink ou não é do tipo esperado; remova-o para continuar`);

/** Cria a pasta se preciso; recusa se ela já existe como outra coisa (symlink incluído). */
function realDir(dir: string): string {
	const st = lstatSync(dir, { throwIfNoEntry: false });
	if (st && !st.isDirectory()) throw symlinkError(dir);
	if (!st) mkdirSync(dir);
	return dir;
}

function writeNoFollow(file: string, data: string, flags: number): void {
	let fd: number;
	try {
		fd = openSync(file, constants.O_WRONLY | constants.O_CREAT | constants.O_NOFOLLOW | flags, 0o644);
	} catch (err) {
		if ((err as NodeJS.ErrnoException).code === "ELOOP") throw symlinkError(file);
		throw err;
	}
	try {
		writeSync(fd, data);
	} finally {
		closeSync(fd);
	}
}

/** Grava um arquivo dentro de .workbench/ sem seguir symlink. */
export const writeInside = (file: string, data: string) => writeNoFollow(file, data, constants.O_TRUNC);
/** Acrescenta a um arquivo dentro de .workbench/ sem seguir symlink. */
export const appendInside = (file: string, data: string) => writeNoFollow(file, data, constants.O_APPEND);

/** Cria .workbench/ (e o .gitignore dela) se preciso e devolve o caminho. */
export function workbenchDir(root: string): string {
	const dir = realDir(path.join(root, ".workbench"));
	const ignore = path.join(dir, ".gitignore");
	const st = lstatSync(ignore, { throwIfNoEntry: false });
	if (!st) writeNoFollow(ignore, IGNORE, constants.O_EXCL);
	else if (!st.isFile()) throw symlinkError(ignore);
	return dir;
}

export const auditDirFor = (root: string) => realDir(path.join(workbenchDir(root), "runs"));
