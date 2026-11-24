# Workbench host (micro MVP)

A Workbench como **o agente**, e não só como orientadora de um agente externo. Segue a direção do [ADR 0001](../docs/adr/0001-workbench-como-host-de-agente.md). Ela roda uma tarefa num projeto local usando o SDK do [Pi](https://pi.dev/) e aplica um **sandbox (perfil de permissão) antes de cada ação**.

É a implementação da Workbench desde a remoção da CLI Go (ver o [README da raiz](../README.md)). Começou como experimento para validar três premissas, todas confirmadas no MVP:
- TypeScript como linguagem do host;
- sandboxes prontos com exceções, em vez de um `workspace.json` escrito à mão;
- sandbox no bash.

## Uso

```sh
npm install
npx pi auth login openai-codex          # uma vez: login com a sua conta do ChatGPT
cd ~/meu-projeto                        # ou -C ~/meu-projeto em qualquer comando
wb scope --sandbox workspace-write
wb run --sandbox docs-only "resuma a arquitetura em docs/arquitetura.md"
wb help                                 # comandos; wb help <comando> para cada um
```

(`wb` é o `bin` do pacote; sem instalar, use `node src/cli.ts` no lugar.)

Os nomes de comandos, flags e chaves seguem as convenções de CLI (clig.dev, GNU, POSIX), em inglês; as mensagens ficam em português. Flags podem vir em qualquer posição, aceitam `--flag=valor`, e `--` encerra as opções.

O modelo padrão é `openai-codex/gpt-6-luna`, com raciocínio `xhigh`. Para trocar, use `-m, --model provider/modelo` e `--thinking off|minimal|low|medium|high|xhigh|max` (mesmos nomes do Pi). O Pi ajusta o nível ao que o modelo suporta, e o nível efetivo fica na auditoria (evento `thinking`).

### Aprovação interativa

Quando o agente tenta ler ou escrever um arquivo fora do sandbox, a Workbench pergunta no terminal:

```
? O agente quer ESCREVER em src/soma.js (o sandbox "docs-only" não permite escrever em src/soma.js)
  [s] sim, desta vez   [t] sim, até o fim desta execução   [n] não  >
```

- `s` libera só aquela chamada. `t` libera aquele arquivo até o fim da execução. `n`, ou Enter, nega.
- Com `--no-input` (ou `WB_NO_INPUT=1`), ou sem terminal interativo, tudo fora do sandbox é negado sem perguntar.
- Regras fixas **nunca** viram pergunta: arquivos sensíveis, caminhos fora da raiz, `.git/` e a configuração da Workbench.
- Com a aprovação ligada, até o sandbox `read-only` recebe `write`/`edit`, e cada escrita vira pergunta. O bash continua só no `workspace-write`.
- Cada resposta fica registrada na auditoria (evento `approval`).

## Workflows

```sh
wb step run brainstorm "uma CLI para guardar tarefas em JSON"
wb step run grill-me
wb step run design
wb step approve design      # depois de revisar o design
wb step run build
wb step run review
wb status                   # onde está cada etapa e qual é o próximo passo
```

| Etapa | Lê | Escreve | Comandos | Pergunta ao usuário |
|---|---|---|---|---|
| `brainstorm` | `idea.md` (gravado a partir do texto passado) e o projeto | `brief.md` | — | sim |
| `grill-me` | `brief.md` | `decisions.md` | — | sim |
| `design` | `brief.md`, `decisions.md` e o projeto | `technical-design.md` | — | não |
| `build` | `technical-design.md` **aprovado** | `plan.md` e o projeto | sandbox | não |
| `review` | `technical-design.md`, `plan.md` e o projeto | `review.md` | sandbox **só leitura** | não |

Todos os arquivos ficam em `docs/workbench/`. Cada etapa roda numa sessão nova, então o contexto passa de uma etapa para a outra só pelos arquivos.

- **Entradas:** uma etapa não roda sem as entradas obrigatórias, e o erro diz qual etapa rodar antes.
- **Aprovação do design:** o `build` só roda com o design aprovado. `wb step approve` grava o sha256 do `technical-design.md` em `.workbench/state.json`. A pasta `.workbench/` tem um `.gitignore` próprio com `*`, então fica fora do git sem mexer no `.gitignore` do projeto. Se o design mudar depois, a aprovação deixa de valer e o build volta a ser barrado. Projetos com o `design.md` da versão anterior: renomeie para `technical-design.md` e aprove de novo.
- **Pedido extra:** nas etapas depois do brainstorm, o texto passado vira um pedido extra (ex.: `wb step run build "só o lote 2"`).
- **Perguntas:** no `brainstorm` e no `grill-me`, o agente usa a ferramenta `ask_user`. Com `--no-input`, ou sem terminal, ele decide com premissas explícitas.

## Sandboxes

| `--sandbox` | Lê | Escreve | Comandos |
|---|---|---|---|
| `read-only` (padrão) | o projeto | nada | nenhum |
| `docs-only` | o projeto | `**/*.md`, `docs/**` | nenhum |
| `workspace-write` | o projeto | o projeto | bash num sandbox `bwrap` |

`read-only` e `workspace-write` têm os mesmos nomes do `--sandbox` do Codex CLI.

O bash do `workspace-write`:
- não tem rede;
- esconde a home (só as ferramentas de `~/.nvm`, `~/go`, `~/.cargo` etc. ficam visíveis, em modo leitura, com credenciais como `~/.cargo/credentials.toml` mascaradas);
- deixa o sistema em modo leitura;
- mascara os arquivos sensíveis do projeto com `/dev/null`, inclusive dentro de pastas ocultas como `.config/`;
- monta `.git/` (e repositórios aninhados, inclusive o arquivo `.git` de submódulos), `.workbench/` e `.workbench.json` só leitura, para o bash não plantar hooks ou `core.fsmonitor` nem forjar a aprovação do design;
- roda com o ambiente limpo.

Em todos os sandboxes vale o seguinte:
- **Sempre negados:** `.env*`, `*.pem`, `*.key`, chaves SSH, `.npmrc`, `.netrc`, `auth.json`, `credentials*.json` e `.workbench/` (estado e auditoria do wb).
- **Nunca escritos:** `.git/`, `.workbench/` e `.workbench.json`, nem pelas ferramentas nem pelo bash. O que já existe é montado só leitura no sandbox. O que ainda não existe (um `.workbench.json`, ou um `.git` num projeto sem repositório ou numa subpasta) e aparece durante a execução é movido para `<caminho>.rejected-<run>` (evento `violation`), mesmo se o run terminar com erro ou Ctrl-C. Se o processo morrer de vez, a limpeza acontece no próximo uso do projeto. O `git status` do host roda sem fsmonitor e sem hooks.
- **Bloqueados:** caminhos fora da raiz, inclusive por symlink.

### Exceções por projeto (opcional)

`.workbench.json` na raiz do projeto:

```json
{ "sandbox": "docs-only", "deny": ["dados/**"], "write": { "docs-only": ["CHANGELOG"] } }
```

- `sandbox`: o padrão do `wb run` neste projeto (a flag `--sandbox` tem precedência).
- `deny`: globs negados além dos sempre negados.
- `write`: globs graváveis a mais, por sandbox.

As chaves antigas em português (`perfil`, `negar`, `perfis`) dão erro com a indicação das novas.

## O que a Workbench garante

- **Sandbox aplicado antes da ação:** a chamada bloqueada volta para o modelo como erro e não acontece.
- **Ferramentas que o sandbox não permite nem são oferecidas ao modelo.** Por exemplo, o `read-only` não recebe `write` nem `bash`.
- **Segredos redigidos** (chaves Anthropic, OpenAI, GitHub, AWS, Google, Slack e chaves privadas) antes de entrarem no transcript e no payload enviado ao provedor.
- **Comandos detectados** a partir de `package.json`, `go.mod` e `Cargo.toml`, listados para o modelo.
- **Auditoria** em `<projeto>/.workbench/runs/<run>.jsonl`, fora do git. Registra cada decisão e cada envio ao provedor (modelo, bytes e sha256, nunca o conteúdo), os tokens de cada resposta do modelo (evento `usage`: entrada, cache, saída, raciocínio e custo, cobrado ou estimado) com o total no evento `end`, além dos arquivos alterados segundo o git. O resumo no fim do `wb run`/`wb step run` mostra o total de tokens. O agente não lê `.workbench/`: trechos de runs anteriores não voltam ao modelo.

## Testes

```sh
npm test          # ponta a ponta com o provider faux do Pi (sem chave, sem rede)
npm run typecheck
```

## Limites conhecidos

- A redação de segredos é heurística (regex, aplicada a cada texto do que vai ao provedor). Um segredo fora desses padrões, num arquivo permitido, chega ao modelo.
- O filtro de `grep`/`find`/`ls` tira linhas de arquivos negados pelo nome, mas o `rg` já pula arquivos ocultos e o `.gitignore` por padrão.
- "Arquivos alterados" compara o `git status` de antes e de depois. Um arquivo que já estava modificado e foi alterado de novo não aparece.
- A aprovação vale por arquivo exato, não por pasta ou padrão, e não cobre comandos: no `workspace-write` o bash é livre dentro do sandbox.
- Ainda não há vários agentes em paralelo nem WhatsApp.
- Na conta do ChatGPT testada (2026-11-24), o Codex só aceita os modelos Luna (`openai-codex/gpt-6-luna` e `openai-codex/gpt-5.6-luna`). Sol, Terra e Astra retornam "not supported when using Codex with a ChatGPT account", e o erro aparece na hora. Isso pode variar com o plano do ChatGPT.
- O `gpt-6-luna` às vezes recusa uma tarefa comum, com justificativas sem relação com ela (ex.: "voting systems"). O host detecta a recusa e pede uma vez para continuar de onde parou (evento `refusal` na auditoria). Se recusar de novo, rode a etapa outra vez: o `plan.md` guarda o progresso. A palavra "evidência" num prompt dispara essa recusa quase sempre (medido: 6/6 com ela, 0/6 sem ela). O host troca "evidência(s)" por "prova(s)" no seu pedido antes de enviar (evento `prompt_rewrite` na auditoria); as instruções das etapas já não a usam.
- O sandbox só funciona no Linux (bubblewrap).
- As ferramentas `read`, `write`, `edit`, `grep`, `find` e `ls` rodam no host, fora do bwrap. A checagem resolve symlinks na hora, mas no `workspace-write` o bash ainda poderia trocar um symlink entre a checagem e a abertura do arquivo (TOCTOU).
- A guarda de `.git` criado durante o run não olha dentro de `node_modules/` nem abaixo de 12 níveis.
- `wb scope` e `wb status` também fazem a limpeza de um run interrompido, e nesse caso os eventos `violation` só aparecem no stderr, não na auditoria.
- Nas toolchains da home, as credenciais são procuradas só até 3 níveis de profundidade, e o `~/.cargo/config.toml` continua visível.
- A busca de arquivos sensíveis a mascarar pula `node_modules/` e vai até 12 níveis de profundidade. Ela roda antes de cada comando, então um arquivo sensível criado durante o próprio comando não é mascarado.
- A troca de "evidência" vale só para o seu pedido, não para arquivos do projeto que o agente lê: um documento com a palavra ainda pode causar recusa no meio da execução.
