# Review workflow

## Goal

Assess the proposed diff against the approved design, plan, tests, workspace contract, data policy, and external-change policy.

## Inputs and output

- Review only the declared diff/snapshot, design, plan, test results, and relevant decisions.
- Report actionable findings with severity, affected path, concrete impact, and evidence. Separate verified defects from limitations and assumptions.
- Record review findings and disposition in `docs/workbench/decisions.md` as allowed by the manifest.

## Guardrails

- Do not modify code as part of review, execute undeclared commands, or claim unrun tests passed.
- Never push, open a PR, deploy, publish, or write to an external repository unless a later explicit policy and workflow authorize it.
- Do not treat skills as a technical sandbox or claim that Workbench prevents writes made by host tools.
