# Review

Regras para quem revisa mudanças neste repositório, pessoa ou agente (inclusive o review antes do push). Um achado que quebre uma delas é defeito, não sugestão.

## Escopo e ferramentas do host

- **Toda ferramenta da sessão passa pela tabela `TOOLS` em `host/src/tools.ts`.** É ela que decide se a ferramenta entra na sessão, como o gate decide cada chamada e qual linha aparece na descrição do escopo (system prompt e `wb scope`). Aponte qualquer ferramenta registrada, liberada no gate ou incluída na lista da sessão por outro caminho, como um parâmetro ou opção solta ao lado do escopo.
- **Todo acesso novo do agente faz parte do `EffectiveScope`.** Isso vale para leitura fora da raiz do projeto, rede, um novo tipo de escrita ou uma nova fonte de dados. O acesso precisa ser preenchido em `resolveScope` e em `stepScope`, e aparecer no `describeScope`. Se o `wb scope` não mostra um acesso, ele não está revisado.
- **O que o system prompt diz precisa bater com o que o gate faz.** Se a descrição afirma que algo nunca é liberado, nenhuma ferramenta pode liberar isso, nem como exceção implícita.
- **Regras fixas não viram pergunta.** Arquivos sensíveis, caminhos fora da raiz, `.git` e a configuração da Workbench são negados por regra (SPEC §2), mesmo com aprovação interativa.

## Dados que vêm do modelo ou de fora

Caminhos, comandos, perguntas, páginas da wiki e qualquer texto escolhido pelo modelo são dados não confiáveis.

- **No terminal**, passam por `safe`/`oneLine` (`host/src/terminal.ts`) antes de aparecer, para que sequências ANSI, quebras de linha ou caracteres bidirecionais não falsifiquem uma pergunta de aprovação.
- **Em caminhos de arquivo**, nunca entram direto em `path.join`, `rmSync` ou similares sem validar que o resultado fica dentro da pasta esperada. Isso inclui valores lidos de arquivos de estado (`runId`, marcadores).
- **Em comandos de shell** sugeridos ou montados, vão entre aspas ou são recusados quando têm espaço, metacaractere ou começam com `-`.
- **Na auditoria e no `onDecision`**, passam por `redactSecrets`. A auditoria guarda decisões, modelo, bytes e sha256, nunca o conteúdo enviado (SPEC §7).
- **Conteúdo devolvido ao modelo** (resultados de ferramenta, páginas da wiki) passa pela mesma limpeza das outras ferramentas e é tratado como dado, nunca como instrução.

## Interfaces equivalentes

Quando a mesma função tem duas portas, como `wb wiki` (CLI) e a ferramenta Pi `wiki`, as duas validam as entradas do mesmo jeito e devolvem os mesmos resultados. Uma entrada recusada numa e aceita na outra é defeito.

## Testes

- Comportamento novo ou alterado tem teste, inclusive no CLI (parsing de opções, mensagens de erro, `checkUsage`) e nos módulos de regra (`gate.ts`, `tools.ts`, `policy.ts`).
- Uma correção de bug traz o teste que falharia sem ela.
- O que depende de TTY e não dá para testar no `cli.test.ts` (que roda com `WB_NO_INPUT=1`) precisa ser isolado numa função testável ou ter o limite registrado no PR.

## Documentação

- **Docs não podem contradizer o código no mesmo commit.** Quem muda comportamento atualiza `host/README.md`, a ajuda do CLI (`HELP` em `cli.ts`) e o `SPEC.md` quando eles descrevem esse comportamento.
- **Referências apontam para o que existe.** Um arquivo citado que foi removido é marcado como "no histórico do git", com o commit quando possível.
- Docs e mensagens ao usuário em português do Brasil. Commits em inglês, no formato Conventional Commits.

## ADRs

- Um ADR `accepted` não é editado. Uma mudança de decisão vira um novo ADR que referencia o anterior (veja `docs/adr/README.md`).
- Ao aceitar um ADR, as consequências que ele lista para o repositório (arquivos removidos, documentos reescritos) já estão feitas no mesmo PR ou registradas no backlog. O ADR não descreve como fato o que ainda não aconteceu.
