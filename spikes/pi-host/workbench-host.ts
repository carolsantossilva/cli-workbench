// Spike: Workbench as a Pi host. The Go CLI prepares/verifies runs and stays the
// source of truth for the contract; this extension applies the prepared run's
// scope at tool-call time and audits what is sent to the provider.
//
// Interactive: WORKBENCH_BIN=/path/to/workbench pi -e ./workbench-host.ts   then  /wb brainstorm
import { execFile } from "node:child_process";
import { createHash } from "node:crypto";
import { appendFileSync, mkdirSync, readFileSync } from "node:fs";
import path from "node:path";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { checkPath, redactSecrets } from "./policy.ts";

export interface RunContext {
	runId: string;
	workflowId: string;
	instructionPath: string;
	inputs: string[];
	readPaths: string[];
	writePaths: string[];
	allowedCommands: string[];
	nextHandoff: string;
}

export interface HostDecision {
	tool: string;
	target: string;
	allowed: boolean;
	reason?: string;
}

function runCli(bin: string, root: string, args: string[]): Promise<any> {
	return new Promise((resolve, reject) => {
		execFile(bin, args, { cwd: root }, (err, stdout) => {
			// JSON mode reports errors as JSON on stdout with a non-zero exit.
			try {
				resolve(JSON.parse(stdout));
			} catch {
				reject(err ?? new Error(`unparseable output from workbench ${args.join(" ")}`));
			}
		});
	});
}

