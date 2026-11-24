---
name: workbench-server
description: Use when planning or changing the Go server project in this Workbench monorepo; verify the module/toolchain and keep changes within server ownership.
---

# Workbench server maintenance

1. Read the root `AGENTS.md`, `apps/server/AGENTS.md`, and `workspace.json`.
2. Inspect `go.mod`, `go.work`, and the installed Go version before relying on a module or toolchain. This scaffold intentionally has no `go.mod`; do not invent a module path.
3. Use only command IDs present in `workspace.json` and allowed by the active workflow. The initial command lists are empty; do not run `go mod init`, `go get`, downloads, or generators implicitly.
4. Keep ownership in `apps/server`. Changes to mobile, web, or shared contracts require an approved design and declared workflow write paths.
5. Keep handlers and transport boundaries testable, validate input at the boundary, and record exact verification results without claiming host tools are sandboxed.
