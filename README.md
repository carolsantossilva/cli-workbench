# Workbench

A Workbench é um agente local de desenvolvimento. Ela roda tarefas e workflows (`brainstorm → grill-me → design → build → review`) num projeto, com o SDK do [Pi](https://pi.dev/), e aplica um sandbox de permissões antes de cada ação do modelo.

A direção está no [ADR 0001](docs/adr/0001-workbench-como-host-de-agente.md), e a especificação da V1 está no [`SPEC.md`](SPEC.md).

## Onde está cada coisa

| Pasta | O que tem |
|---|---|
| [`host/`](host/README.md) | a CLI `wb` em TypeScript: uso, sandboxes, workflows, garantias e limites |
| [`docs/adr/`](docs/adr/README.md) | decisões de arquitetura |
| [`docs/workbench/`](docs/workbench/) | backlog e os documentos de exploração do host (`pi-host/`) |
| [`templates/`](templates/) | templates e skills do antigo scaffold `workbench new` (monorepo mobile/web/server), guardados para um futuro comando de criação de projeto |
| [`spikes/pi-host/`](spikes/pi-host/README.md) | spike de outubro/2026 que validou o host; registro histórico |

## Começar

```sh
cd host
npm install
npx pi auth login openai-codex          # uma vez: login com a sua conta do ChatGPT
node src/cli.ts help
```

Requer Linux (o sandbox usa `bubblewrap`) e Node.js 24 ou mais.

## Sobre `templates/`

Os arquivos de `templates/` ainda descrevem a versão anterior: citam `workspace.json` e os comandos `workbench workflow run/verify`. Eles foram guardados sem mudanças, como ponto de partida. Antes de virarem a base de um comando `wb new`, precisam ser adaptados ao host.

## Histórico

Até novembro de 2026, a Workbench era uma CLI em Go que só orientava um agente externo (Codex-first): validava um `workspace.json` e preparava e conferia handoffs de workflow. Essa CLI, o `plan.md` dela e o `design.md` Codex-first foram removidos (a especificação atual está no `SPEC.md`) e continuam no histórico do git.
