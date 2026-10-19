# Spike: Workbench como host baseado no Pi

Descartável, não é código de produção. Pergunta respondida: *o SDK do Pi permite que a Workbench aplique o contrato de um workflow no momento da ação, mantendo o núcleo Go como fonte de verdade?* Resposta: **sim, para o workflow `brainstorm`, com um modelo roteirizado.**

Contexto: [`docs/workbench/pi-host/brief.md`](../../docs/workbench/pi-host/brief.md) e [ADR 0001](../../docs/adr/0001-workbench-como-host-de-agente.md).

## Arquivos

- `policy.ts`: checagens puras: path dentro da raiz (com realpath contra symlink), globs de `readPaths`/`writePaths` e redação de segredos.
- `workbench-host.ts`: extensão do Pi. Chama `workbench workflow run/verify --format json`, injeta a instrução e o escopo no system prompt, bloqueia tool calls fora do escopo, redige segredos nos resultados de ferramentas e audita cada request em `.workbench/pi-host/sends.jsonl` (hash e tamanho, nunca o conteúdo). Funciona via SDK e via `pi -e`.
- `spike-faux.ts`: roda de ponta a ponta com o provider `faux` do Pi. Não usa chave, rede nem custo.

## Rodar

```sh
npm install --ignore-scripts
go build -o /tmp/wb-pi-spike-bin ../../cmd/workbench
node spike-faux.ts /tmp/wb-pi-spike-bin /tmp/wb-pi-spike
```

Interativo, com um modelo real (não testado neste spike):

```sh
cd <workspace criado com workbench new>
WORKBENCH_BIN=/caminho/workbench npx pi -e /caminho/workbench-host.ts
# no TUI: /login, depois /wb brainstorm ... /wb-verify
```

## Resultado (2026-10-18, Pi 1.3.0)

O modelo roteirizado tentou:

| Tentativa | Resultado |
|---|---|
| ler `docs/workbench/idea.md`, `notes.md` | permitido |
| ler `apps/web/README.md` | bloqueado: fora de `readPaths` |
| ler `../../../../etc/hostname` | bloqueado: fora da raiz |
| ler `docs/workbench/link.md` (symlink para `apps/web`) | bloqueado: o realpath sai de `readPaths` |
| chave falsa `sk-ant-…` dentro de `notes.md` | redigida antes de entrar no transcript; **não chegou ao modelo** |
| escrever `apps/web/App.tsx`, `workspace.json` | bloqueado: fora de `writePaths` (os arquivos não existem / estão intactos) |
| `bash curl -d @notes.md …` | bloqueado: bash livre fora do contrato |
| escrever `docs/workbench/brief.md` | permitido |

Depois disso, `workbench workflow verify brainstorm <run>` retornou `completed`. O audit registrou 3 requests (`faux/faux-1`, bytes e sha256), e a escrita em `.workbench/pi-host/` não foi tratada como mudança fora de escopo.

## O que o spike NÃO prova

- **Modelo real:** só o provider `faux` foi exercitado. `before_provider_request` não dispara para ele, então a redação no nível do payload HTTP não foi testada. A auditoria via evento `context` foi testada.
- **TUI:** a extensão carrega no CLI (`pi -e … --list-models` sem erro), mas `/wb` não foi usado interativamente.
- **Execução de comandos:** bash é sempre bloqueado. Comandos declarados deveriam passar por `workbench workflow run --command`, o que não foi implementado.
- **Política `ask`:** não foi implementada (seria `ctx.ui.confirm`). Tudo fora do escopo é `deny`.
- **Skills:** desligadas (`noSkills`). Não houve filtragem por `allowedSkills`.
- **Duplicação:** a checagem de globs foi reimplementada em TS (`path.matchesGlob`) e pode divergir da semântica do núcleo Go.
- Redação de segredos por regex é heurística (5 padrões).
- `grep`/`find` sobre diretórios são bloqueados, porque um diretório não casa com `dir/**`. Isso é aceitável no spike e precisa ser decidido no design.

## Achados sobre o Pi

- `tool_call` → `{ block, reason }`; o motivo volta ao modelo como erro da ferramenta. Uma falha no handler bloqueia (fail-safe).
- `tool_result` pode substituir o conteúdo, e é o melhor ponto para redigir segredos: eles não são persistidos nem enviados.
- `before_agent_start` → `systemPrompt` substitui o prompt do turno.
- `context` dispara para qualquer provider; `before_provider_request` vê o payload real (só providers HTTP).
- Há login OAuth para assinatura ChatGPT Plus/Pro (`openai-codex`) em `pi-ai/dist/auth/oauth/openai-chatgpt.js`. A política da OpenAI para esse uso não foi verificada. Créditos do Claude Max não podem ser usados em harnesses de terceiros.
