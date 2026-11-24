# Workbench host (micro MVP)

A Workbench como **o agente**, e não só como orientadora de um agente externo. Segue a direção do [ADR 0001](../docs/adr/0001-workbench-como-host-de-agente.md). Ela roda uma tarefa num projeto local usando o SDK do [Pi](https://pi.dev/) e aplica um **perfil de permissão antes de cada ação**.

Este código não substitui o núcleo Go. É um experimento para validar três premissas:
- TypeScript como linguagem do host;
- perfis prontos com exceções, em vez de um `workspace.json` escrito à mão;
- sandbox no bash.

## Uso

```sh
npm install
npx pi auth login openai-codex          # uma vez: login com a sua conta do ChatGPT
node src/cli.ts escopo ~/meu-projeto --perfil codigo
node src/cli.ts run ~/meu-projeto --perfil docs "resuma a arquitetura em docs/arquitetura.md"
```

O modelo padrão é `openai-codex/gpt-6-luna`. Para trocar, use `--modelo provider/modelo`.

### Aprovação interativa

Quando o agente tenta ler ou escrever um arquivo fora do perfil, a Workbench pergunta no terminal:

```
? O agente quer ESCREVER em src/soma.js (o perfil "docs" não permite escrever em src/soma.js)
  [s] sim, desta vez   [t] sim, até o fim desta execução   [n] não  >
```

- `s` libera só aquela chamada. `t` libera aquele arquivo até o fim da execução. `n`, ou Enter, nega.
- Com `--sem-perguntas`, ou sem terminal interativo, tudo fora do perfil é negado sem perguntar.
- Regras fixas **nunca** viram pergunta: arquivos sensíveis, caminhos fora da raiz, `.git/` e a configuração da Workbench.
- Com a aprovação ligada, até o perfil `leitura` recebe `write`/`edit`, e cada escrita vira pergunta. O bash continua só no perfil `codigo`.
- Cada resposta fica registrada na auditoria (evento `approval`).

## Perfis

| Perfil | Lê | Escreve | Comandos |
|---|---|---|---|
| `leitura` (padrão) | o projeto | nada | nenhum |
| `docs` | o projeto | `**/*.md`, `docs/**` | nenhum |
| `codigo` | o projeto | o projeto | bash num sandbox `bwrap` |

O sandbox do perfil `codigo`:
- não tem rede;
- esconde a home (só as ferramentas de `~/.nvm`, `~/go`, `~/.cargo` etc. ficam visíveis, em modo leitura);
- deixa o sistema em modo leitura;
- mascara os arquivos sensíveis do projeto com `/dev/null`;
- roda com o ambiente limpo.

Em todos os perfis vale o seguinte:
- **Sempre negados:** `.env*`, `*.pem`, `*.key`, chaves SSH, `.npmrc`, `.netrc`, `auth.json` e `credentials*.json`.
- **Nunca escritos:** `.git/`, `.workbench/` e `.workbench.json`.
- **Bloqueados:** caminhos fora da raiz, inclusive por symlink.

### Exceções por projeto (opcional)

`.workbench.json` na raiz do projeto:

```json
{ "perfil": "docs", "negar": ["dados/**"], "perfis": { "docs": { "escrita": ["CHANGELOG"] } } }
```

## O que a Workbench garante

- **Perfil aplicado antes da ação:** a chamada bloqueada volta para o modelo como erro e não acontece.
- **Ferramentas que o perfil não permite nem são oferecidas ao modelo.** Por exemplo, o perfil `leitura` não recebe `write` nem `bash`.
- **Segredos redigidos** (chaves Anthropic, OpenAI, GitHub, AWS, Google, Slack e chaves privadas) antes de entrarem no transcript e no payload enviado ao provedor.
- **Comandos detectados** a partir de `package.json`, `go.mod` e `Cargo.toml`, listados para o modelo.
- **Auditoria** em `~/.local/state/workbench/runs/<run>.jsonl`. Registra cada decisão e cada envio ao provedor (modelo, bytes e sha256, nunca o conteúdo), além dos arquivos alterados segundo o git.

## Testes

```sh
npm test          # ponta a ponta com o provider faux do Pi (sem chave, sem rede)
npm run typecheck
```

## Limites conhecidos

- A redação de segredos é heurística (regex). Um segredo fora desses padrões, num arquivo permitido, chega ao modelo.
- O filtro de `grep`/`find`/`ls` tira linhas de arquivos negados pelo nome, mas o `rg` já pula arquivos ocultos e o `.gitignore` por padrão.
- "Arquivos alterados" compara o `git status` de antes e de depois. Um arquivo que já estava modificado e foi alterado de novo não aparece.
- A aprovação vale por arquivo exato, não por pasta ou padrão, e não cobre comandos: no perfil `codigo` o bash é livre dentro do sandbox.
- Ainda não há workflows (brainstorm, design, build…), vários agentes nem WhatsApp.
- O sandbox só funciona no Linux (bubblewrap).
