# Decisões — Workbench como host baseado no Pi (grill-me)

Status: grill-me, rodada 1 (2026-10-18). Entradas: `brief.md` e o spike em [`spikes/pi-host`](../../../spikes/pi-host/) (README com os resultados). Estas decisões de direção ainda não são design aprovado. A D1 contraria o `design.md` e precisa de um ADR antes de qualquer implementação.

## D1 — Papel do host: caminho principal

**Decisão:** o host baseado no Pi passa a ser o caminho principal da Workbench. A Workbench *é* o agente.

**Rejeitadas:** segundo caminho, com o Codex continuando como principal; extensão avulsa do Pi, fora do produto.

**Consequências:**
- Reverte dois princípios do `design.md` §8: "Codex-first" e "Sem chamadas próprias a modelos ou APIs de IA". Também afeta §1 ("O agente existente continua responsável por raciocinar…") e as restrições globais do `plan.md`. **Exige um ADR.**
- A §7 (governança de dados) passa a ser cumprível de fato: registro de envio, provedores permitidos e mascaramento antes do envio.
- O núcleo Go continua sendo a fonte do contrato (ver D4). O host em TS vira um componente de primeira classe, e o Node passa a ser dependência do caminho principal.
- O produto passa a responder por login, custo, UX do TUI e qualidade do agente.

**Em aberto:**
- O Codex/Claude Code continua suportado em modo orientado (degradado) ou deixa de ser? Isso define se `workflow run/verify` como CLI pública continua sendo uma promessa.
- Empacotamento: um binário só (Go embute/instala o host) ou dois artefatos?

## D2 — Modelo e custo: configurável; começar pelo mais rápido

**Decisão:** o provedor e o modelo são configuráveis, idealmente por workflow no `workspace.json`. Para começar, usar o caminho mais rápido de implementar: **API key por variável de ambiente**.

**Por quê é o mais rápido:** o Pi já resolve os dois métodos de autenticação (`ModelRuntime`: env/API key e `/login` OAuth), então "configurável" sai quase de graça. A API key funciona headless, não depende de fluxo de navegador e não tem risco de termos de uso. O OAuth pode ser ligado depois sem mudar a arquitetura.

**Comparação de custo (a pedido): OAuth com assinatura vs. API key**

Preços por 1M tokens (consultados em 2026-10-18):

| Modelo | Input | Cache read | Output |
|---|---|---|---|
| Claude Opus 5.5 | $4.00 | $0.20 | $20.00 |
| Claude Sonnet 5.5 | $2.00 | $0.20 | $10.00 |
| Claude Haiku 5.5 | $0.10 | — | $0.50 |
| OpenAI gpt-6.1-sol | $2.00 | $0.10 | $10.00 |
| OpenAI gpt-5.3-codex | $1.75 | $0.175 | $14.00 |

