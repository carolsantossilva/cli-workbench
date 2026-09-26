# Station

Station is a local CLI for validating a workspace contract and preparing/verifying policy-bounded workflow handoffs. It does not call language models or provide a task/planner app. The agent or host remains responsible for carrying out the work.

## Build and install

The module declares Go 1.26 (`go.mod`); use Go 1.26 or newer on a current security-patched release (local verification used Go 1.27.1). The CLI itself has no Node.js requirement.

From this repository:

```sh
go build -o ./station ./cmd/station
./station --help
go install ./cmd/station
```

`go run ./cmd/station --help` is also useful during development. Go may download the module dependencies listed in `go.mod` when they are not cached; this is part of building/installing Station, not something Station does to a generated workspace.

## Commands

Commands without a path use the current directory as the workspace root. `inspect` accepts an optional relative or absolute path. The CLI supports `--help` and root `--version`.

```text
station new <idea>
station inspect [path] [--format text|json]
station validate [--format text|json]
station workflow run <workflow-id> [--format text|json] [--command <command-id>]
station workflow verify <workflow-id> <run-id> [--format text|json]
```

`--format` is available on `inspect`, `validate`, and both `workflow` commands; it is not a root-wide flag and is not supported by `new`. The default is text. JSON operations return a result object with fields such as `ok`, `status`, `code`, and `exitCode`, plus operation-specific data. In JSON mode errors are returned as JSON on stdout; in text mode errors go to stderr. `validate` prints `workspace.json is valid` on success in text mode. Workflow IDs are `brainstorm`, `grill-me`, `design`, `build`, and `review`.

`new` takes one plain, nonblank text argument and stores it as the idea; it does not parse JSON. A JSON-looking string passed as that argument is still just idea text. Examples, run from a suitable workspace directory:

```sh
station new "A personal planner idea"
station inspect --format json
station validate
station workflow run brainstorm --format json
# After the handoff output has been written, use workflowRun.runId from the JSON result:
station workflow verify brainstorm <run-id> --format json
```

`workflow run` prepares the workflow instruction, permitted input context, handoff information, and a run ID; it does not invoke an agent or model. Text output includes instruction/input content, while JSON output reports context metadata without embedding the full instruction or input contents. Preparation records local audit and run-state files under `.station/`. `workflow verify` checks that run's declared outputs and observable workspace changes, then records completion. It requires the run ID from the corresponding preparation.

With `--command`, `workflow run` may execute a command only when its ID is declared in `commands` and allowed by that workflow's `allowedCommands`, subject to execution policy. The executable and arguments are run directly (not through a shell); shell executables are rejected. The generated manifest declares no commands, so this option has nothing to run until an operator deliberately configures it.

## Creating a workspace

Run `station new` in an existing, empty destination directory. It refuses a non-empty directory and does not overwrite existing entries. It writes the idea, `workspace.json`, the embedded workspace schema, workflow instruction files, root and app-area README/AGENTS guidance, and local skills for the app areas and ADR authoring. These are a contract and guidance scaffold, not application source code: no Expo/Vite or other project generator runs; no package manifests, dependencies, `node_modules`, Go module, shared package, or Git repository are created. In particular, the current `station new` command has no module-path flag or prompt and leaves out `apps/server/go.mod`.

Scaffold files are written one at a time. The scaffold API can emit `apps/server/go.mod` only when supplied an explicit module path, but `station new` currently has no option for that value and therefore omits the file. Choose a module path in an approved design rather than inventing a publishable path.

If filesystem I/O fails partway through scaffold creation, already-created files may remain; Station does not roll them back, so inspect and recover the partial destination manually before retrying.

## `workspace.json` and policy

`workspace.json` is validated against the embedded schema (`internal/assets/workspace.schema.json`) and semantic policy checks. Its required top-level fields are `schemaVersion`, `workspace`, `projects`, `commands`, `policies`, `workflows`, and `externalRepositories`; `metadata` is optional. `station validate` checks the manifest without writing files. After changing policy, run it from the workspace root.

The generated policy is conservative: execution defaults to `ask`; data defaults to `ask`, with `design` allowed and `secret` denied (unknown data is also `ask`); external changes default to `deny`. No external repositories are initially configured. If one is later configured, reads require both its `readOnly: true` setting and declared repository/workflow read paths; external writes are denied.

The generated `workflows.build` object starts with this scope (excerpt from `workspace.json`):

```json
{
  "requiredSkills": [],
  "allowedSkills": ["station-mobile", "station-web", "station-server"],
  "allowedCommands": [],
  "readPaths": ["docs/station/**"],
  "writePaths": ["docs/station/plan.md"],
  "inputs": ["docs/station/design.md"],
  "outputs": ["docs/station/plan.md"],
  "summaries": [],
  "handoff": "docs/station/plan.md + paths alterados + design.md"
}
```

Thus the first build run reads the approved design and writes its plan artifact; it cannot read or write application source. After the plan itself is approved, add it to `inputs`, then make the smallest necessary changes to `readPaths`, `writePaths`, and, if needed, `allowedCommands`; each command ID must also have a matching top-level `commands` entry. Run `station validate` before the implementation run. Validation checks schema, paths, references, and policy structure; it does not attest that design/plan approval happened or sandbox tools outside Station.

## Inspection and limits

`station inspect` is read-only and never runs project commands. Without `workspace.json`, it inventories immediate-root entries only (up to its entry limit) and reads only a fixed allowlist of small root metadata files such as `README.md`, `AGENTS.md`, `go.mod`, and `package.json`; it does not recursively inspect source files. With a valid manifest, it reports projects and entries under declared workflow `readPaths`, without reading application file contents. `workspace.json` itself is read and validated. The implementation currently reads that manifest without an explicit byte limit.

Station itself makes no model/provider calls and does not clone/fetch, push, open PRs, deploy, or write to external repositories. However, host tools are not sandboxed or intercepted by the CLI, and local skills/instructions being present does not prove the host loaded or invoked them. Postflight verification is best-effort: it reports observed out-of-scope changes rather than preventing or undoing them, and filesystem changes that preserve both size and modification time may be missed when Git evidence is unavailable. An explicitly permitted subprocess runs with the operator's OS privileges and may itself have effects beyond Station's local operations.

See `internal/cli/` for command parsing, `internal/modes/existing.go` for inspection behavior, and `internal/scaffold/monorepo.go` for generated files and default policy.
