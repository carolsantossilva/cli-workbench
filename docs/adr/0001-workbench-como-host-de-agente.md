# 0001 — Workbench como host de agente, construído sobre o Pi

- **Status:** proposed
- **Escrito em:** 2026-10-18
- **Direção escolhida em:** 2026-10-18, no grill-me de `docs/workbench/pi-host/` (decisão D1), pelo desenvolvedor do projeto. Este ADR ainda não foi aceito.
- **Escopo:** papel da Workbench na V1 e suas fronteiras com o agente e com o provedor de modelos.
- **Implementação:** nenhuma. Existe só um spike descartável em [`spikes/pi-host`](../../spikes/pi-host/), fora do código de produção, sem verificação além da descrita em "Evidência".

## Contexto

O `design.md` define a Workbench como uma CLI que orienta um agente externo. Ela é "Codex-first", não chama modelos (§8), e "o agente existente continua responsável por raciocinar, chamar ferramentas e executar as mudanças" (§1). A implementação atual segue isso: `workflow run` prepara o contexto, `workflow verify` confere o resultado e, entre os dois, o host faz o que quiser. O próprio README admite as consequências: "host tools are not sandboxed or intercepted by the CLI", e a verificação posterior é best-effort.

Ao mesmo tempo, o `design.md` §7 exige coisas que só quem controla o loop do agente consegue garantir:
- registro local do que foi enviado a um provedor;
- lista de provedores permitidos;
- detecção e mascaramento de segredos antes do envio;
- distinção aplicada entre dados permitidos e proibidos.

O §7 também prevê que "quando o host do agente permitir, a Workbench deverá complementar as regras com comandos, hooks ou verificações determinísticas locais".

O [Pi](https://pi.dev/) é um harness de agente mínimo, com licença MIT, em TypeScript, com SDK e modo RPC. É a base do OpenClaw. Ele expõe os pontos de controle necessários:
- `tool_call`, que pode bloquear;
- `tool_result`, que pode reescrever o resultado;
- `before_agent_start`, que define o system prompt;
- `context` e `before_provider_request`, que veem o que vai ao modelo;
- operações plugáveis para todas as ferramentas embutidas, inclusive o bash.

O spike mostrou que esses pontos bastam para aplicar o escopo de um workflow no momento da ação, usando o JSON que a CLI atual já emite.

## Decisão

A Workbench passa a ser o **agente**, e não só o orientador de um agente externo. O caminho principal da Workbench é um host próprio, construído sobre o SDK do Pi, que:

1. obtém do núcleo Go o run preparado (`workflow run`) e conclui com `workflow verify`;
2. monta o contexto de cada etapa a partir da instrução e dos inputs declarados, numa sessão própria por etapa;
3. aplica o escopo do workflow (leitura, escrita, comandos, provedores) antes de cada ação, em vez de só reportá-lo depois;
4. audita localmente cada envio ao provedor;
5. faz a chamada ao modelo usando um provedor configurado pelo usuário.

O núcleo Go continua sendo a fonte única do contrato: schema, validação, preparação, verificação e auditoria. O host consome esse contrato e não o redefine.

## Alternativas consideradas

- **Manter Codex-first, com o host Pi como segundo caminho.** Isso preservaria o `design.md` atual, mas as garantias da §7 continuariam opcionais e o caminho principal continuaria sem elas. Rejeitada no grill-me.
- **Extensão avulsa do Pi, fora do produto.** É a opção mais barata, mas não muda as garantias da Workbench. Rejeitada no grill-me.
- **Hooks de outros hosts (Codex, Claude Code).** Não foram avaliados em detalhe. A cobertura varia por host, e nenhum deles dá controle do payload enviado ao provedor. Isso continua como possível modo degradado (ver "Em aberto").
- **Escrever um harness próprio do zero.** Daria controle total, mas exigiria reimplementar loop, sessões, compactação, TUI e provedores. O Pi entrega isso com licença MIT e com a possibilidade de fork.

## Consequências

**Mudanças no `design.md` e no `plan.md`, se este ADR for aceito:**
- §8 "Codex-first" e "Sem chamadas próprias a modelos ou APIs de IA" deixam de valer para o caminho principal.
- §1: a Workbench passa a ser responsável pelo loop do agente. O raciocínio continua sendo do modelo.
- As restrições globais do `plan.md` que citam esses princípios precisam ser revistas.
- O texto desses documentos não deve ser alterado antes da aceitação.

**Positivas:**
- A §7 passa a ser cumprível de fato: auditoria de envio, provedores permitidos e redação antes do envio. O spike demonstrou isso com um modelo roteirizado.
- `readPaths`/`writePaths` passam a ser aplicados antes da ação, e o postflight vira confirmação, não a única defesa.
- O contexto mínimo por etapa é imposto pelo host, não pedido ao agente.

**Negativas e custos:**
- O Node.js passa a ser dependência do caminho principal. A CLI Go hoje não tem nenhuma.
- A Workbench passa a responder por autenticação, custo de modelo, UX do TUI e qualidade do agente. Antes, tudo isso era do Codex.
- Dependência do roadmap da Earendil (o Pi está na versão 1.3.0 hoje), mitigada pela licença MIT.
- Custo do modelo: créditos do Claude Max não podem ser usados em harnesses de terceiros. Para o Pi usar a assinatura do ChatGPT, ainda falta confirmar os termos de uso. A estimativa de custo por API está em `docs/workbench/pi-host/decisions.md` (D2) e ainda não foi medida.
- O Pi não tem permissões nativas. Sem a camada de política da Workbench, ele é menos restritivo que o Codex, então essa camada não é opcional.

**Decisões derivadas.** São direções escolhidas no grill-me, não aceitas, e cada uma pede um registro próprio quando for fechada:
- D2: provedor e modelo configuráveis, começando por API key via variável de ambiente.
- D3: bash livre dentro de um sandbox derivado do escopo do workflow.
- D4: o núcleo Go compila o escopo em regras normalizadas que o host e o sandbox aplicam.

## Em aberto antes da aceitação

- O Codex e o Claude Code continuam suportados em modo orientado (degradado) ou deixam de ser suportados? Isso define se `workflow run/verify` continua como interface pública estável para outros hosts.
- Empacotamento e distribuição: um artefato ou dois, e como o binário Go encontra ou instala o host.
- Plataformas da V1: o sandbox da D3 é direto no Linux (`bubblewrap`), mas não no macOS.

## Evidência

O spike [`spikes/pi-host`](../../spikes/pi-host/README.md) (README, 2026-10-18, Pi 1.3.0, provider `faux`, sem rede) rodou o workflow `brainstorm` num workspace gerado por `workbench new`. O modelo roteirizado tentou ler fora de `readPaths`, sair da raiz, passar por um symlink, escrever fora de `writePaths` e usar bash. Todas as tentativas foram bloqueadas. Uma chave falsa num input permitido foi redigida antes de chegar ao modelo, e `workbench workflow verify` retornou `completed`.

O spike não testou um modelo real, o TUI, a política `ask` nem o sandbox.

## Referências

- `design.md` §1, §7, §8; `docs/workbench/plan.md` (restrições globais)
- `docs/workbench/pi-host/brief.md` e `decisions.md`
- [Pi](https://pi.dev/) · [Armin Ronacher sobre o Pi e o OpenClaw](https://lucumr.pocoo.org/2026/1/31/pi/)
