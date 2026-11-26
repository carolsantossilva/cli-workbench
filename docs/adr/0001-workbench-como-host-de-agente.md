# 0001 — Workbench como host de agente, construído sobre o Pi

- **Status:** accepted
- **Escrito em:** 2026-10-18; revisado em 2026-11-24, antes do aceite, para refletir as decisões tomadas durante o micro MVP.
- **Direção escolhida em:** 2026-10-18, no grill-me de `docs/workbench/pi-host/` (decisão D1), por carolsantossilva. Ajustada em 2026-11-23 (host autônomo em TypeScript, sem o núcleo Go no caminho).
- **Aceito em:** 2026-11-24, por carolsantossilva.
- **Escopo:** papel da Workbench na V1 e suas fronteiras com o agente e com o provedor de modelos.
- **Implementação:** micro MVP em [`host/`](../../host/README.md) (PR #5). O spike de 2026-10-18 continua em [`spikes/pi-host`](../../spikes/pi-host/) como registro.

## Contexto

O `design.md` define a Workbench como uma CLI que orienta um agente externo. Ela é "Codex-first", não chama modelos (§8), e "o agente existente continua responsável por raciocinar, chamar ferramentas e executar as mudanças" (§1). A implementação em Go segue isso: `workflow run` prepara o contexto, `workflow verify` confere o resultado e, entre os dois, o host faz o que quiser. O próprio README admite as consequências: "host tools are not sandboxed or intercepted by the CLI", e a verificação posterior é best-effort.

Essa regra vinha do fluxo de trabalho da época, com o Codex como agente, e não de um requisito do produto. Ao mesmo tempo, o `design.md` §7 exige coisas que só quem controla o loop do agente consegue garantir:
- registro local do que foi enviado a um provedor;
- lista de provedores permitidos;
- detecção e mascaramento de segredos antes do envio;
- distinção aplicada entre dados permitidos e proibidos.

O [Pi](https://pi.dev/) é um harness de agente mínimo, com licença MIT, em TypeScript, com SDK e modo RPC. É a base do OpenClaw. Ele expõe os pontos de controle necessários:
- `tool_call`, que pode bloquear;
- `tool_result`, que pode reescrever o resultado;
- `context` e `before_provider_request`, que veem o que vai ao modelo;
- definições de ferramenta substituíveis, inclusive o `spawnHook` do bash.

O spike de outubro mostrou que esses pontos bastam para aplicar um escopo no momento da ação. O MVP de novembro mostrou o mesmo com um modelo real e com sandbox.

## Decisão

A Workbench passa a ser o **agente**, e não só o orientador de um agente externo. O caminho principal é um host próprio em **TypeScript, sobre o SDK do Pi**, distribuído como um artefato só, que:

1. **Roda a tarefa ou a etapa de workflow ele mesmo**, numa sessão própria por etapa. O contexto passa de uma etapa para a outra só pelos arquivos declarados (`docs/workbench/`).
2. **Aplica o escopo antes de cada ação** (leitura, escrita, comandos). O escopo vem de três fontes:
   - sandboxes prontos (`read-only`, `docs-only`, `workspace-write`);
   - detecção automática dos comandos do projeto;
   - exceções pequenas no `.workbench.json` (`sandbox`, `deny`, `write`).

   Não há um `workspace.json` escrito à mão.
3. **Roda o bash num sandbox** (`bubblewrap`). Fica sem rede, sem a home e com o sistema só leitura. Arquivos sensíveis, `.git` e o estado da Workbench também são protegidos.
4. **Audita localmente** cada decisão e cada envio ao provedor: modelo, bytes e sha256, nunca o conteúdo. Segredos são redigidos antes do envio.
5. **Chama o modelo pelo provedor configurado pelo usuário** via Pi. O padrão é o login do ChatGPT, com `openai-codex/gpt-6-luna` e raciocínio `xhigh`.

O núcleo Go é **removido** do repositório; ele continua no histórico do git. Os templates e as skills do scaffold (`workbench new`) são guardados em `templates/`, sem código, para o host reaproveitar quando ganhar um comando de criação de projeto.

## Alternativas consideradas

- **Manter Codex-first, com o host Pi como segundo caminho.** Preservaria o `design.md` atual, mas as garantias da §7 continuariam opcionais e o caminho principal continuaria sem elas. Rejeitada no grill-me.
- **Host Pi consumindo o núcleo Go** (`workflow run`/`verify` como contrato e escopo compilado pelo Go). Era a primeira versão deste ADR. Rejeitada em 2026-11-23: manteria dois runtimes e um `workspace.json` escrito à mão, sem ganho para o uso real. O Go tinha sido escolhido só por desempenho, o que não pesa num processo que passa a maior parte do tempo esperando o modelo.
- **Extensão avulsa do Pi, fora do produto.** É a opção mais barata, mas não muda as garantias da Workbench. Rejeitada no grill-me.
- **Hooks de outros hosts (Codex, Claude Code), ou mantê-los em modo orientado pela CLI Go.** A cobertura varia por host, e nenhum deles dá controle do payload enviado ao provedor. Manter a CLI Go só para isso significaria manter dois produtos. Rejeitada em 2026-11-24.
- **Escrever um harness próprio do zero.** Daria controle total, mas exigiria reimplementar loop, sessões, compactação, TUI e provedores. O Pi entrega isso com licença MIT e com a possibilidade de fork.

## Consequências

**Mudanças no repositório:**
- O `design.md` é reescrito para o host. §8 "Codex-first" e "Sem chamadas próprias a modelos ou APIs de IA" deixam de valer, e pela §1 a Workbench passa a ser responsável pelo loop do agente. O raciocínio continua sendo do modelo.
- O `docs/workbench/plan.md` (plano de implementação do Go) e o `workspace.json` saem junto com o Go.

**Positivas:**
- A §7 passa a ser cumprível de fato: auditoria de envio e redação antes do envio, verificadas com modelo real.
- O escopo é aplicado antes da ação. Ferramentas que o sandbox não permite nem são oferecidas ao modelo.
- O contexto mínimo por etapa é imposto pelo host, não pedido ao agente.
- O projeto não precisa de configuração: os sandboxes prontos e a detecção cobrem o caso comum.

**Negativas e custos:**
- O produto passa a depender do Node.js (24 ou mais).
- O Codex e o Claude Code deixam de ser suportados: a Workbench só funciona como o próprio agente.
- O `workbench new` (scaffold de monorepo) deixa de existir até o host ganhar um comando equivalente.
- A Workbench passa a responder por autenticação, custo de modelo, UX e qualidade do agente. Antes, tudo isso era do Codex.
- Dependência do roadmap da Earendil (o host usa o Pi 1.6.0), mitigada pela licença MIT.
- Na conta do ChatGPT testada (2026-11-24), o Codex só aceita os modelos Luna (`gpt-6-luna` e `gpt-5.6-luna`); Sol, Terra e Astra retornam "not supported when using Codex with a ChatGPT account". Isso pode variar com o plano. O `gpt-6-luna` recusa algumas tarefas comuns sem motivo real ("voting systems"). A palavra "evidência" no prompt dispara a recusa quase sempre, e o host contorna isso com uma troca de palavra e uma nova tentativa.
- A V1 é só Linux: o sandbox usa `bubblewrap`.
- O Pi não tem permissões nativas, então a camada de política da Workbench não é opcional.

**Decisões derivadas do grill-me**, no estado em que ficaram:
- **D2 (provedor e modelo):** configuráveis com `--model`. O padrão passou a ser o login do ChatGPT pelo Pi, em vez de API key por variável de ambiente.
- **D3 (bash livre dentro de um sandbox):** implementada com `bubblewrap`.
- **D4 (núcleo Go compila o escopo):** substituída pelos sandboxes prontos, pela detecção automática e pelas exceções do `.workbench.json`.

## Pontos que estavam em aberto

Decididos em 2026-11-24, antes do aceite:

- **Codex e Claude Code em modo orientado:** não são suportados. A CLI Go é removida (ver "Decisão").
- **Termos de uso do login do ChatGPT:** o login continua como padrão, com o risco registrado. Os termos da OpenAI não autorizam nem proíbem explicitamente o uso com harnesses de terceiros. Fontes secundárias relatam que a OpenAI tolera, mas a documentação dela recomenda API key para uso programático. Se a OpenAI restringir, a saída é trocar o padrão para um provedor com API key (`--model`), sem mudar o host.
- **macOS:** fora da V1. O caminho futuro é o sandbox nativo do macOS (Seatbelt, via `sandbox-exec`), o mesmo que o Codex usa.
- **Canal remoto** (por exemplo, WhatsApp): fora da V1. Pede um ADR próprio antes de ser implementado. Requisito já conhecido: nenhum fluxo pode juntar dados privados, entrada não confiável e saída para fora (a "lethal trifecta").

## Evidência

- **Spike** [`spikes/pi-host`](../../spikes/pi-host/README.md) (2026-10-18, Pi 1.3.0, provider `faux`, sem rede). Ele consome a CLI Go, que agora só existe no histórico do git, e fica como registro. Um modelo roteirizado tentou ler fora do escopo, sair da raiz, passar por symlink, escrever fora do escopo e usar bash, e tudo foi bloqueado. Uma chave falsa foi redigida antes de chegar ao modelo.
- **Micro MVP** [`host/`](../../host/README.md) (2026-11-23/24, Pi 1.6.0, PR #5). A suíte de testes usa o provider `faux` e cobre:
  - sandboxes e o sandbox do bash;
  - aprovação interativa e os cinco workflows;
  - redação de segredos;
  - fugas de `.git`, de symlink e de credenciais.

  Execuções com o `gpt-6-luna` real:
  - `read-only` explicou o código;
  - `docs-only` recusou `.env` e edições de código;
  - `workspace-write` corrigiu um bug e rodou os testes no sandbox, com a rede bloqueada;
  - o fluxo completo `brainstorm → review` rodou num projeto de teste. O build ficou barrado até a aprovação do design, e a review achou um defeito real.

  O hook de revisão antes do push encontrou cinco falhas de segurança, todas corrigidas com testes.

## Referências

- `design.md` §1, §7, §8 e `docs/workbench/plan.md`, na versão Codex-first (no histórico do git)
- `docs/workbench/pi-host/brief.md` (no histórico do git) e [`decisions.md`](../workbench/pi-host/decisions.md)
- [`host/README.md`](../../host/README.md)
- [Pi](https://pi.dev/) · [Armin Ronacher sobre o Pi e o OpenClaw](https://lucumr.pocoo.org/2026/1/31/pi/)
