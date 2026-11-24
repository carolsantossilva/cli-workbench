# 0002 — Projeto como nível acima dos repositórios

- **Status:** proposed
- **Escrito em:** 2026-11-24
- **Escopo:** o que o `wb` entende por "projeto", onde ficam as especificações, onde cada etapa roda e onde ficam estado e auditoria.
- **Decidido em 2026-11-24:** só os itens 4 (as especificações ficam num repo ou numa issue, conforme o contexto, com issues só do GitHub e só comentários de admins) e 6 (o `wb` cria as worktrees) da Decisão. O resto continua proposto.
- **Implementação:** parcial. A auditoria já foi para `<projeto>/.workbench/runs/` (PR #5). Hoje "projeto" ainda é um único diretório, na prática um repo. O resto deste ADR não foi implementado.

## Contexto

Hoje o `wb` trata o diretório passado em `-C` (ou o atual) como o projeto inteiro. Esse diretório vira a raiz do sandbox, guarda `docs/workbench/` (as saídas das etapas) e `.workbench/` (estado, aprovações e, desde a PR #5, a auditoria). Na prática, isso equivale a um repositório.

Um jeito comum de organizar trabalho não cabe nisso. Uma ideia ou produto tem uma pasta própria e um ou mais repositórios de código, que costumam entrar como worktrees, uma por branch, dentro dessa pasta:

```
projetos/<p>/
├── .workbench.json              # quais repos fazem parte do projeto
├── .workbench/                  # estado e auditoria do wb
└── .worktree/<repo>/<branch>/   # código, um diretório por branch
```

Essa pasta não é um repositório git: ela organiza o trabalho, mas não guarda decisões. As decisões (ideia, brief, design, plano, review) precisam de histórico e de revisão. Por isso, devem ficar onde o contexto pede:
- num **repo**, quando a decisão é sobre aquele código;
- numa **issue**, quando é uma discussão, envolve vários repos ou ainda não tem repo.

Rodar `wb` dentro de uma worktree, como é hoje, separa a auditoria e o estado das etapas do projeto a que pertencem.

## Decisão

1. **Dois níveis: projeto e repo.**
   - O **projeto** é a pasta de trabalho. Ela guarda `.workbench.json`, `.workbench/` (estado, aprovações e auditoria de todos os runs) e as worktrees. Ela não guarda especificações.
   - O **repo** é onde o código muda: um repositório git ou uma worktree dentro do projeto.
   - Um repositório sem projeto em volta continua funcionando como hoje. Ele é o próprio projeto, com um repo só.

2. **O projeto se declara em `.workbench.json`, com a chave `repos`.**
   - O valor é uma lista de globs relativos à pasta do projeto, por exemplo `[".worktree/*/*"]`, que indicam onde estão os repos.
   - O `wb` não lê manifestos de ferramentas pessoais, como o `repos` de um script de organização. Essas ferramentas podem gerar o `.workbench.json`.

3. **Descoberta a partir do diretório atual.**
   - O `wb` sobe pelos diretórios até achar um `.workbench.json` com `repos`. Esse diretório é o projeto.
   - Se não achar, o projeto é a raiz git do diretório atual, ou o próprio diretório atual. É o comportamento de hoje.
   - `-C` continua indicando o projeto explicitamente.

4. **As especificações ficam num repo ou numa issue, conforme o contexto.**
   - **Num repo:** em `docs/workbench/` daquele repo, como hoje. Entram no git, no histórico e nas PRs do código a que se referem.
   - **Numa issue:** cada etapa vira o corpo da issue ou um comentário nela. Serve para discussões, decisões que envolvem vários repos, ou ideias que ainda não têm repo.
   - O destino é escolhido por run (ex.: `--spec repo:<nome>` ou `--spec issue:<dono>/<repo>#<n>`). Um padrão pode ficar no `.workbench.json`.
   - Projetos que já têm `docs/workbench/` dentro de um repo continuam funcionando sem migração: é o destino "repo".
   - Issues são só do GitHub, via `gh`, usando o login que o `gh` já tem.
   - Não há regra de quantas issues por ideia. Com `--spec issue:<dono>/<repo>#<n>`, a saída da etapa vira um comentário naquela issue. Com `--spec issue:<dono>/<repo>` (sem número), o `wb` abre uma issue nova. Assim dá para juntar as etapas numa issue só ou separar em várias, conforme o caso.
   - O agente só lê o corpo e os comentários escritos por quem tem permissão `admin` no repo da issue. O host confere isso pela API de permissões do GitHub (`/repos/{dono}/{repo}/collaborators/{usuário}/permission`), não pelo `author_association` do comentário, que não distingue admin de outros colaboradores. Como quem tem escrita no repo pode editar textos de outras pessoas, o host também confere quem fez a última edição (campo `editor` da API GraphQL); texto editado por quem não é admin fica de fora. "Nunca editado" se confere por `lastEditedAt` vazio, não por `editor` nulo: `lastEditedAt` preenchido com `editor` nulo (conta apagada) também fica de fora. Autor, editor e conteúdo vêm da mesma consulta. Se a permissão não puder ser conferida (falta de acesso do `gh`, bot, GitHub App, conta apagada), o texto fica de fora. O resto da issue não chega ao modelo.

5. **Cada etapa roda no seu nível.**
   - `brainstorm`, `grill-me` e `design` leem o projeto inteiro, incluindo os repos, e escrevem só no destino da especificação.
   - `build` e `wb run` rodam num **repo**, escolhido com `--repo <caminho ou nome>` ou pelo diretório atual quando ele está dentro de um repo do projeto. A raiz do sandbox é o repo. A especificação entra como leitura, exceto o plano, que o `build` atualiza.
   - Auditoria e estado ficam sempre no `.workbench/` do projeto, qualquer que seja o nível do run.

6. **O `wb` cria as worktrees.** Um comando (nome a definir, ex.: `wb repo checkout <repo> <branch>`) cria a worktree de um repo declarado em `.worktree/<repo>/<branch>`, a partir da branch base atualizada. O mesmo comando reabre a worktree se ela já existir. O `build` pode pedir uma worktree nova em vez de exigir que ela já exista. O lugar dos clones (dentro do projeto ou numa pasta compartilhada entre projetos) fica configurável no `.workbench.json`.

7. **Symlinks continuam bloqueados.** Um link dentro do projeto que aponta para fora dele, como um link para o clone original, segue negado. Isso também impede o agente de mexer num clone que deveria ficar limpo.

## Alternativas consideradas

- **Manter "projeto = um repo" (status quo).** É o mais simples, mas um produto com vários repos não tem onde guardar o estado e a auditoria comuns, e a auditoria fica espalhada pelos repos.
- **Especificações na pasta do projeto.** A pasta não é um repo git, então as decisões ficariam sem histórico nem revisão. Rejeitada em 2026-11-24.
- **A pasta do projeto vira um repo git só de especificações.** Daria histórico, mas cria um repo a mais por projeto e afasta a decisão do código ou da discussão a que ela pertence. Rejeitada em 2026-11-24.
- **Ler o manifesto do script de organização diretamente.** Funcionaria para quem usa esse script, mas prende o `wb` a uma convenção pessoal. Um `.workbench.json` gerado pelo script dá o mesmo resultado sem esse acoplamento.
- **Auditoria global em `~/.local/state/workbench/runs/` (como era antes da PR #5).** Misturava runs de todos os projetos e afastava a auditoria do projeto a que ela se refere. Foi trocada por `.workbench/runs/` dentro do projeto, que se ignora no git sozinha e que o agente não lê.

## Consequências

- **Issues são um canal externo.** Publicar uma etapa numa issue envia conteúdo para fora da máquina, então:
  - quem publica é o host, não o agente, e com confirmação do usuário;
  - o agente escreve um rascunho local, e o host publica;
  - na leitura, só entra texto de admins do repo (item 4). Mesmo assim, ele continua sendo tratado como entrada externa: uma conta de admin pode estar comprometida, e um texto colado de outro lugar pode trazer instruções;
  - um mesmo fluxo não deve juntar dados privados, essa entrada e o envio para fora. Com os três juntos, um texto injetado na issue pode fazer o agente vazar dados (a "lethal trifecta", [Simon Willison, 2025](https://simonwillison.net/2025/Jun/16/the-lethal-trifecta/)).
- **Aprovação do design:**
  - num repo, continua sendo o sha256 do arquivo;
  - numa issue, é o sha256 do texto aprovado, guardado em `.workbench/state.json` junto com a referência da issue (e do comentário, se o design for um).
- O sandbox do `build` pode ter duas áreas de escrita: o repo do código e o plano, quando a especificação está em outro repo. Hoje a raiz é uma só, então `sandbox.ts` e `policy.ts` precisam aceitar mais de uma raiz, cada uma com suas regras.
- `wb status` mostra as etapas do projeto, onde está cada especificação e, para cada repo, os runs que aconteceram nele.
- O `git status` usado para listar os arquivos alterados roda em cada repo tocado, não só na raiz.
- Numa worktree, `.git` é um arquivo que aponta para o clone (`gitdir: ...`), que pode estar fora do projeto. O `git worktree add` e o `fetch` rodam no host, fora do sandbox. Dentro do sandbox, o `.git` da worktree continua só leitura, como hoje, e o clone externo fica fora da raiz. Por isso o agente não faz commit; commits ficam com o usuário ou com um passo do host.
- Projetos de um repo só não mudam: o `.workbench.json` sem `repos` continua valendo como hoje.

## Em aberto antes da aceitação

- O formato exato de `repos`: só globs, ou também nomes (`{"workbench": ".worktree/workbench/*"}`) para o `--repo` aceitar um nome curto.
- Um `build` que precise mexer em dois repos ao mesmo tempo: um run por repo, ou um sandbox com vários repos.
- Quem escolhe o destino quando não há padrão: o `wb` pergunta no primeiro `brainstorm`, ou exige `--spec`.
