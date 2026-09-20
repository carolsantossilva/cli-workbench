---
name: station-mobile
description: Use when planning or changing the mobile project in this Station monorepo; inspect its real toolchain and keep changes within mobile ownership.
---

# Station mobile maintenance

1. Read the root `AGENTS.md`, `apps/mobile/AGENTS.md`, and `workspace.json`.
2. Inspect the existing mobile files before assuming React Native, Expo, package-manager, or TypeScript versions. The scaffold does not install or prove any tool is available.
3. Use only command IDs present in `workspace.json` and allowed by the active workflow. The initial command lists are empty; do not run `npm`, `npx`, Expo, or another generator unless a later explicit command declaration permits it.
4. Keep ownership in `apps/mobile`. Changes to web, server, or shared contracts require an approved design and declared workflow write paths.
5. For UI work, preserve platform conventions and check accessible labels, focus/interaction, and device-specific behavior relevant to the change. Record tests actually run; do not claim host enforcement from these instructions.
