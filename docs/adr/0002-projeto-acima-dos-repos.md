# 0002 — Projeto como nível acima dos repositórios

- **Status:** proposed
- **Escrito em:** 2026-11-24
- **Escopo:** o que o `wb` entende por "projeto", onde cada etapa roda e onde ficam estado e auditoria.
- **Implementação:** parcial. A auditoria já foi para `<projeto>/.workbench/runs/` (PR #5). Hoje "projeto" ainda é um único diretório, na prática um repo. O resto deste ADR não foi implementado.

## Contexto

Hoje o `wb` trata o diretório passado em `-C` (ou o atual) como o projeto inteiro. Esse diretório vira a raiz do sandbox, guarda `docs/workbench/` (as saídas das etapas) e `.workbench/` (estado, aprovações e, desde a PR #5, a auditoria). Na prática, isso equivale a um repositório.

Um jeito comum de organizar trabalho não cabe nisso. Uma ideia ou produto tem uma pasta própria, com a especificação, e um ou mais repositórios de código. Os repositórios costumam entrar como worktrees, uma por branch, dentro dessa pasta. Exemplo:

```
projetos/<p>/
├── docs/workbench/          # ideia, brief, design, plano, review
├── .workbench/              # estado e auditoria do wb
└── .worktree/<repo>/<branch>/   # código, um diretório por branch
```

Nesse arranjo, a especificação não pertence a nenhum repo: o design de um produto pode envolver vários. Rodar `wb` dentro de uma worktree, como é hoje, separa a auditoria e o estado das etapas da especificação que os originou. Também obriga a escolher um repo para guardar documentos que são do projeto.

## Decisão

1. **Dois níveis: projeto e repo.**
   - O **projeto** é a pasta da especificação. Ela guarda `docs/workbench/` e `.workbench/`, ou seja, estado, aprovações e auditoria de todos os runs do projeto.
   - O **repo** é onde o código muda: um repositório git ou uma worktree dentro do projeto.
   - Um repositório sem projeto em volta continua funcionando como hoje. Ele é o próprio projeto, com um repo só.

2. **O projeto se declara em `.workbench.json`, com a chave `repos`.**
   - O valor é uma lista de globs relativos à pasta do projeto, por exemplo `[".worktree/*/*"]`, que indicam onde estão os repos.
   - O `wb` não lê manifestos de ferramentas pessoais, como o `repos` de um script de organização. Essas ferramentas podem gerar o `.workbench.json`.

3. **Descoberta a partir do diretório atual.**
   - O `wb` sobe pelos diretórios até achar um `.workbench.json` com `repos`. Esse diretório é o projeto.
   - Se não achar, o projeto é a raiz git do diretório atual, ou o próprio diretório atual. É o comportamento de hoje.
   - `-C` continua indicando o projeto explicitamente.

4. **Cada etapa roda no seu nível.**
   - `brainstorm`, `grill-me` e `design` rodam no **projeto**. Leem o projeto inteiro, incluindo os repos, e escrevem só em `docs/workbench/`.
   - `build` e `wb run` rodam num **repo**, escolhido com `--repo <caminho ou nome>` ou pelo diretório atual quando ele está dentro de um repo do projeto. A raiz do sandbox é o repo. A especificação do projeto entra como leitura, exceto o `plan.md`, que o `build` atualiza.
   - `review` roda no repo e escreve o resultado no projeto.
   - Auditoria e estado ficam sempre no `.workbench/` do projeto, qualquer que seja o nível do run.

5. **Symlinks continuam bloqueados.** Um link dentro do projeto que aponta para fora dele, como um link para o clone original, segue negado. Isso também impede o agente de mexer num clone que deveria ficar limpo.

## Alternativas consideradas

- **Manter "projeto = um repo" (status quo).** É o mais simples, mas a especificação de um produto com vários repos precisaria morar num deles ou ser duplicada. A auditoria ficaria espalhada pelos repos.
- **Ler o manifesto do script de organização diretamente.** Funcionaria para quem usa esse script, mas prende o `wb` a uma convenção pessoal. Um `.workbench.json` gerado pelo script dá o mesmo resultado sem esse acoplamento.
- **Auditoria global em `~/.local/state/workbench/runs/` (como era antes da PR #5).** Misturava runs de todos os projetos e afastava a auditoria do projeto a que ela se refere. Foi trocada por `.workbench/runs/` dentro do projeto, que se ignora no git sozinha e que o agente não lê.

## Consequências

- O sandbox do `build` passa a ter duas áreas de escrita: o repo e o `plan.md` do projeto. Hoje a raiz é uma só, então `sandbox.ts` e `policy.ts` precisam aceitar mais de uma raiz, cada uma com suas regras.
- `wb status` mostra as etapas do projeto e, para cada repo, os runs que aconteceram nele.
- O `git status` usado para listar os arquivos alterados roda em cada repo tocado, não só na raiz.
- Projetos de um repo só não mudam: o `.workbench.json` sem `repos` continua valendo como hoje.

## Em aberto antes da aceitação

- O formato exato de `repos`: só globs, ou também nomes (`{"workbench": ".worktree/workbench/*"}`) para o `--repo` aceitar um nome curto.
- Um `build` que precise mexer em dois repos ao mesmo tempo: um run por repo, ou um sandbox com vários repos.
- Se o `wb` deve criar worktrees (`wb repo add`), ou deixar isso para outras ferramentas.
- A migração de projetos que hoje têm `docs/workbench/` dentro de um repo.
