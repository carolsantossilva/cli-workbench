import assert from "node:assert/strict";
import { test } from "node:test";
import { decideToolCall, workbenchExtension } from "../src/extension.ts";
import { describeScope } from "../src/host.ts";
import { resolveScope, SANDBOX_NAMES } from "../src/sandbox-profiles.ts";
import { enabledToolNames, TOOLS, type ToolContext } from "../src/tools.ts";
import { STEP_IDS, STEPS, stepScope } from "../src/workflows.ts";

/** Todas as combinações de escopo (sandboxes e etapas, com e sem wiki) e de usuário presente. */
function contexts(): ToolContext[] {
	const out: ToolContext[] = [];
	for (const cfg of [{}, { kbAi: { wiki: true } }]) {
		const scopes = [...SANDBOX_NAMES.map((n) => resolveScope(n, cfg)), ...STEP_IDS.map((id) => stepScope(STEPS[id], cfg))];
		for (const scope of scopes)
			for (const interactive of [false, true])
				for (const askUser of [undefined, async () => "ok"]) out.push({ scope, interactive, askUser });
	}
	return out;
}

const recordingGate = () => {
	const calls: { tool: string; allowed: boolean }[] = [];
	const gate = {
		decide: (tool: string, _target: string, allowed: boolean) => (calls.push({ tool, allowed }), allowed ? undefined : { block: true, reason: "negado" }),
		checkFile: async (tool: string) => (calls.push({ tool, allowed: true }), undefined),
		serial: <T>(f: () => Promise<T>) => f(),
	};
	return { gate: gate as never, calls };
};

test("cada ferramenta aparece numa só entrada da tabela", () => {
	const names = TOOLS.flatMap((t) => t.names);
	assert.equal(new Set(names).size, names.length);
});

test("toda ferramenta habilitada tem linha na descrição do escopo e é liberada no gate", async () => {
	for (const ctx of contexts()) {
		const description = describeScope(ctx.scope, [], ctx.interactive, ctx.askUser);
		for (const t of TOOLS) assert.ok(description.includes(t.describe(ctx, [])), `${t.names[0]} sem linha em ${ctx.scope.profile.name}`);
		for (const name of enabledToolNames(ctx)) {
			const { gate, calls } = recordingGate();
			const result = await decideToolCall(name, { path: "README.md" }, { ...ctx, root: "/projeto", gate, audit: (() => {}) as never });
			assert.equal(result, undefined, `${name} habilitada mas negada em ${ctx.scope.profile.name}`);
			assert.deepEqual(calls.map((c) => c.tool), [name]);
		}
	}
});

test("ferramenta desabilitada que não depende de caminho é negada no gate", async () => {
	for (const ctx of contexts()) {
		const enabled = new Set(enabledToolNames(ctx));
		for (const t of TOOLS.filter((t) => !t.names.some((n) => ["read", "write"].includes(n)))) {
			for (const name of t.names.filter((n) => !enabled.has(n))) {
				const { gate } = recordingGate();
				const result = await decideToolCall(name, {}, { ...ctx, root: "/projeto", gate, audit: (() => {}) as never });
				assert.ok(result?.block, `${name} desabilitada mas liberada em ${ctx.scope.profile.name}`);
			}
		}
	}
});

test("a extensão registra só as ferramentas próprias habilitadas", () => {
	const own = new Set(TOOLS.filter((t) => t.register).flatMap((t) => t.names));
	for (const ctx of contexts()) {
		const registered: string[] = [];
		const pi = { registerTool: (tool: { name: string }) => registered.push(tool.name), on: () => undefined };
		workbenchExtension({ ...ctx, root: "/projeto", gate: {} as never, audit: (() => {}) as never })(pi as never);
		assert.deepEqual(registered.sort(), enabledToolNames(ctx).filter((n) => own.has(n)).sort());
	}
});

test("ferramenta fora da tabela é negada", async () => {
	const { gate } = recordingGate();
	const result = await decideToolCall("web_fetch", {}, { scope: resolveScope("workspace-write", {}), root: "/projeto", gate, audit: (() => {}) as never });
	assert.ok(result?.block);
});
