---
name: station-web
description: Use when planning or changing the web project in this Station monorepo; inspect its real toolchain and keep changes within web ownership.
---

# Station web maintenance

1. Read the root `AGENTS.md`, `apps/web/AGENTS.md`, and `workspace.json`.
2. Inspect existing manifests and source before assuming React, a bundler, package manager, or TypeScript version. The scaffold does not install or prove any tool is available.
3. Use only command IDs present in `workspace.json` and allowed by the active workflow. The initial command lists are empty; do not run package installers, `npx`, or generators unless a later explicit command declaration permits it.
4. Keep ownership in `apps/web`. Changes to mobile, server, or shared contracts require an approved design and declared workflow write paths.
5. Prefer semantic browser controls, keyboard operation, and responsive behavior. Verify accessibility and tests relevant to the changed interaction, and report only commands actually run.
