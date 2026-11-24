import assert from "node:assert/strict";
import { test } from "node:test";
import { oneLine, safe } from "../src/terminal.ts";

test("escapa ANSI, controles, bidi, separadores de linha e largura zero", () => {
	assert.equal(safe("a\u001b[2Jb"), "a\\x1b[2Jb");
	assert.equal(safe("x\u202ey"), "x\\u{202e}y");
	assert.equal(safe("x\u2066y\u2028z\u200bw\ufeff"), "x\\u{2066}y\\u{2028}z\\u{200b}w\\u{feff}");
	assert.equal(safe("linha 1\nlinha 2\tfim"), "linha 1\nlinha 2\tfim", "mantém \\n e \\t");
	assert.equal(oneLine("a\nb\r"), "a\\nb\\x0d");
});