Fontes: tabela de preços Anthropic (referência da skill claude-api, cache de 2026-10-06; escrita em cache ≈ 1,25× input); [OpenAI pricing](https://developers.openai.com/api/docs/pricing). Os números da OpenAI vieram de busca e precisam ser confirmados na página.

Premissas de uso. São **estimativas, não medições**. Num loop de agente, cada request reenvia o contexto, e cerca de 90% do input sai do cache:
- workflow de documento (brainstorm, grill-me, design): ~15 requests, ~400k tokens de input acumulado, ~15k de output;
- lote de build: ~60 requests, ~4M de input acumulado, ~60k de output;
- ciclo completo: 4 workflows de documento + review + ~5 lotes de build.

| Por API key | Doc workflow | Lote de build | Ciclo completo |
|---|---|---|---|
| Sonnet 5.5 | ~$0.32 | ~$2.30 | ~$14 |
| Opus 5.5 | ~$0.57 | ~$3.90 | ~$24 |
| gpt-6.1-sol | ~$0.27 | ~$1.80 | ~$11 |

Uso intenso (3 a 5 lotes de build por dia útil): **~$150–250/mês com Sonnet 5.5**, **~$260–430/mês com Opus 5.5**. Tokens de raciocínio contam como output e podem aumentar isso.

| Por assinatura (OAuth) | Preço | Funciona no Pi? |
|---|---|---|
| ChatGPT Plus | $20/mês, com janelas de 5h | Tecnicamente sim: o Pi tem OAuth `openai-codex` (`pi-ai/dist/auth/oauth/openai-chatgpt.js`). **Mas o Pi não está entre os parceiros do "Sign in with ChatGPT"** ([anúncio](https://daily.dev/posts/openai-makes-sign-in-with-chatgpt-a-way-to-use-your-subscription-in-third-party-developer-tools-7ewaoo6ze)), e o login dele é do tipo informal que o OpenClaw usava. Risco de termos. |
| ChatGPT Pro | $100–200/mês (fontes divergem) | Mesmo risco |
| Claude Pro/Max | $20–200/mês | **Não.** Créditos do Max não podem ser usados em harness de terceiros; o uso no Pi é cobrado pela API ([fonte](https://yage.ai/share/pi-coding-agent-locked-out-en-20260518.html)) |

**Conclusão:** com uso intenso, a assinatura do ChatGPT sairia de 2 a 10 vezes mais barata que a API, *se* for permitida. Hoje isso não está garantido para o Pi. Com uso leve (alguns ciclos por mês), a API custa menos de $50/mês e não tem risco nenhum. Por isso: API key primeiro, OAuth configurável depois que a questão de termos estiver resolvida.

**Em aberto:**
- Medir o custo real: o audit do host já registra bytes por request; falta somar `usage` por run. A estimativa acima deve ser substituída por uma medição do primeiro dogfooding.
- O formato no `workspace.json`: `workflows.<id>.model: "provider/model"`, mais uma lista de provedores permitidos em `policies.data`.

## D3 — Execução de comandos: bash livre em sandbox

**Decisão:** o agente tem bash livre, mas sempre dentro de um sandbox cujo filesystem e rede refletem o escopo do workflow.

**Rejeitadas:** só comandos declarados, delegados a `workbench workflow run --command` (restritivo demais para build); bash com confirmação por comando (depende de atenção humana constante).

**Consequências:**
- O sandbox passa a ser a garantia principal para bash. Os checks em `tool_call` continuam valendo para as ferramentas de arquivo.
- Achado do spike: as ferramentas embutidas do Pi têm *operações plugáveis* (`WriteOperations`, `ReadOperations`… "delegate to remote systems such as SSH"). Dá para fazer **read/write/edit também executarem dentro do sandbox**, com uma única fronteira para tudo.
- Precisa de tecnologia por SO. No Linux, `bubblewrap` (sem daemon, rápido). No macOS não há equivalente direto: `sandbox-exec` está deprecado; a alternativa é um container ou uma VM. Isso conflita com o público "desenvolvedor individual" se o macOS for prioridade.
- Rede: decidir se o padrão é desligado. `go mod download` e `npm install` precisam de rede, e o `design.md` §7 trata isso como mudança externa ou dado.
- Variáveis de ambiente e segredos do usuário não podem vazar para o sandbox (env limpo e allowlist).

**Em aberto:**
- Linux-only na primeira versão é aceitável?
- Política de rede: bloqueada por padrão, com allowlist de hosts por workflow?
- Os `commands` declarados no `workspace.json` continuam existindo (para `verify`, CI e outros hosts) ou viram só documentação?

## D4 — Onde mora a política em runtime: regras compiladas

**Decisão:** o núcleo Go compila o escopo do workflow em um conjunto de regras normalizadas. O host só aplica essas regras e não reimplementa a semântica de globs.

**Rejeitadas:** o host em TS reimplementar a checagem (como no spike, com risco de divergência); uma chamada ao núcleo Go por tool call (latência, processo extra).

**Consequências:**
- Novo artefato do `workflow run`: por exemplo, `workflowRun.rules` com paths normalizados e realpath, padrões já traduzidos (regex ou lista de prefixos), modo (`allow`/`ask`/`deny`) por ação, comandos e hosts de rede.
- **As mesmas regras alimentam o sandbox da D3.** As regras de escrita viram bind mounts de leitura e escrita, as de leitura viram mounts somente leitura e o resto não existe dentro do sandbox. É a mesma fonte para as duas camadas.
- Globs que não são prefixos de diretório (`**/*.md`, por exemplo) não viram mounts diretamente. Ou o schema passa a restringir `writePaths` a prefixos e arquivos, ou o compilador expande e reporta.
- Versionar o formato das regras (`rulesVersion`), porque host e núcleo podem estar em versões diferentes.

**Em aberto:**
- As regras são calculadas uma vez por run ou recompiladas quando o `workspace.json` muda no meio do run? (Hoje `workflow run` grava um snapshot.)

## Próximo passo sugerido

1. ADR registrando a D1 e revertendo os princípios do §8, com D2–D4 como consequências: escrito como [`docs/adr/0001`](../../adr/0001-workbench-como-host-de-agente.md), status `proposed`.
2. Rodada 2 do grill-me com os itens "Em aberto" acima, principalmente suporte a macOS × sandbox e o destino do modo Codex.
3. Só depois disso, o workflow `design`.
