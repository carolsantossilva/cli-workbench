# Workspace instructions

- Start Codex from the workspace root. This workspace does not initialize Git, so do not assume a Git project root exists.
- Read `workspace.json` and the relevant `apps/<area>/AGENTS.md` before changing mobile, web, or server files.
- Keep ownership within the selected project. Cross-project or shared changes need an approved design and explicit `writePaths` in the workflow contract.
- Run only command IDs present in `workspace.json` and allowed by the active workflow. The initial command lists are empty; do not install dependencies or invoke generators until a command is explicitly declared.
- Workflows and local skills are versioned guidance. Their presence does not prove a host loaded or invoked them.
- Do not send workspace data to providers or make remote changes. Workbench prepares local context but cannot control independent host tools.
- Keep the initial scaffold minimal. Do not create product requirements or application code before the brainstorm, grill-me, and approved design handoffs.