export function createWorkbenchHost(opts: { bin: string; root: string }) {
	const { bin, root } = opts;
	const auditPath = path.join(root, ".workbench", "pi-host", "sends.jsonl");
	const decisions: HostDecision[] = [];
	let run: RunContext | undefined;

	async function prepare(workflowId: string): Promise<RunContext> {
		const res = await runCli(bin, root, ["workflow", "run", workflowId, "--format", "json"]);
		if (!res.ok) throw new Error(`workbench workflow run ${workflowId}: ${res.code} ${res.message ?? ""}`);
		const c = res.workflowRun.context;
		run = {
			runId: res.workflowRun.runId,
			workflowId: c.workflowId,
			instructionPath: c.instructionPath,
			inputs: (c.inputs ?? []).map((i: { path: string }) => i.path),
			readPaths: c.readPaths ?? [],
			writePaths: c.writePaths ?? [],
			allowedCommands: c.allowedCommands ?? [],
			nextHandoff: c.nextHandoff,
		};
		return run;
	}

	async function verify(): Promise<any> {
		if (!run) throw new Error("no active workflow run");
		const res = await runCli(bin, root, ["workflow", "verify", run.workflowId, run.runId, "--format", "json"]);
		if (res.ok) run = undefined;
		return res;
	}

	function decide(tool: string, target: string, allowed: boolean, reason?: string) {
		decisions.push({ tool, target, allowed, reason });
		return allowed ? undefined : { block: true, reason: `Workbench policy: ${reason}` };
	}

	function extension(pi: ExtensionAPI) {
		pi.on("tool_call", async (event) => {
			const input = event.input as Record<string, unknown>;
			const target = String(input.path ?? input.command ?? ".");
			if (!run) return decide(event.toolName, target, false, "no active Workbench workflow; run /wb <workflow>");
			const scope = { readPaths: run.readPaths, writePaths: run.writePaths };
			switch (event.toolName) {
				case "read":
				case "grep":
				case "find": {
					const d = checkPath(root, String(input.path ?? "."), "read", scope);
					return decide(event.toolName, target, d.allow, d.allow ? undefined : d.reason);
				}
				case "ls": {
					const d = checkPath(root, String(input.path ?? "."), "list", scope);
					return decide(event.toolName, target, d.allow, d.allow ? undefined : d.reason);
				}
				case "write":
				case "edit": {
					const d = checkPath(root, String(input.path ?? ""), "write", scope);
					return decide(event.toolName, target, d.allow, d.allow ? undefined : d.reason);
				}
				case "bash":
					// Declared commands go through `workbench workflow run --command`, never free bash.
					return decide("bash", target, false, "free bash is not part of the workflow contract");
				default:
					return decide(event.toolName, target, false, "tool not covered by the workflow contract");
			}
		});

		// Redact before the result enters the transcript, so secrets are neither sent nor persisted.
		pi.on("tool_result", async (event) => {
			let changed = false;
			const content = event.content.map((block) => {
				if (block.type !== "text") return block;
				const r = redactSecrets(block.text);
				if (r.found.length === 0) return block;
				changed = true;
				decisions.push({ tool: "redact", target: String(event.input.path ?? "?"), allowed: true, reason: r.found.join(",") });
				return { ...block, text: r.text };
			});
			return changed ? { content, structuredContent: undefined } : undefined;
		});

		pi.on("before_agent_start", async (event) => {
			if (!run) return;
			const instruction = readFileSync(path.join(root, run.instructionPath), "utf8");
			return {
				systemPrompt: [
					event.systemPrompt,
					`# Active Workbench workflow: ${run.workflowId} (run ${run.runId})`,
					instruction,
					"## Enforced scope",
					`Inputs: ${run.inputs.join(", ") || "none"}`,
					`Readable: ${[...run.readPaths, ...run.writePaths].join(", ")}`,
					`Writable: ${run.writePaths.join(", ")}`,
					"Calls outside this scope are blocked by the host, not just discouraged.",
				].join("\n\n"),
			};
		});

		// Audit every outbound request: which model, how big, a hash — never the content itself.
		// `context` fires for every provider (conversation only, no system prompt);
		// `before_provider_request` sees the exact wire payload but only for HTTP providers.
		function audit(source: "context" | "payload", body: string, found: string[], model: { provider?: string; id?: string } | undefined) {
			mkdirSync(path.dirname(auditPath), { recursive: true });
			appendFileSync(
				auditPath,
				JSON.stringify({
					ts: new Date().toISOString(),
					source,
					runId: run?.runId ?? null,
					workflowId: run?.workflowId ?? null,
					provider: model?.provider ?? null,
					model: model?.id ?? null,
					bytes: Buffer.byteLength(body),
					sha256: createHash("sha256").update(body).digest("hex"),
					secretsRedacted: found,
				}) + "\n",
			);
		}

		pi.on("context", async (event, ctx) => {
			const body = JSON.stringify(event.messages);
			const r = redactSecrets(body);
			audit("context", body, r.found, ctx.model);
			return r.found.length > 0 ? { messages: JSON.parse(r.text) } : undefined;
		});

		pi.on("before_provider_request", async (event, ctx) => {
			const body = JSON.stringify(event.payload ?? null);
			const r = redactSecrets(body);
			audit("payload", body, r.found, ctx.model);
			return r.found.length > 0 ? JSON.parse(r.text) : undefined;
		});

		pi.registerCommand("wb", {
			description: "Start a Workbench workflow: /wb <brainstorm|grill-me|design|build|review>",
			handler: async (args, ctx) => {
				const id = args.trim();
				const r = await prepare(id);
				ctx.ui.notify(`Workbench ${id} prepared (run ${r.runId}); writable: ${r.writePaths.join(", ")}`);
				pi.sendUserMessage(`Start the Workbench ${id} workflow. Read the inputs (${r.inputs.join(", ")}) and produce ${r.nextHandoff}.`);
			},
		});

		pi.registerCommand("wb-verify", {
			description: "Verify the active Workbench workflow run",
			handler: async (_args, ctx) => {
				const res = await verify();
				ctx.ui.notify(`Workbench verify: ${res.status}${res.ok ? "" : ` (${res.code})`}`);
			},
		});
	}

	return { prepare, verify, extension, decisions, auditPath, get run() { return run; } };
}

export default function (pi: ExtensionAPI) {
	createWorkbenchHost({ bin: process.env.WORKBENCH_BIN ?? "workbench", root: process.cwd() }).extension(pi);
}
