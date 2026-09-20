# Station — Especificação inicial da V1

Status: rascunho aprovado para documentação inicial
Público principal: desenvolvedor individual
Interface: CLI
Prioridade: Codex-first

## 1. Visão

A Station será uma CLI composta por workflows, skills e especificações para orientar um agente de IA na construção e evolução de software.

Ela não será, na V1, uma IDE, um serviço de modelos ou uma plataforma com banco de dados próprio. O agente existente continua responsável por raciocinar, chamar ferramentas e executar as mudanças. A Station define como o trabalho deve ser conduzido, quais decisões precisam ser tomadas e quais artefatos devem ser produzidos.

O filesystem e o Git do próprio projeto são a fonte de verdade. Não haverá um Project Store separado.

## 2. Objetivo da V1

Validar um fluxo semiautônomo e controlado para criar aplicações full-stack, começando por um caminho opinativo baseado em:

- React Native/Expo para o aplicativo;
- React para a aplicação web;
- TypeScript no cliente;
- Go para o servidor;
- monorepo como estrutura padrão;
- execução local;
- workflows guiados por políticas e checkpoints de design.

O quick win principal será o fluxo **design-to-scaffold** de um monorepo:

```text
ideia → brainstorm → grill-me → design aprovado → scaffold → testes/revisão
```

A implementação de funcionalidades completas será adicionada gradualmente, em lotes definidos pelo workflow e controlados pelas políticas do workspace.

### Projeto de validação da Station

O primeiro projeto construído pela Station será um app de tarefas/planner pessoal inspirado no Tiimo. Ele servirá como caso de dogfooding da Station.

O ponto de partida será uma ideia curta, por exemplo:

> Quero criar um app de tarefas parecido com o Tiimo.

A Station deverá usar seus próprios workflows de brainstorm, grill-me e design para descobrir requisitos, limites, arquitetura e critérios de aceite do app. Não haverá um contrato funcional detalhado definido antecipadamente para o projeto-alvo.

A validação se concentrará na Station: execução correta dos workflows, produção dos artefatos, criação e evolução do `workspace.json`, scaffold do monorepo, execução local, testes e respeito às políticas. O app de tarefas não implementará os workflows da Station.

## 3. Modos de entrada

### Projeto novo

`station:new` inicia um projeto a partir de uma ideia, conduz a descoberta e prepara o scaffold inicial de React Native/Expo, aplicação web React e servidor Go em um monorepo.

### Repositório existente

`station:existing` começa pela leitura da estrutura, convenções e estado do monorepo. Na V1, esse modo terá escopo mais limitado: análise, planejamento e pequenas alterações controladas.

Ambos os modos convergem para os mesmos workflows de design, construção e revisão.

## 4. Workflows da V1

```text
station:new ou station:existing
        ↓
brainstorm
        ↓
grill-me
        ↓
design
        ↓
build
        ↓
review
```

### Brainstorm

Transforma uma ideia ou objetivo em entendimento compartilhado, requisitos, critérios de sucesso e limites de escopo.

### Grill-me

Submete o entendimento ou plano a uma entrevista crítica. Deve revelar ambiguidades, premissas frágeis, riscos, casos extremos e decisões ainda não resolvidas.

### Design

Produz uma especificação técnica suficiente para construir o próximo incremento. Deve registrar arquitetura, componentes, fluxo de dados, tratamento de erros, testes e decisões relevantes.

Pesquisa arquitetural ampla e comparação sistemática de soluções ficam fora da V1; o design deve resolver apenas o necessário para o objetivo atual.

### Build

Executa o plano em lotes definidos pelo workflow. Cada lote deve apresentar intenção, arquivos afetados, comandos relevantes e critério de verificação. A execução prossegue automaticamente quando estiver dentro das políticas; exceções, bloqueios e ações não autorizadas interrompem o fluxo.

### Review

Verifica diff, testes, aderência ao design, riscos de segurança e possíveis violações das políticas de dados.

## 5. Artefatos

Os artefatos são arquivos legíveis no próprio repositório:

```text
workspace.json
docs/station/brief.md
docs/station/design.md
docs/station/decisions.md
docs/station/plan.md
```

Esses arquivos substituem qualquer estado interno de projeto. O histórico do Git registra a evolução do código e dos documentos.

### Workspace manifest

