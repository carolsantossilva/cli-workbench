# 0002 — Projeto como nível acima dos repositórios

- **Status:** proposed
- **Escrito em:** 2026-11-24
- **Escopo:** o que o `wb` entende por "projeto", onde ficam as especificações, onde cada etapa roda e onde ficam estado e auditoria.
- **Decidido em 2026-11-24:** repos detectados e `.workbench.json` só com exceções (item 2); `.workbench/` como marcador do projeto (item 3); especificações num repo ou numa issue, com issues só do GitHub e só texto de admins (item 4); o `wb` cria as worktrees (item 6).
- **Propostos em 2026-11-24, para revisão:** como as configurações de repo e projeto se combinam (item 3), o nome curto e o `build` de um repo por run (item 5), o destino sem padrão (item 4), a profundidade da busca (item 2) e o canal remoto (item 8). Não há pontos em aberto; se estes forem aceitos, o ADR vira `accepted`.
- **Implementação:** parcial. A auditoria já foi para `<projeto>/.workbench/runs/` (PR #5). Hoje "projeto" ainda é um único diretório, na prática um repo. O resto deste ADR não foi implementado.

## Contexto

Hoje o `wb` trata o diretório passado em `-C` (ou o atual) como o projeto inteiro. Esse diretório vira a raiz do sandbox, guarda `docs/workbench/` (as saídas das etapas) e `.workbench/` (estado, aprovações e, desde a PR #5, a auditoria). Na prática, isso equivale a um repositório.

Um jeito comum de organizar trabalho não cabe nisso. Uma ideia ou produto tem uma pasta própria e um ou mais repositórios de código, que costumam entrar como worktrees, uma por branch, dentro dessa pasta:

```
projetos/<p>/
├── .workbench/                  # marca o projeto; estado e auditoria do wb
├── .workbench.json              # opcional: só exceções
└── .worktree/<repo>/<branch>/   # código, um diretório por branch
```

Essa pasta não é um repositório git: ela organiza o trabalho, mas não guarda decisões. As decisões (ideia, brief, design, plano, review) precisam de histórico e de revisão. Por isso, devem ficar onde o contexto pede:
- num **repo**, quando a decisão é sobre aquele código;
- numa **issue**, quando é uma discussão, envolve vários repos ou ainda não tem repo.

Rodar `wb` dentro de uma worktree, como é hoje, separa a auditoria e o estado das etapas do projeto a que pertencem.

## Decisão

1. **Dois níveis: projeto e repo.**
   - O **projeto** é a pasta de trabalho. Ela guarda `.workbench/` (estado, aprovações e auditoria de todos os runs), o `.workbench.json` opcional e as worktrees. Ela não guarda especificações.
   - O **repo** é onde o código muda: um repositório git ou uma worktree dentro do projeto.
   - Um repositório sem projeto em volta continua funcionando como hoje. Ele é o próprio projeto, com um repo só.

2. **Os repos são detectados, não declarados.**
   - Segue o princípio que substituiu o `workspace.json` da CLI em Go, escrito à mão e central: sandboxes prontos, detecção em vez de declaração (como os comandos de teste, em `detect.ts`) e um arquivo pequeno por projeto, só com exceções.
   - Repo é todo diretório dentro do projeto com `.git` (pasta, ou arquivo no caso de worktree), até 3 níveis abaixo da pasta do projeto, o que cobre `.worktree/<repo>/<branch>`. A busca não desce em `node_modules`, em `.workbench/` nem dentro de um repo já achado: repositórios aninhados num repo (submódulos) são parte dele, não do projeto.
   - O `.workbench.json` continua só com exceções: as de hoje (`sandbox`, `deny`, `write`) e, com este ADR, o destino padrão das especificações e o lugar dos clones. Ele não lista repos.
   - O `wb` não lê manifestos de ferramentas pessoais, como o `repos` de um script de organização.

3. **A pasta `.workbench/` marca o projeto.**
   - Ela é criada por `wb init` na pasta do projeto, ou no primeiro run.
   - Para achar o projeto, o `wb` parte da raiz do repo atual e sobe pelos diretórios acima dela até achar uma `.workbench/`. Se achar, aquele diretório é o projeto.
   - A busca para antes de entrar em outro repo git, ou seja, num diretório com `.git` (pasta ou arquivo): um repo de fora não vira projeto do de dentro. Ela nunca considera a home nem o que está acima dela, comparando caminhos reais, depois de resolver symlinks.
   - Se não achar, o projeto é o próprio repo (ou o diretório atual, fora de um repo). É o comportamento de hoje.
   - A busca começa **acima** da raiz do repo: um `.workbench/` que um run antigo deixou dentro do repo não impede que o projeto de fora seja achado. Esse `.workbench/` antigo deixa de ser usado; a auditoria que está nele continua lá.
   - Se o repo tem um `.workbench.json` próprio, os `deny` dele continuam valendo e somam com os do projeto: aparecer um projeto em volta nunca afrouxa o sandbox.
   - Para `sandbox` e `write`, vale o mais próximo: num run dentro de um repo, os valores do `.workbench.json` do repo vencem os do projeto, e o do projeto só preenche o que o repo não define. Assim, um repo que já tinha configuração continua se comportando como antes. Os globs de `write` são sempre relativos à raiz do repo e não liberam nada fora dele.
   - `-C` continua indicando o projeto explicitamente.

4. **As especificações ficam num repo ou numa issue, conforme o contexto.**
   - **Num repo:** em `docs/workbench/` daquele repo, como hoje. Entram no git, no histórico e nas PRs do código a que se referem.
   - **Numa issue:** cada etapa vira o corpo da issue ou um comentário nela. Serve para discussões, decisões que envolvem vários repos, ou ideias que ainda não têm repo.
   - O destino é escolhido por run (ex.: `--spec repo:<nome>` ou `--spec issue:<dono>/<repo>#<n>`). Um padrão pode ficar no `.workbench.json`.
   - Precedência: a flag `--spec` vence o `.workbench.json`, como o `--sandbox` já faz hoje, e o `.workbench.json` vence o estado guardado. O número de issue guardado em `.workbench/state.json` só completa um destino sem número.
   - Projetos que já têm `docs/workbench/` dentro de um repo continuam funcionando sem migração: é o destino "repo".
   - Sem `--spec` e sem padrão: num projeto de um repo só, o destino é o próprio repo, como hoje. Num projeto com vários repos, o `wb` pergunta no primeiro `brainstorm` e guarda a escolha no estado; com `--no-input`, ou sem terminal, ele para com erro pedindo `--spec`, sem escolher sozinho um lugar para publicar.
   - Issues são só do GitHub, via `gh`, usando o login que o `gh` já tem.
   - Não há regra de quantas issues por ideia. Com `--spec issue:<dono>/<repo>#<n>`, a saída da etapa vira um comentário naquela issue. Com `--spec issue:<dono>/<repo>` (sem número), o `wb` abre uma issue nova e guarda o número em `.workbench/state.json`; as etapas seguintes continuam nela até outro `--spec` dizer o contrário. Assim dá para juntar as etapas numa issue só ou separar em várias, conforme o caso.
   - O agente só lê o corpo e os comentários escritos por quem tem permissão `admin` no repo da issue. O host confere isso pela API de permissões do GitHub (`/repos/{dono}/{repo}/collaborators/{usuário}/permission`), não pelo `author_association` do comentário, que não distingue admin de outros colaboradores. Como quem tem escrita no repo pode editar textos de outras pessoas, o host também confere quem fez a última edição (campo `editor` da API GraphQL); texto editado por quem não é admin fica de fora. "Nunca editado" se confere por `lastEditedAt` vazio, não por `editor` nulo: `lastEditedAt` preenchido com `editor` nulo (conta apagada) também fica de fora. Autor, editor e conteúdo vêm da mesma consulta. Se a permissão não puder ser conferida (falta de acesso do `gh`, bot, GitHub App, conta apagada), o texto fica de fora. O resto da issue não chega ao modelo.

5. **Cada etapa roda no seu nível.**
   - `brainstorm`, `grill-me` e `design` leem o projeto inteiro, incluindo os repos, e escrevem só no destino da especificação.
   - `build` e `wb run` rodam num **repo**, escolhido com `--repo` ou pelo diretório atual quando ele está dentro de um repo do projeto. A raiz do sandbox é o repo. A especificação entra como leitura, exceto o plano, que o `build` atualiza.
   - `--repo` aceita um caminho relativo ao projeto, ou o nome do repositório no remoto `origin` (ex.: `cli-workbench`), quando só há uma worktree dele. Com várias, aceita `<nome>@<branch>`; se o nome for ambíguo, o `wb` para e lista as opções.
   - Um `build` mexe em **um repo por run**. Se o plano tem lotes em repos diferentes, cada lote indica o repo e roda no seu próprio run, em sequência. O sandbox continua com uma raiz de código só, e cada run tem a sua auditoria.
   - Auditoria e estado ficam sempre no `.workbench/` do projeto, qualquer que seja o nível do run.

6. **O `wb` cria as worktrees.** Um comando (nome a definir, ex.: `wb repo checkout <repo> <branch>`) cria a worktree de um repo do projeto em `.worktree/<repo>/<branch>`, a partir da branch base atualizada. O mesmo comando reabre a worktree se ela já existir. O `build` pode pedir uma worktree nova em vez de exigir que ela já exista. O lugar dos clones (dentro do projeto ou numa pasta compartilhada entre projetos) fica configurável no `.workbench.json`.

7. **Symlinks continuam bloqueados.** Um link dentro do projeto que aponta para fora dele, como um link para o clone original, segue negado. Isso também impede o agente de mexer num clone que deveria ficar limpo.

8. **O canal remoto só vê projetos liberados.** O canal remoto (ex.: WhatsApp) não varre o disco atrás de projetos. A configuração do canal lista os diretórios que ele pode usar (ex.: `~/projetos/*`), e dentro deles valem os projetos com `.workbench/`. Uma mensagem que chega de fora só alcança o que foi liberado de propósito.

## Alternativas consideradas

- **Manter "projeto = um repo" (status quo).** É o mais simples, mas um produto com vários repos não tem onde guardar o estado e a auditoria comuns, e a auditoria fica espalhada pelos repos.
- **Especificações na pasta do projeto.** A pasta não é um repo git, então as decisões ficariam sem histórico nem revisão. Rejeitada em 2026-11-24.
- **A pasta do projeto vira um repo git só de especificações.** Daria histórico, mas cria um repo a mais por projeto e afasta a decisão do código ou da discussão a que ela pertence. Rejeitada em 2026-11-24.
- **Declarar os repos em `.workbench.json` (chave `repos`).** Foi a primeira versão deste ADR. Contradiz o combinado de detectar em vez de declarar, e o `wb` acha os repos sozinho pelo `.git`. Rejeitada em 2026-11-24.
- **`.workbench.json` como marcador do projeto.** Obrigaria a criar o arquivo mesmo sem exceções. A pasta `.workbench/` já existe em todo projeto usado pelo `wb`. Rejeitada em 2026-11-24.
- **Um `build` com vários repos no mesmo sandbox.** Exigiria várias raízes de escrita e misturaria a auditoria de repos diferentes num run. Um run por repo resolve o mesmo caso em sequência.
- **O canal remoto descobre os projetos sozinho.** Tornaria qualquer projeto no disco alcançável por uma mensagem de fora. Uma lista explícita no canal limita isso.
- **Ler o manifesto do script de organização diretamente.** Funcionaria para quem usa esse script, mas prende o `wb` a uma convenção pessoal, e a detecção dispensa o manifesto.
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
- Quando a especificação está em outro repo, o `build` tem uma segunda área de escrita além do repo do código: só o arquivo do plano. Hoje a raiz é uma só, então `sandbox.ts` e `policy.ts` precisam aceitar esse arquivo extra. Quando o plano está numa issue, o agente atualiza um rascunho local e o host publica.
- `wb status` mostra as etapas do projeto, onde está cada especificação e, para cada repo, os runs que aconteceram nele.
- O `git status` usado para listar os arquivos alterados roda em cada repo tocado, não só na raiz.
- Numa worktree, `.git` é um arquivo que aponta para o clone (`gitdir: ...`), que pode estar fora do projeto. O `git worktree add` e o `fetch` rodam no host, fora do sandbox. Dentro do sandbox, o `.git` da worktree continua só leitura, como hoje, e o clone externo fica fora da raiz. Por isso o agente não faz commit; commits ficam com o usuário ou com um passo do host.
- Projetos de um repo só não mudam: sem `.workbench/` acima do repo, o repo continua sendo o projeto, e o `.workbench.json` dele continua valendo.

## Em aberto antes da aceitação

Nenhum. Os itens propostos estão listados no cabeçalho.