O `workspace.json` é o contrato versionado da Station para o workspace. Ele deve conter, no mínimo:

- `schemaVersion`;
- identidade do workspace e tipo de estrutura (`monorepo` na V1);
- projetos internos, com path e função, como `mobile`, `web`, `server` e `shared`;
- comandos de desenvolvimento, teste, lint, build e diagnóstico;
- políticas de execução, dados e mudanças externas;
- configuração dos workflows, incluindo skills obrigatórias, skills permitidas, comandos permitidos, caminhos de leitura/escrita e artefatos esperados;
- repositórios externos opcionais.

O monorepo é o repositório principal. Repositórios externos são read-only por padrão e só podem ser alterados por uma política explícita.

O manifesto não armazena estado transitório de sessões, logs ou histórico de execução. Ele descreve a composição e as regras evolutivas do workspace; o filesystem, o Git e os artefatos da Station guardam o restante.

## 6. Protocolo de contexto

Não haverá um Context Engine separado. A otimização de contexto será uma regra dos próprios workflows e poderá ser declarada no `workspace.json`.

Cada workflow deve declarar:

- quais arquivos e artefatos são entradas;
- quais comandos de inspeção podem ser usados;
- quais informações precisam ser resumidas;
- qual artefato deve ser produzido;
- quais informações serão passadas à próxima etapa.

O `workspace.json` também pode declarar quais skills e comandos cada workflow pode usar. A CLI deve validar esse contrato antes e durante a execução; o manifesto não deve ser tratado apenas como documentação.

O agente não deve reler o repositório inteiro em cada etapa. O fluxo deve priorizar artefatos, diffs, arquivos relevantes e resultados de comandos.

Exemplo de handoff:

```text
brainstorm → brief.md
grill-me → decisões e riscos incorporados ao design.md
design → design.md
build → plan.md + arquivos relevantes + design.md
review → diff + testes + decisões
```

## 7. Governança de dados

O envio de dados para provedores externos é permitido com controle. A execução da IA, porém, deve ser autônoma dentro das políticas declaradas, sem exigir um token literal de autorização em cada fase.

A V1 deve prever, no mínimo:

- política por projeto, pasta ou tipo de arquivo;
- lista de provedores permitidos;
- detecção e bloqueio ou mascaramento de segredos;
- aprovação antes do envio de novas categorias de dados;
- registro local do que foi preparado ou enviado para um provedor;
- distinção explícita entre dados locais, dados permitidos e dados proibidos.

As políticas devem permitir pelo menos três resultados para uma ação: permitir automaticamente, pedir autorização por ser uma exceção ou bloquear.

Leitura, edição local e testes podem ser executados automaticamente quando estiverem dentro da política. Push, PR, deploy, alterações em sistemas externos e escrita em repositórios externos devem seguir as regras de mudanças externas do manifesto. Repositórios externos são read-only por padrão.

Skills fornecem orientação, mas não são uma garantia técnica por si só. Quando o host do agente permitir, a Station deverá complementar as regras com comandos, hooks ou verificações determinísticas locais.

## 8. Princípios de execução

- A Station tem uma CLI própria; não haverá interface gráfica própria.
- Codex-first; o formato inicial deve aproveitar diretamente skills e workflows do Codex.
- Sem chamadas próprias a modelos ou APIs de IA.
- Sem banco de dados interno ou Project Store.
- Filesystem e Git como fonte de verdade.
- Autonomia delimitada por políticas: o agente executa tudo que estiver permitido e interrompe apenas em exceções, bloqueios ou ações que exigem autorização.
- Construção incremental, com diffs e testes verificáveis.
- Contexto mínimo suficiente para cada etapa.

## 9. Fora do escopo da V1

- interface web ou desktop;
- execução cloud ou híbrida;
- suporte profundo a várias stacks;
- pesquisa arquitetural autônoma abrangente;
- deploy e operação em produção;
- múltiplos agentes independentes;
- colaboração multiusuário;
- construtor universal de qualquer tipo de sistema.

## 10. Evolução posterior

Depois de validar o fluxo local, os próximos incrementos possíveis são:

1. implementação de funcionalidades completas em lotes;
2. suporte mais profundo a repositórios existentes;
3. skill de pesquisa e decisão arquitetural;
4. políticas e hooks de governança mais fortes;
5. execução híbrida ou remota;
6. suporte a outras stacks e agentes.
