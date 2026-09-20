# Station — Plano de Implementação

> **Para agentes implementadores:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recomendado) ou superpowers:executing-plans para implementar este plano tarefa por tarefa. Os passos usam a sintaxe de checkbox (- [ ]) para acompanhamento.

**Objetivo:** entregar o menor núcleo funcional da Station capaz de validar um workspace.json, preparar e verificar os workflows da V1, controlar caminhos/comandos/dados conforme políticas declaradas e orientar o fluxo new/existing até o scaffold de um monorepo, sem chamadas próprias a modelos ou APIs de IA.

**Arquitetura:** a Station será uma CLI local, sem banco de dados e sem Context Engine separado. O workspace.json será o contrato versionado; a CLI carregará esse contrato, validará a execução, produzirá pacotes de contexto/handoff e verificará artefatos no filesystem. O agente existente continuará responsável por raciocínio, edição de arquivos e chamadas de ferramentas; a Station não gerará conteúdo de produto nem chamará modelos.

**Stack técnico recomendado:** Go para a CLI, Cobra para a árvore de comandos e uma biblioteca Go compatível com JSON Schema Draft 2020-12, sujeitas a pesquisa atual antes de fixar versões. Testes usam inicialmente `testing`/`go test` da biblioteca padrão; o scaffold usa templates locais. As versões observadas no ambiente são um registro histórico do planejamento e serão verificadas novamente antes da implementação; não há módulo nem dependências da Station instalados neste estado.

**Especificação:** /home/carogo/projetos-carol/station/design.md.

## Restrições globais

As seguintes regras são requisitos da especificação e valem para todas as tarefas:

- “A Station tem uma CLI própria; não haverá interface gráfica própria.”
- “Codex-first; o formato inicial deve aproveitar diretamente skills e workflows do Codex.”
- “Sem chamadas próprias a modelos ou APIs de IA.”
- “Sem banco de dados interno ou Project Store.”
- “Filesystem e Git como fonte de verdade.”
- “Autonomia delimitada por políticas: o agente executa tudo que estiver permitido e interrompe apenas em exceções, bloqueios ou ações que exigem autorização.”
- “Construção incremental, com diffs e testes verificáveis.”
- “Contexto mínimo suficiente para cada etapa.”
- O tipo de estrutura da V1 é monorepo.
- O monorepo padrão contempla mobile React Native/Expo, web React, TypeScript no cliente e servidor Go.
- Repositórios externos são read-only por padrão e só podem ser alterados por uma política explícita.
- O manifesto não armazena estado transitório de sessões, logs ou histórico de execução.
- Push, PR, deploy, alterações em sistemas externos e escrita em repositórios externos seguem as regras de mudanças externas do manifesto.
- A V1 não inclui interface web/desktop, cloud/híbrido, deploy de produção, múltiplos agentes independentes, colaboração multiusuário ou um construtor universal.
- O app de tarefas inspirado no Tiimo é apenas o primeiro projeto de dogfooding posterior; não será criado neste plano nem na implementação da Station.
- O estado inicial observado não é Git. O usuário autorizou explicitamente `git init` e commits locais atômicos para esta implementação; essa autorização não permite push, PR, deploy ou qualquer escrita externa.
- O trabalho de implementação deve usar uma worktree local se disponível, preservando a árvore aprovada e trazendo os commits concluídos de volta somente após revisão e verificação.

## Foco de revisão

Estas são as cinco classes de entrada mais prováveis de causar falhas e que devem ser cobertas explicitamente pelos testes das tarefas:

1. workspace.json inválido, incompleto, com versão de schema desconhecida ou com referências duplicadas; a CLI deve exibir diagnósticos determinísticos e não criar auditoria operacional nem executar comandos.
2. Caminho absoluto, .., symlink ou glob que escape da raiz do workspace; a CLI deve rejeitar suas próprias operações fora dos limites e reportar no postflight writes externos ao contrato que conseguir observar.
3. Comando com executável/argumento não declarado ou com composição de shell; a CLI deve rejeitar a execução em vez de interpolar texto em um shell.
4. Arquivo `.env*`, PEM, categoria de dado desconhecida ou repositório externo; deve ser omitido/redigido do contexto ou resultar em ask/deny conforme a regra. Sem integração de host, a Station não impede transmissão por ferramenta externa.
5. Handoff com artefato ausente, desatualizado ou escrito fora de writePaths; o workflow deve parar com estado verificável e reportar a divergência sem alegar que preveniu escrita direta pelo host.

---

## 1. Estado verificado, limites e divergências

Em 20/09/2026, a inspeção local encontrou:

    /home/carogo/projetos-carol/station/design.md

Na inspeção inicial, o arquivo de especificação tinha 204 linhas e 9.141 bytes e era o único arquivo do diretório. Depois de criar este plano, a árvore contém `design.md` e `docs/station/plan.md`; não há workspace.json, código-fonte, testes, templates, go.mod ou configurações de lint/build. `git rev-parse --is-inside-work-tree` falhou com “not a git repository”; portanto o diretório não é um repositório Git nem uma worktree verificável.

Ferramentas locais verificadas sem alterações em 20/09/2026: Go go1.27.1 linux/amd64, Node.js v24.21.0, npm 11.19.0, Git e jq. pnpm, Yarn, Bun, Deno, TypeScript, ESLint e Prettier não foram encontrados no PATH. Isso confirma que Go e Node são ambos opções locais; não prova disponibilidade em outro host ou em um futuro workspace. Cobra v1.10.2 existe no cache do Go, mas não é dependência deste diretório; jsonschema v6.0.3 não foi encontrado no cache local consultado.

Divergências e limites registrados, sem resolvê-los silenciosamente:

- A especificação chama design.md de “rascunho aprovado para documentação inicial”. Este plano trata o conteúdo como a especificação aprovada fornecida pelo usuário, mas não altera o status textual do arquivo.
- A especificação lista docs/station/plan.md como artefato da Station, e o Prompt 1 exige exatamente esse caminho para o plano da própria Station. Este arquivo ocupa esse caminho neste workspace. Em um projeto de dogfooding separado, o mesmo caminho poderá conter o plano daquele projeto; a CLI deve operar sempre relativa à raiz do workspace para evitar confusão entre os dois contextos.
- A especificação exige servidor Go, mas não define o module path. O scaffold não deve inventar um endereço publicável; o module path deve ser uma entrada explícita do design do workspace ou da configuração de scaffold antes de criar apps/server/go.mod.
- A especificação exige skills obrigatórias e permitidas por workflow, mas não fornece uma lista de IDs para todo workspace nem um mecanismo host de resolução. O schema armazenará essas listas e a CLI verificará sua consistência; disponibilidade efetiva no host ficará como estado reportado, não como fato presumido.
- A especificação diz que Git é fonte de verdade, mas este diretório não tem Git. A V1 deve usar snapshot de filesystem para detectar mudanças quando Git não estiver presente e usar git diff apenas quando a inspeção confirmar um repositório.
- Expo e web React têm ferramentas de scaffold atuais, mas nenhuma delas foi verificada ou instalada neste workspace. A Station não deve executar npx, instalar dependências ou fazer rede implicitamente durante station new; essas ações só podem ser comandos declarados e permitidos no workspace do projeto.

## 2. Decisões de arquitetura

### 2.1 Limite entre Station e agente

O núcleo V1 será um controlador de contratos, não um agente de IA:

1. A CLI localiza a raiz e carrega workspace.json.
2. A CLI valida schema, referências, políticas, inputs, skills declaradas, comandos declarados e limites de caminho.
3. A CLI prepara um pacote de contexto com referências a arquivos relevantes, hashes e resumos mínimos; ela não relê o repositório inteiro.
4. O agente existente usa esse contrato para raciocinar e editar arquivos por suas ferramentas normais.
5. A CLI verifica os artefatos esperados, as mudanças de caminho e o resultado de testes/comandos declarados.

Não haverá cliente HTTP de modelo, SDK de IA, banco de sessões, daemon ou serviço remoto nesta fase.

### 2.2 Organização da CLI

O código será dividido por responsabilidade, com Cobra restrito à borda de CLI e sem framework de aplicação adicional:

    cmd/station/main.go            # entrypoint compilável
    internal/cli/                  # parsing, help, despacho e erros
    internal/workspace/            # loader, schema e validação semântica
    internal/policy/               # decisões de execução, dados, caminhos e externos
    internal/workflows/            # definições, contexto, handoffs e verificação
    internal/modes/                # new/existing
    internal/scaffold/             # estrutura monorepo e templates não funcionais
    internal/audit/                # registro local redigido em JSONL
    internal/assets/               # schema e assets locais embutidos no binário
    testdata/                      # fixtures versionadas

A CLI será um módulo Go (`go.mod`/`go.sum`) com subcomandos aninhados. A implementação deve revalidar se Cobra continua sendo a opção adequada e compatível no momento da implementação; a recomendação é usar Cobra para registrar a árvore de comandos, help, validação de argumentos e tratamento de erros, em vez de manter dispatcher/flags próprios. A documentação de Cobra mostra subcomandos ligados a um comando-pai e recomenda modularizar somente quando a CLI crescer; GitHub CLI e Docker usam agrupamento por domínio/recurso. Se uma pesquisa atualizada revelar incompatibilidade, manutenção inadequada ou custo desproporcional, registrar a evidência e escolher uma alternativa mínima antes de codificar. O validador JSON Schema será dependência explícita e versionada; não será copiado nem presumido como instalado. Revalidar a versão suportada de Go e a compatibilidade da biblioteca na implementação, em vez de tratar os números deste plano como permanentes.

A recomendação continua sendo Go por dois motivos operacionais verificados no plano: a CLI pode ser compilada em um executável e Go está disponível localmente. Isso pode reduzir dependências e custo de inicialização, mas não constitui medição comparativa da Station; performance não é critério de aceitação V1 e não se deve anunciar uma vantagem numérica sem benchmark controlado. Node continua tecnicamente viável e disponível neste ambiente. A implementação deve revalidar somente aspectos atuais que possam alterar decisões (toolchain mínima, suporte JSON Schema e dependências da CLI); benchmarks não são necessários salvo se surgir um problema medido de latência.

### 2.3 Contrato de saída e estados

Toda operação pública deve produzir texto humano por padrão e JSON determinístico quando --format json for solicitado. O resultado interno terá esta forma:

    Result{
      OK bool
      Status string // ready | completed | ask | blocked | invalid
      Code string
      Diagnostics []Diagnostic // code, message, path, severity
      Artifacts []Artifact // path, required, exists, sha256
      AuditEventID string
    }

Exit codes estáveis:

- 0: operação válida e concluída ou pronta para a próxima etapa;
- 2: argumentos ou configuração inválidos;
- 3: a política exige autorização/decisão externa;
- 4: a política bloqueia a ação;
- 5: input ou artefato esperado ausente/desatualizado;
- 6: pré-requisito local não disponível.

station não deve interpretar ask como autorização. Ele interrompe, registra a decisão pendente e devolve o controle ao usuário/agente.

## 3. workspace.json: estrutura e validação

O manifesto terá `schemaVersion: 1`. O schema canônico ficará em `internal/assets/workspace.schema.json` e declarará o dialeto JSON Schema Draft 2020-12 em seu próprio `$schema`. A CLI o incorporará ao binário com `embed`; `station new` escreverá uma cópia versionada em `schemas/workspace.schema.json` no workspace criado. Na V1, só haverá schema embutido para `schemaVersion: 1`; versões desconhecidas serão rejeitadas. Assim, a execução não depende do cwd nem de URI remoto. Não colocar `./schemas/workspace.schema.json` em `$schema` do manifesto como se esse fosse o dialeto: embora alguns editores usem esse atalho para associar um schema a um documento, a convenção é específica de tooling e não requisito da Station nesta V1.

Formato mínimo proposto:

O exemplo abaixo mostra somente os grupos de campos do contrato; por conter objetos vazios, não é um fixture válido. Os fixtures de teste devem preencher os campos obrigatórios do schema sem introduzir políticas ou skills default não aprovadas.

    {
      "schemaVersion": 1,
      "workspace": {
        "id": "example-workspace",
        "kind": "monorepo"
      },
      "projects": [
        { "id": "mobile", "path": "apps/mobile", "kind": "mobile", "language": "typescript" },
        { "id": "web", "path": "apps/web", "kind": "web", "language": "typescript" },
        { "id": "server", "path": "apps/server", "kind": "server", "language": "go" }
      ],
      "commands": {
        "dev": [],
        "test": [],
        "lint": [],
        "build": [],
        "diagnostics": []
      },
      "policies": {
        "execution": {},
        "data": {},
        "externalChanges": {}
      },
      "workflows": {},
      "externalRepositories": []
    }

Regras semânticas que não devem ficar somente no JSON Schema:

- workspace.id é uma string estável; kind só aceita monorepo na V1.
- IDs de projetos, comandos, workflows e repositórios externos são únicos dentro do respectivo conjunto.
- Projeto interno tem path relativo à raiz, sem / inicial, sem componente .., e seu path canônico permanece dentro da raiz.
- Para um projeto novo, mobile, web e server vêm do escopo aprovado da V1. `shared` é opcional e só deve ser criado quando o design do projeto justificar código compartilhado. Um workspace existente pode declarar os projetos internos que sua estrutura observada comporta.
- Cada comando tem id, categoria (dev, test, lint, build ou diagnostics), executável, array de argumentos, projeto/cwd interno e política de timeout. O comando nunca é uma string shell livre.
- workflows contêm requiredSkills, allowedSkills, allowedCommands, readPaths, writePaths, inputs, outputs, summaries e handoff. requiredSkills deve ser subconjunto de allowedSkills; allowedCommands deve referenciar IDs existentes.
- Cada path declarado em workflow é relativo e não pode escapar da raiz. Paths de escrita devem ser subconjuntos da superfície permitida pela política.
- externalRepositories exige id, path local e readOnly: true como default efetivo. URL remota, clone, fetch e escrita remota não fazem parte do núcleo V1.
- Políticas usam apenas os resultados allow, ask e deny, com a regra mais específica vencendo a regra default.
- Campos desconhecidos em objetos de contrato são rejeitados na V1, exceto um bloco versionado metadata sem efeito operacional. Isso torna migrações observáveis em vez de ignorar typos.

O loader deve parsear JSON, selecionar o schema embutido pela `schemaVersion`, validar a estrutura, normalizar a raiz absoluta e então executar as regras semânticas. Um erro de parse ou schema não pode criar diretórios, gravar auditoria operacional ou executar comando.

## 4. Comandos iniciais da CLI

A especificação define os modos `station:new` e `station:existing`, mas não fixa comandos de terminal. Pesquisa de convenções (fontes na seção 16) mostra árvores com subcomandos aninhados e verbo após um recurso, como `gh workflow run` e `gh repo create`, além do modelo de pai/filho de Cobra. Para manter o fluxo aprovado e adotar nomes acionáveis, o modo existente será exposto como `inspect` (ID interno do modo continua `existing`) e a execução será agrupada em `workflow run`.

Comandos V1 propostos e fundamentados:

    station validate
    station new <ideia>
    station inspect [caminho]
    station workflow run <workflow-id> [--command <command-id>]
    station workflow verify <workflow-id> <run-id>
    station --version
    station --help

`--format json` ficará disponível em `validate`, `inspect` e `workflow`; não será um comando adicional. A implementação deve conferir help, formato e escolhas do parser com a documentação oficial atual do framework antes de fechar detalhes, sem inventar comandos sugeridos apenas para completar uma hierarquia.

Por padrão, `new` usa o diretório atual como destino; `validate` e `workflow run` usam o diretório atual; `inspect` aceita caminho opcional para observar um repositório existente. A CLI não muda de diretório nem adivinha outra raiz. Essa regra é uma decisão de ergonomia deste plano, pois a especificação não define parâmetro de raiz; deve ser validada por testes de integração.

Comportamento:

- `validate` carrega e valida o manifesto, paths, comandos, políticas e referências entre workflows. Não escreve arquivos.
- `new <ideia>` exige diretório de destino vazio e retorna erro sem escrever se encontrar qualquer entrada preexistente. Cria estrutura inicial, manifesto, instruções do monorepo, skills locais e emite contexto inicial para a ideia. Não inicializa Git, não cria especificação funcional do produto, não instala dependências e não chama ferramentas externas.
- `inspect [caminho]` corresponde ao modo de produto `existing`: começa com inventário local read-only mesmo sem `workspace.json`. Antes do manifesto, pode listar entradas imediatas e ler somente metadados de projeto de uma allowlist explícita e rasa (`AGENTS.md`, `README.md`, `go.mod`, `go.work`, `package.json`, `pnpm-workspace.yaml`, `app.json` e configurações de workspace reconhecidas pela implementação). Não lê arquivos-fonte em massa, não segue links para fora, não executa comandos e não escreve. Sem manifesto válido, serve apenas para análise; um `workspace.json` conservador precisa ser criado/revisado pelo operador ou agente como bootstrap de política e passar `station validate` antes de workflows. A V1 não acrescenta um comando implícito de adoção. Com manifesto válido, respeita `readPaths`; writes continuam sujeitos ao contrato e ao limite de observabilidade descrito na seção 6.
- `workflow run <id>` valida inputs, grava um evento local `prepared` com run-id e hashes dos paths declarados, e imprime as instruções/handoff, skills e comandos permitidos. O agente humano/Codex cria o conteúdo dos artefatos; a Station não chama modelos.
- `workflow verify <id> <run-id>` compara o snapshot `prepared` com o estado atual dos paths declarados, verifica outputs e reporta writes fora de `writePaths` que forem observáveis. Não substitui controle de ferramentas do host.
- Para observar paths fora do workflow sem ampliar o contexto, a enumeração complementar no filesystem registra somente path, existência, tamanho e mtime; hash de conteúdo é restrito aos paths declarados. Sem evidência Git, uma mudança que preserve tamanho e mtime pode não ser detectada.
- `--command <id>` permite executar somente um executável e argv declarados e permitidos pelo workflow, sem shell. `ask` ou `deny` encerram sem iniciar processo.
- `--format json` omite conteúdo integral de arquivos por padrão e retorna paths, hashes, contagens e diagnósticos.

O parser deve rejeitar flags desconhecidas, argumentos posicionais extras, workflow inexistente e combinações de modo/opção não suportadas. Mensagens de erro apontam o argumento ou caminho do campo inválido. O uso de subcomandos hierárquicos, ajuda por subcomando e formas singulares de nomes deve ser exercitado em testes do contrato público da CLI.

## 5. Workflows e responsabilidades

Os workflows são contratos declarativos que convergem para a sequência aprovada:

    station new ou station inspect (modo existente)
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

| Workflow | Entradas mínimas | Responsabilidade | Instruções versionadas / saídas | Limite de escrita |
|---|---|---|---|---|
| brainstorm | ideia ou estado existente, workspace.json quando existente | produzir entendimento compartilhado, critérios e limites | `docs/station/workflows/brainstorm.md` → `docs/station/brief.md` | somente outputs declarados; bootstrap de existing é read-only |
| grill-me | brief.md, estado relevante | desafiar ambiguidades, premissas, riscos e casos extremos | `docs/station/workflows/grill-me.md` → `docs/station/decisions.md` e design seguinte | somente artefatos de Station declarados |
| design | brief.md, decisions.md, manifesto e paths dos projetos | registrar arquitetura, fluxo, erros, testes e decisões do próximo incremento | `docs/station/workflows/design.md` → `docs/station/design.md` | somente docs/station/design.md |
| build | design aprovado, manifesto e plano/contexto relevante | dividir execução em lotes, aplicar política e verificar diffs/testes | `docs/station/workflows/build.md` → `docs/station/plan.md`, arquivos do lote e resultados reais | paths de projeto explicitamente aprovados |
| review | diff/snapshot, design, plan, testes e decisões | verificar aderência, segurança e políticas de dados | `docs/station/workflows/review.md` → resultado em decisions.md | documentação de revisão; nenhum push/PR/deploy |

Os cinco arquivos em `docs/station/workflows/` são instruções legíveis/versionadas para o agente, não extensões nativas presumidas do Codex. `station workflow run <id>` seleciona instrução e inputs do manifesto, apresenta o handoff, e depois verifica outputs quando executado novamente. `station new` os cria a partir de assets locais da Station; a CLI não chama modelos nem presume que qualquer host os execute automaticamente.

grill-me não terá um arquivo de relatório adicional, porque a especificação não o lista: suas decisões e riscos entram em decisions.md e no design seguinte. docs/station/design.md é o design do projeto que está sendo conduzido; não deve sobrescrever o /home/carogo/projetos-carol/station/design.md que especifica a própria Station.

### Configuração de skills

O manifesto distingue `requiredSkills` de `allowedSkills` por workflow. A scaffold do monorepo inclui três skills locais versionadas, com nomes declarados nos próprios metadados: `station-mobile`, `station-web` e `station-server`, em `.agents/skills/<name>/SKILL.md`. Cada workflow pode permitir as três; um agente escolhe somente a skill que corresponda às áreas tocadas, e nenhuma delas é obrigatória por padrão. `AGENTS.md` explica limites que se aplicam sempre; as skills guardam instruções acionáveis de manutenção por stack. A CLI valida consistência entre IDs declarados e arquivos locais/metadados quando estes existem, sem alegar que o host carregou ou executou a skill.

As skills locais dizem ao agente para ler o manifesto e instructions da área, confirmar versões e comandos realmente declarados, e consultar documentação primária atual quando uma escolha puder ter mudado. Elas não adicionam dependências, comandos permitidos ou política de produto automaticamente. Skills externas/globalmente instaladas continuam configuráveis por ID, mas sua existência no host fica `unverified` se não houver interface local de resolução. A CLI nunca instala, baixa ou renomeia skills.

## 6. Políticas de execução, governança de dados e mudanças externas

### 6.1 Execução local

Os controles se aplicam às operações que a própria CLI faz e aos comandos iniciados por ela. A Station não intercepta ferramentas de filesystem do Codex ou de outro host; portanto não é sandbox e não consegue impedir, em tempo real, um write arbitrário que o agente faça fora da CLI. Para esses writes, a V1 declara limites, orienta o agente e detecta/relata divergências no postflight. Não apresentar validação ou auditoria como enforcement técnico completo.

O mecanismo de autorização recebe uma ação estruturada:

    {
      kind: "read" | "write" | "command" | "external-write" | "data-send",
      path: "apps/web/src/main.tsx",
      commandId: "web.test",
      dataCategory: "source",
      provider: "local"
    }

Ele retorna { decision: "allow" | "ask" | "deny", reason, matchedRule }. A implementação deve avaliar nesta ordem: bloqueios de segredo/fora da raiz, regra mais específica do projeto/path/tipo, regra do workflow, default da política. Qualquer ausência de evidência deve resultar em ask ou deny, nunca em permissão implícita.

Contrato JSON V1 adotado para `policies.execution`: `default` opcional com `allow`, `ask` ou `deny` (ausente equivale a `ask`) e `rules`, uma lista opcional de regras com `decision` e pelo menos um seletor entre `kind`, `project`, `path`, `commandId`, `category` e `provider`. `kind` aceita `read`, `write` ou `command`; `path` é relativo, usa `/` e admite glob `**` entre segmentos. Campos desconhecidos, referências a projetos/comandos inexistentes ou decisões inválidas tornam `station validate` inválido e não podem resultar em allow. Entre regras correspondentes, vence a que tem mais seletores; empate usa a decisão mais restritiva (`deny`, depois `ask`, depois `allow`).

O escopo do workflow continua sendo um limite independente: uma regra de policy não amplia `readPaths`, `writePaths` ou `allowedCommands`. Uma regra allow só libera ação dentro do escopo correspondente; write fora de `writePaths` é deny, e read/comando não listados usam ask/deny, nunca um default allow implícito. Sem regra específica, um item explicitamente listado pelo workflow é uma autorização declarada; fora disso aplica-se o default. `external-write` permanece deny na V1, mesmo se `externalChanges` contiver dados, porque não há executor de efeitos externos.

Paths usados pela CLI devem ser canonicalizados antes da decisão, rejeitando traversal e links existentes que escapem dos limites. A CLI passa executável e argumentos como vetor para qualquer processo autorizado, sem shell. A validação não elimina condições de corrida do filesystem nem controla processos iniciados pelo host por fora.

### 6.2 Dados

`policies.data` deve suportar:

- categorias explícitas source, design, test-output, secret e unknown;
- lista de providers allowlist, inicialmente sem providers externos porque a Station V1 não envia dados;
- regras por projeto/path e decisão allow, ask ou deny;
- omissão ou redaction configurável de secret, bloqueando por padrão sua inclusão no pacote de contexto;
- decisão ask para categoria nova/unknown;
- distinção entre contexto preparado localmente e envio feito por um host externo. Sem hook do host, a Station só registra preparação, não confirma envio.

Contrato JSON V1 adotado para `policies.data`: `default` opcional (ausente equivale a `ask`), `categories` opcional com decisões por categoria, `providers` opcional como lista de IDs e `rules` opcional com seletores `project`, `path` e/ou `category`, mais `decision`. Referências de projeto inexistentes e campos desconhecidos invalidam o manifesto. `secret` só aceita deny; categoria `unknown` não pode ser allow e fica ask por padrão. A regra correspondente mais específica precede a decisão da categoria e o default; empate usa deny > ask > allow. `providers` é apenas allowlist declarativa: não há adapter nem ação `data-send` permitida nesta V1.

Bootstrap de `station new`: manter `default: ask`; permitir somente a categoria `design` para que a ideia fornecida pelo usuário e os handoffs locais de design possam formar contexto para os workflows aprovados. `source` e `test-output` permanecem em `ask` até configuração explícita; `secret` continua `deny`, `unknown` continua `ask`, e envio a provider permanece bloqueado. Esse default permite preparação local de contexto, não autoriza o host a transmitir dados.

O núcleo inicial deve excluir arquivos `.env*` e detectar cabeçalhos PEM em arquivos que seriam adicionados ao contexto; o evento guarda somente tipo, path, linha e hash do trecho, nunca o valor. Não implementar heurística de tokens de alta entropia no primeiro núcleo sem avaliar seus falsos positivos/negativos. A lista pequena de padrões fica centralizada e coberta por testes.

Como a Station não fará chamadas de modelo nem integrações de provider, a V1 registra preparação local e decisões de preflight. O estado `sent` só pode ser registrado quando o host chamar uma interface explícita de auditoria; não haverá adapter de envio nesta etapa. Não prometer que essas anotações impedem exfiltração por integração externa.

### 6.3 Auditoria

O manifesto não conterá logs. A implementação pode usar `.station/audit.jsonl` como registro local append-only para decisões da CLI, comandos iniciados e metadados de contexto preparado, com eventId/runId, timestamp, workflow, paths redigidos, categorias, hashes e resultado. `workflow verify` acrescenta evento de conclusão ligado ao evento `prepared`; nunca edita linhas antigas. Nunca gravar conteúdo ou valores de segredo; excluir o próprio log do contexto. A auditoria não é banco de dados, não captura ações externas feitas sem integração de host e não é prova de todos os efeitos do processo.

### 6.4 Mudanças externas

Sem uma regra explícita, external-write, push, PR, deploy, login, upload e criação de recurso externo devem resultar em `deny` ou `ask`, nunca `allow`. Repositórios externos são lidos somente de checkout local já existente e permanecem read-only por padrão. A V1 não faz clone, fetch, push, PR, deploy, login ou chamada a serviço externo. A autorização excepcional de `git init` para este desenvolvimento da Station não muda o default dos workspaces que ela criará.

## 7. Monorepo mobile/web/server

O scaffold lógico padrão terá:

    apps/mobile/       # React Native/Expo + TypeScript
    apps/web/          # React + TypeScript
    apps/server/       # Go
    packages/shared/   # opcional: só quando o design justificar código compartilhado
    docs/station/      # brief, design, decisions e plan do workspace
    docs/station/workflows/ # instruções versionadas dos cinco workflows
    .agents/skills/     # skills locais do repositório descobertas pelo Codex
    AGENTS.md           # instruções concisas de escopo do monorepo
    schemas/           # schema versionado do workspace
    workspace.json

`station new` cria a árvore, manifesto, instruções AI e skills locais sem instalar pacotes e sem inventar o module path Go. A raiz recebe `AGENTS.md` curto; `apps/mobile/AGENTS.md`, `apps/web/AGENTS.md` e `apps/server/AGENTS.md` fornecem instruções específicas de cada área e apontam para a skill correspondente. Como `station new` não inicializa Git, a instrução raiz deve dizer ao agente para iniciar no diretório raiz do workspace e ler o `AGENTS.md` da área antes de trabalhar nela: a documentação atual do Codex diz que, sem raiz de projeto detectável, a busca de instruções verifica apenas o diretório atual. Em execução a partir da raiz, as skills locais ficam no próprio caminho de descoberta `.agents/skills`; a Station não afirma que um host as carregou. As skills versionadas ficam em `.agents/skills/station-mobile/SKILL.md`, `.agents/skills/station-web/SKILL.md` e `.agents/skills/station-server/SKILL.md`. Suas instruções são específicas da stack e orientam descoberta dos comandos/versões reais; não criam comandos não declarados nem autorizam efeitos externos. Isso usa as convenções oficiais de `AGENTS.md` em camadas e `.agents/skills` do Codex, mas não presume que outros hosts as reconheçam.

Os cinco arquivos de workflow definidos na seção 5 são parte dos assets da Station e da scaffold inicial. O workflow `build`, após design aprovado, pode permitir comandos de scaffold declarados no manifesto. Esses comandos ficam visíveis e reproduzíveis; não são executados escondidos.

Recomendação técnica para o primeiro scaffold real, sujeita ao design do projeto:

- mobile: estrutura contratual e instruções Expo/React Native com TypeScript; um app funcional ou template externo só via comando declarado e após pesquisa atual;
- web: estrutura contratual e instruções React + TypeScript; bundler fica decisão do design, sem Create React App;
- server: estrutura contratual e instruções Go; `go.mod` só depois de receber um module path explícito;
- shared: package TypeScript sem dependência de plataforma, caso o design defina contratos compartilhados.

Essa scaffold inicial não cria um app de tarefas e não transforma a Station em gerador universal. Ela fixa a forma mínima e as instruções que a Station valida e orienta. Antes de escolher ferramentas concretas para um projeto futuro, o agente deve pesquisar documentação oficial vigente, comparar apenas alternativas que mudem a decisão e verificar separadamente a disponibilidade local.

## 8. Repositórios externos read-only

O loader deve aceitar somente referências locais verificáveis na V1:

    {
      "id": "reference-repo",
      "path": "../reference-repo",
      "readOnly": true,
      "allowedReadPaths": ["docs/**", "src/**"]
    }

O path externo pode ficar fora da raiz do workspace para leitura, mas deve ser resolvido e listado explicitamente. Nenhum writePath de workflow pode intersectar um repositório externo read-only. Se o mesmo path aparecer como projeto interno e externo, a validação bloqueia. readOnly: false exige uma regra de externalChanges explícita e continua em ask até que uma etapa posterior implemente suporte técnico; o valor não transforma escrita em operação automática.

## 9. Contexto e handoffs

Não haverá Context Engine separado. `internal/workflows/context.go` deve montar um `ContextPacket` com:

    {
      workflowId,
      workspaceRoot,
      inputs: [{ path, sha256, bytes, lineCount, role }],
      summaries: [{ path, summaryKind, content }],
      allowedSkills,
      allowedCommands,
      readPaths,
      writePaths,
      expectedOutputs,
      nextHandoff
    }

O pacote inclui a instrução versionada do workflow selecionado, mais conteúdo somente de arquivos pequenos explicitamente classificados como input; para os demais inclui metadados e resultados de comandos declarados. Ele não inclui `.env*`, segredos, node_modules, caches, binários ou todo o repositório. O resumo prefere artefatos, diff/snapshot, arquivos relevantes e resultados de comandos. A saída deve informar quais dados foram omitidos e que a Station não controla o que um host independente transmite.

Handoffs determinísticos:

    brainstorm → docs/station/brief.md
    grill-me   → docs/station/decisions.md
    design     → docs/station/design.md
    build      → docs/station/plan.md + paths alterados + design.md
    review     → snapshot/diff + testes + decisions.md

Cada output deve ter path, existência, hash antes/depois e status. `run` retorna um `run-id` rastreável no log local; `verify` exige o mesmo ID, garante que ele corresponde ao workflow e compara o snapshot antes/depois. Um run-id concluído não pode ser verificado de novo como ativo. A CLI não declara workflow concluído apenas porque o preflight passou; o postflight precisa encontrar os outputs e detectar/reportar alterações fora do contrato quando observáveis. Não afirmar que preveniu writes feitos diretamente pelo host.

## 10. Testes e verificações

### Unitários

Usar o pacote `testing` da biblioteca padrão Go e fixtures locais. Os testes unitários não devem depender de rede. A suíte deve cobrir:

- parser: help, version, comandos válidos, flags desconhecidas e exit codes;
- loader: JSON inválido, schema inválido, schemaVersion desconhecido e diagnóstico com path;
- semântica: IDs duplicados, projeto fora da raiz, workflow apontando para comando inexistente, skill obrigatória fora da allowlist;
- skills locais: ID permitido aponta para `.agents/skills/<id>/SKILL.md` válido quando local, metadado obrigatório ausente resulta em diagnóstico; presença não é reportada como prova de carregamento do host;
- paths: traversal, absoluto, symlink externo, glob de leitura/escrita e colisão com log/audit;
- comandos: executável/args vetorizados, comando não permitido, ask, deny e nenhuma chamada a shell;
- políticas: precedência projeto > path > categoria > default e os três resultados;
- segredo: exclusão de .env*, detecção PEM e redaction sem vazamento no erro/audit;
- contexto: exclusão de segredos/caches, hash determinístico, resumo mínimo e handoff com outputs;
- auditoria: JSONL append-only, campos redigidos e nenhum valor secreto;
- scaffold: não sobrescrever arquivo existente, gerar apenas paths permitidos e preservar input.

Exemplo de contrato de teste para a precedência da política:

    func TestSecretRuleOverridesProjectAllow(t *testing.T) {
        decision := authorize(Action{
            Kind: "data-send",
            Path: ".env.local",
            DataCategory: "secret",
            Provider: "local",
        }, manifestFixture)

        if decision.Decision != "deny" {
            t.Fatalf("decision = %q, want deny", decision.Decision)
        }
        if decision.MatchedRule != "data.secret.default" {
            t.Fatalf("matched rule = %q, want data.secret.default", decision.MatchedRule)
        }
    }

### Integração

Os testes de integração devem criar workspaces temporários com `t.TempDir`/`os.MkdirTemp` e gravar manifestos mínimos. O harness compila uma vez o binário com `go build -o <binário-temporário> ./cmd/station` e executa-o usando `os/exec` com executável e argumentos separados; não precisa copiar schema para o fixture, pois ele está embutido. Deve verificar stdout, stderr, exit code e árvore final. `os/exec` não invoca shell implicitamente; a CLI não deve usar `sh -c` nem equivalente. Casos obrigatórios:

1. manifesto válido retorna 0 e nenhum write extra;
2. manifesto inválido retorna 2 e não cria auditoria;
3. `station workflow run brainstorm` sem brief.md retorna 5;
4. `station workflow run build --command server.test.ask`, usando um command ID de fixture cuja política é ask, retorna 3 e não executa processo;
5. write iniciado pela própria CLI fora de writePaths retorna 4 e mantém a árvore original;
6. repositório externo é legível quando permitido e qualquer tentativa de escrita iniciada pela CLI retorna 4;
7. ausência de Git usa snapshot sem executar git init;
8. `station inspect` sem workspace.json executa somente descoberta read-only da allowlist, sem criar arquivos nem executar comandos;
9. comando allow inicia uma vez o executável declarado com argv/cwd exatos e sem interpretação de shell.

### Aceitação ponta a ponta

O cenário de aceitação deve usar um workspace temporário e uma ideia genérica, não o app Tiimo:

    1. executar `station new "uma ideia de aplicação"` em um diretório temporário vazio;
    2. confirmar workspace.json, schema e diretórios apps/mobile, apps/web, apps/server e docs/station, sem dependências instaladas; confirmar packages/shared somente se o design da fixture o declarar;
    3. confirmar AGENTS.md da raiz e das três áreas, cinco arquivos de instrução de workflow e três skills locais descobríveis sob `.agents/skills`, e que `workspace.json` lista IDs correspondentes;
    4. executar `station validate` e confirmar 0;
    5. simular o agente criando brief.md, decisions.md e docs/station/design.md por um harness de teste;
    6. executar `station workflow run <id>`, simular o agente alterando artefatos e executar `station workflow verify <id> <run-id>` para os cinco workflows, confirmando instruções, inputs, skills, comandos, contexto, outputs e divergências;
    7. executar um comando de teste declarado com executável/argv exatos e confirmar que comando ask/deny não inicia processo;
    8. executar inspeção de existing com workspace.json ausente e confirmar leitura apenas da allowlist e nenhuma escrita;
    9. simular alteração fora de writePaths e confirmar detecção no postflight, sem afirmar prevenção quando o agente escreve diretamente;
    10. confirmar que a CLI não fez chamada de modelo/API, login, clone, push, PR, deploy ou criação do app de tarefas.

Como este diretório não possui lint/build configurados, as verificações equivalentes serão `gofmt -w` nos arquivos Go alterados, `go test ./...`, `go vet ./...` e `go build ./cmd/station`, além da aceitação acima. A tarefa de implementação só poderá declarar uma fase concluída após executar os comandos aplicáveis e registrar os resultados reais.

## 11. Dogfooding posterior com o app inspirado no Tiimo

O dogfooding ocorrerá em um workspace separado, depois que o núcleo da Station estiver verificável. O ponto de partida poderá ser a ideia curta aprovada na especificação:

> Quero criar um app de tarefas parecido com o Tiimo.

O procedimento será:

1. criar um workspace novo com station new;
2. usar brainstorm para produzir brief, critérios de sucesso e limites sem pré-carregar um contrato funcional do app;
3. usar grill-me para desafiar ambiguidades, riscos, privacidade, casos extremos e decisões pendentes;
4. usar design para definir somente o próximo incremento implementável;
5. usar build em lotes com paths, comandos, testes e artefatos declarados;
6. usar review para verificar diffs, testes, design, governança de dados e aderência ao manifesto;
7. registrar o que a própria Station facilitou ou bloqueou em decisions.md do workspace do app.

Este plano não cria telas, entidades, fluxos, contrato de API, backlog ou especificação funcional do app. A validação é da Station: workflows executados, artefatos gerados, workspace.json evoluído, scaffold local, testes e respeito às políticas.

## 12. Fases de implementação, dependências e conclusão observável

### Task 0: Fase 0 — Pesquisa de implementação, baseline e isolamento

Dependência: nenhuma.

- [ ] Reinspecionar a árvore, ler design.md e plan.md completos, verificar alterações desde a aprovação e preservar qualquer arquivo inesperado.
- [ ] Verificar localmente versões de Go, Git e ferramentas; separar disponibilidade local de documentação externa.
- [ ] Antes de fixar dependências/API/flags, atualizar pesquisa de fontes primárias para releases da toolchain Go, Cobra, validador Draft 2020-12 e convenções atuais do Codex para AGENTS.md e `.agents/skills`; registrar data, URL, informação, escolha, alternativa/limitação e estado local.
- [ ] Reavaliar nomes, hierarquia e flags CLI (`new`, `inspect`, `validate`, `workflow run/verify`, `--format`, `--help`, `--version`) contra padrões documentados e CLIs originais. Se algum nome/forma não estiver claro ou convencional, atualizar este plano antes da implementação do comando, preservando os modos e responsabilidades de produto aprovados.
- [ ] Rever cada decisão técnica do plano e marcar uma de três origens: requisito do design, instrução do usuário, ou decisão de implementação a revalidar. Se pesquisa atual contradisser a recomendação, alterar o plano com evidência antes de implementar essa parte. Usar toolchain Go suportada em release de manutenção corrigida, pois a diretiva `go` não fixa patch.
- [ ] Não pesquisar decisões de produto já aprovadas para reabri-las. Não enviar dados do projeto, não fazer login nem mutação externa; usar somente fontes públicas e leitura.
- [ ] Como o usuário autorizou esta implementação, iniciar Git local no diretório somente após snapshot/inspeção, criar `.gitignore` restrito a worktrees/artefatos de execução, registrar estado inicial em commit-base e preparar branch/worktree de implementação. Não inicializar Git em workspaces criados futuramente pela Station.
- [ ] Antes de editar qualquer caminho planejado, verificar se já existe; comparar e adaptar quando necessário em vez de sobrescrever.

**Critério de conclusão:** pesquisa datada foi atualizada para decisões técnicas mutáveis; Git/worktree local é verificável; o commit-base conserva o design e plano existentes; nenhum efeito fora do escopo local ocorreu.

### Task 1: Fase 1 — Pacote CLI mínimo e contrato de schema

Dependência: Fase 0.

**Arquivos:**

- Criar: go.mod, go.sum, cmd/station/main.go.
- Criar: internal/assets/embed.go, internal/assets/workspace.schema.json.
- Criar: internal/cli/root.go, internal/cli/validate.go, internal/cli/errors.go.
- Criar: internal/workspace/load.go, internal/workspace/validate.go.
- Criar: internal/cli/parse_test.go, internal/workspace/validate_test.go, internal/assets/embed_test.go, testdata/workspace.valid.json, testdata/workspace.invalid.json.

**Interfaces:**

    loadWorkspace(root string) (*Workspace, error)
    validateWorkspace(manifest *Workspace, root string) []Diagnostic
    runCLI(ctx context.Context, argv []string, deps Dependencies) Result

- [ ] Escrever testes para `station --help`, `station --version`, `station workflow --help`, `station validate`, comando desconhecido, JSON inválido, schema inválido e manifesto válido. Usar subcomandos na hierarquia definida na seção 4.
- [ ] Rodar `go test ./cmd/station ./internal/cli ./internal/workspace`; confirmar falhas de compilação/teste antes da implementação.
- [ ] Conferir documentação/release e compatibilidade atual do Cobra e do validador JSON Schema Go para Draft 2020-12; confirmar versões/licenças/API consultadas neste plano, verificar grafo final de módulos e disponibilidade local, e registrar qualquer mudança antes de adicionar dependências.
- [ ] Criar o módulo Go e fixar as versões efetivamente consultadas em go.mod/go.sum. Definir o piso de Go conforme política de suporte vigente e ambiente local revalidado, sem copiar o número antigo deste plano automaticamente.
- [ ] Implementar a árvore CLI com Cobra e erros tipados para invalid, ask, deny, artifact-missing e prerequisite-missing. Manter os códigos documentados consistentes em help/docs/testes.
- [ ] Implementar schema Draft 2020-12 local/estrito como asset embutido, com os campos das seções 3 e 6; configurar `DefaultDraft(Draft2020)`, pré-carregar o asset e `UseLoader(nil)` para que refs externos não abram arquivos nem rede. Não usar `format` na V1; se isso mudar, habilitar `AssertFormat()` explicitamente e testar. Só rejeitar campos desconhecidos quando houver `additionalProperties: false` deliberado no contrato. Testar que cópia de schema gerada permanece idêntica ao asset.
- [ ] Implementar loader que recuse raiz ausente quando obrigatória, workspace.json ausente, JSON inválido e schemaVersion sem schema embutido; não buscar schema ou referências na rede.
- [ ] Implementar validação semântica de IDs, paths, referências de comando/workflow/skill e kind: monorepo.
- [ ] Rodar `gofmt`, `go test ./...`, `go vet ./...` e `go build ./cmd/station`; registrar se a resolução da dependência exige rede e não mascarar falha de disponibilidade.

**Critério de conclusão:** o binário `station validate` valida fixture válido, rejeita fixtures inválidos com diagnóstico localizado e nenhum comando de projeto é executado.

### Task 2: Fase 2 — Motor de políticas, caminhos, comandos e auditoria

Dependência: Fase 1.

**Arquivos:**

- Criar: internal/policy/decision.go, internal/policy/paths.go, internal/policy/commands.go, internal/policy/data.go, internal/policy/secret_patterns.go.
- Criar: internal/audit/write.go.
- Criar: internal/policy/decision_test.go, internal/policy/paths_test.go, internal/policy/commands_test.go, internal/policy/secrets_test.go, internal/audit/write_test.go.
- Alterar: internal/cli/validate.go e internal/cli/parse_test.go para validar a forma/decisões das policies além do schema estrutural.

**Interfaces:**

    authorize(action Action, manifest *Workspace, workflowID string) Decision
    assertWorkspacePath(root, declaredPath string, mode PathMode) (absolutePath, matchedRule string, err error)
    authorizeCommand(commandID, workflowID string, manifest *Workspace) (Decision, Command)
    scanData(path, content, category string, policy DataPolicy) ScanResult
    appendAudit(root string, event AuditEvent) (eventID, path string, err error)

- [x] Escrever testes para cada regra do foco de revisão: traversal, symlink externo (incluindo symlink final com barra conforme advisory Go vigente), comando não declarado, ask, deny, segredo e repositório externo.
- [x] Rodar os testes e confirmar falhas por funções ausentes.
- [x] Implementar validação lexical de paths e fazer I/O da Station relativo a `os.Root` ancorado em workspace ou repo externo; rejeitar absoluto, `..`, symlink fora da raiz e qualquer escrita pela CLI na raiz externa. Não alegar isolamento de processos, mounts ou devices.
- [x] Implementar autorização por precedência e os três resultados allow/ask/deny; não usar booleano que confunda ask com allow.
- [x] Fazer `station validate` validar os contratos execution/data, referências de projeto/comando e regras secret/unknown antes de qualquer execução.
- [x] Implementar comando como { executable, args[] }, rejeitando string shell, argumentos não declarados e workflow sem permissão.
- [x] Implementar exclusão de `.env*` e detecção PEM redigida no pacote de contexto, retornando somente tipo/path/linha/hash; não implementar heurística ampla de token neste núcleo.
- [x] Implementar .station/audit.jsonl append-only, criando o diretório somente depois de uma decisão válida e nunca para manifestos inválidos.
- [x] Rodar `go test ./...`; fixtures comprovam que o valor secreto não aparece no resultado serializado do scanner nem na auditoria. A verificação de stdout/stderr ficará junto à publicação do ContextPacket na Fase 3, pois a Fase 2 não imprime nem expõe conteúdo de arquivo.

**Critério de conclusão:** operações próprias da CLI e comandos iniciados por ela recebem decisão explícita; nenhuma operação negada é iniciada pela CLI. Writes feitos diretamente por um host externo não são prevenidos, mas a diferença de snapshot é reportada quando observável.

### Task 3: Fase 3 — Workflows, contexto e handoffs

Dependência: Fases 1 e 2.

**Arquivos:**

- Criar: internal/workflows/definitions.go, internal/workflows/context.go, internal/workflows/run.go.
- Criar: internal/workflows/workflows_test.go, internal/workflows/context_test.go, test/integration/workflow_handoff_test.go.

**Interfaces:**

    getWorkflowDefinition(id string, manifest *Workspace) (WorkflowDefinition, error)
    prepareWorkflow(root, id string, input Input) (ContextPacket, runID string, err error)
    verifyWorkflow(root, id, runID string, manifest *Workspace) (WorkflowResult, error)
    snapshotWorkspace(root string, paths []string) (Snapshot, error) // entries: sha256, bytes, mtime

- [x] Escrever testes para os cinco workflows, inputs/outputs ausentes, skill obrigatória fora da allowlist, run-id inválido/reutilizado e write fora de writePaths.
- [x] Rodar os testes; falhas RED capturadas durante a fase e seus limites estão registradas no ledger da execução.
- [x] Implementar as definições de brainstorm, grill-me, design, build e review com os paths e artefatos da tabela da seção 5.
- [x] Implementar ContextPacket com hashes, line count, resumos mínimos, allowlists e handoff; excluir segredos, caches, node_modules e binários.
- [x] Implementar snapshot de filesystem quando Git estiver ausente e adapter read-only de git diff quando o repositório for confirmado.
- [x] Implementar preflight que registra run-id + hashes apenas de inputs/outputs/readPaths/writePaths declarados, dentro de `.station/audit.jsonl`; não persistir conteúdo nem segredos.
- [x] Implementar `workflow verify` com run-id preparado, workflow correspondente, outputs presentes/alterados e divergências de path detectadas; acrescentar evento de conclusão e rejeitar reuso do run-id.
- [x] Fazer `station workflow run <id> --command <id>` usar authorizeCommand sem iniciar shell/processo quando a decisão não for allow; imprimir instruções versionadas e handoff de contexto sem acionar IA.
- [x] Rodar `go test ./...` e verificar o cenário `brainstorm` com manifesto de fixture: input obrigatório ausente resulta em bloqueio/código 5 sem auditoria operacional.

**Critério de conclusão:** cada workflow expõe instrução versionada, contrato verificável e handoff mínimo; o CLI não declara conclusão com output ausente e sinaliza writes observáveis fora da política, sem alegar bloqueio de ações do host.

### Task 4: Fase 4 — Modos new/existing e monorepo

Dependência: Fases 1–3.

**Arquivos:**

- Criar: internal/modes/new.go, internal/modes/existing.go.
- Criar: internal/scaffold/monorepo.go, internal/scaffold/embed.go e templates da árvore mínima, `AGENTS.md`, os três `apps/*/AGENTS.md`, instruções versionadas dos cinco workflows e skills `.agents/skills/station-{mobile,web,server}/SKILL.md`. Criar template `packages/shared/README.md` somente se o design da fixture declarar `shared`.
- Criar: test/integration/new_mode_test.go, test/integration/existing_mode_test.go, test/acceptance/station_v1_test.go.

**Interfaces:**

    createNewWorkspace(root, idea string, options NewOptions) (NewResult, error)
    inspectExistingWorkspace(root string) (ExistingInspection, error)
    scaffoldMonorepo(root string, design Design, policy Policy) (ScaffoldResult, error)

- [ ] Escrever testes para `new` em diretório vazio, recusa sem sobrescrever arquivo existente, preservar conteúdo, copiar schema/workflows/AGENTS/skills embutidos e não instalar dependências; testar `inspect` sem manifesto como read-only e sem comandos.
- [ ] Rodar os testes e confirmar falha inicial.
- [ ] Antes de finalizar cada skill local, pedir a um subagente independente que resolva um cenário realista da respectiva área sem a skill e registrar lacunas observáveis; repetir com a skill e as instruções do diretório, verificando que escolhe apenas comandos realmente declarados e respeita limites de ownership. Se o teste não mostrar ganho útil, mover o detalhe permanente para AGENTS.md ou remover a skill redundante.
- [ ] Implementar new para criar apenas diretórios/README/manifesto/schema aprovados, copiar schema/templates embutidos, gravar a ideia como input de contexto e não criar contrato funcional do produto.
- [ ] Implementar `inspect` como modo `existing`: sem manifesto, leitura read-only rasa da allowlist definida na seção 4; com manifesto, ler apenas readPaths. Nunca executar comandos nessa descoberta inicial nem inicializar Git.
- [ ] Implementar scaffold monorepo com roles mobile/web/server e shared opcional segundo o design; exigir module path explícito antes de gerar apps/server/go.mod.
- [ ] Implementar configuração de comandos vazia ou declarada explicitamente; não invocar npx, npm create, Expo, Vite, Go download ou qualquer rede de forma implícita.
- [ ] Adicionar ao CLI `new`, `inspect`, `validate`, `workflow run` e `workflow verify`, mantendo validate sem efeitos colaterais.
- [ ] Rodar a aceitação em diretório temporário, confirmar a árvore, validar o manifesto e provar que o diretório de origem não foi alterado além dos arquivos esperados.

**Critério de conclusão:** um workspace vazio recebe scaffold contratual mínimo e instruções/skills descobríveis por Codex; um workspace existente é analisado sem destruição; nenhum app de tarefas é criado.

### Task 5: Fase 5 — Verificação completa e documentação operacional

Dependência: Fases 1–4.

**Arquivos:**

- Modificar: README.md com instalação local, comandos, limites e exemplo de manifesto.
- Modificar: docs/station/plan.md somente se a revisão encontrar inconsistência real antes da aprovação.
- Criar: test/acceptance/no_external_side_effects_test.go se a cobertura de efeitos externos não puder permanecer no cenário principal.

- [ ] Rodar `go test ./...` com toda a suíte unitária, integração e aceitação.
- [ ] Rodar `gofmt -w` nos arquivos Go modificados, `go vet ./...` e `go build ./cmd/station`.
- [ ] Rodar a validação de fixture monorepo e, quando o fixture tiver module path, go test ./... no servidor mínimo.
- [ ] Confirmar que não existe chamada própria a API de modelo, login, token literal de autorização, push, PR, deploy, clone ou escrita em repositório externo.
- [ ] Revisar com `git diff --check`, `git status` e leitura do diff em cada commit e antes da integração local; confirmar que a mudança de plano/commit-base não incorporou alterações imprevistas.
- [ ] Reler design.md e este plano, conferir cada requisito da seção 5 do Prompt 1, remover marcadores incompletos, comandos não suportados, escopo excessivo e dependências não declaradas.
- [ ] Registrar em README.md as limitações reais: presença de skills locais não prova que o host as carregou, scaffold externo não automático, module path Go obrigatório e enforcement de ferramentas do host fora da CLI.

**Critério de conclusão:** todos os testes e verificações aplicáveis passam com saída registrada; qualquer falha ou ferramenta ausente permanece listada como limitação, sem afirmar conclusão indevida.

## 13. Arquivos previstos e responsabilidades

| Arquivo/diretório | Responsabilidade |
|---|---|
| go.mod / go.sum | módulo, versão da toolchain declarada e dependências efetivamente resolvidas |
| cmd/station/main.go | entrypoint compilável, sem lógica de domínio |
| internal/assets/workspace.schema.json | schema canônico Draft 2020-12 embutido no binário e copiado aos workspaces novos |
| internal/cli/* | árvore Cobra, help, despacho e exit codes |
| internal/workspace/* | leitura, schema e validação semântica |
| internal/policy/* | decisões, paths, comandos e dados |
| internal/audit/* | auditoria local redigida |
| internal/workflows/* | contratos, contexto, snapshots e handoffs |
| internal/modes/* | entrada de projeto novo e existente |
| internal/scaffold/* | monorepo mínimo com AGENTS.md, skills locais e templates seguros embutidos para `station new` |
| internal/assets/workflows/* | instruções versionadas dos workflows impressas pela CLI |
| *_test.go | comportamento isolado e regras de segurança |
| test/integration/* | CLI, manifesto, workflows e filesystem temporário |
| test/acceptance/* | fluxo ponta a ponta sem efeitos externos |
| README.md | uso local e limitações observáveis |
| AGENTS.md, apps/*/AGENTS.md | instruções AI específicas do repositório e dos diretórios da stack |
| .agents/skills/station-*/SKILL.md | skills locais de manutenção mobile, web e server |
| .station/audit.jsonl | proposta de log operacional local; nunca parte do manifesto |

Se algum desses caminhos já existir na implementação, o executor deve preservar o conteúdo e adaptar o plano com evidência; não deve sobrescrever silenciosamente.

## 14. Riscos, decisões abertas e recomendações

### Riscos principais

- **Enforcement incompleto:** uma CLI não intercepta automaticamente ferramentas externas usadas pelo agente. Mitigação: decisões estruturadas, comandos vetorizados, paths verificados, auditoria e hooks determinísticos quando o host oferecer; declarar a limitação em toda aceitação.
- **Deriva do contrato:** workflows podem ser alterados sem atualizar schema/testes. Mitigação: schema estrito, fixture válido, station validate no início e fim de cada fase.
- **Vazamento de segredo por contexto/auditoria:** mitigação: scanner antes de resumo, redaction por hash/linha, exclusão de .env, testes de não vazamento e default deny.
- **Scaffold externo mutável:** templates Expo/Vite e versões de Node mudam. Mitigação: não embutir versões sem lockfile, não executar geradores implicitamente, registrar comando/versão quando explicitamente permitido. Node é requisito potencial dos projetos web/mobile, não da CLI Go.
- **Module path Go ausente:** mitigação: bloquear somente o scaffold do servidor até receber valor explícito; não preencher endereço fictício como se fosse publicável.
- **Ausência de Git:** mitigação: snapshot de filesystem e critérios de hash; não declarar diff Git inexistente.
- **Skills locais não carregadas pelo host:** a CLI valida presença/metadados dos arquivos locais, mas não afirma que Codex ou outro host os carregou ou invocou.
- **Enforcement fora da CLI:** tools do host podem escrever ou enviar dados sem passar pelo processo da Station. Mitigação V1: instruções persistentes, contexto omitindo segredos, execução de comandos somente via allowlist quando usada e comparação postflight; um sandbox real fica fora da V1.
- **Lock de verificação obsoleto:** se o processo terminar abruptamente durante `workflow verify`, o lock por run pode permanecer e bloquear nova verificação como `ask`. A V1 reporta a condição sem removê-lo automaticamente; só remover após confirmar que não há verificação ativa.
- **Classificação de binário por prefixo:** a heurística de snapshot inspeciona no máximo o primeiro MiB para decidir se calcula hash integral; conteúdo binário que comece depois desse prefixo pode não ser classificado como binário. O conteúdo de arquivos acima do limite de input (64 KiB) não é incluído no contexto, mas seus metadados/hash podem aparecer; não tratar essa classificação como garantia de formato.

### Decisões recomendadas

- Usar Go para a CLI V1; isso favorece executável compilado e evita exigir runtime Node para invocar a Station. A vantagem de performance não foi medida e não é critério V1; Node permanece alternativa viável.
- Usar Cobra para a árvore de comandos somente após revalidar compatibilidade, suporte e versão atuais; o padrão pesquisado favorece subcomandos por recurso e verbos de ação.
- Usar JSON Schema Draft 2020-12 com schema local versionado e validação runtime; não aceitar schema remoto como dependência operacional.
- Usar `testing`, `go test` e `go vet` da ferramenta Go para o núcleo de testes, sem framework de teste externo.
- Manter dependências externas ao mínimo: biblioteca JSON Schema compatível com Draft 2020-12, com versão registrada em `go.mod`/`go.sum`; parser e demais funções usam biblioteca padrão quando suficiente.
- Manter scaffold inicial determinístico e sem rede; integrar Expo/Vite somente via comandos declarados e autorizados pelo workspace.
- Manter o default de repositórios externos read-only e sem clone/fetch na V1.

### Decisões abertas que devem permanecer visíveis

- Module path do servidor Go de cada projeto novo.
- Versões concretas de Expo/React Native, React, TypeScript e Vite no momento do dogfooding; devem ser escolhidas pelo design do projeto e registradas no lockfile, não neste plano.
- Se e quando um host fornecer hooks para interceptar comandos do agente; a ausência do hook não pode ser apresentada como enforcement completo.
- Política final de providers externos e categorias de dados para um projeto que realmente autorize envio; a V1 da Station só prepara e registra preflight.

### Decisões já resolvidas para esta implementação

- Bootstrap de `station inspect` sem manifesto: inventário imediato e leitura limitada à allowlist da seção 4; nenhum comando, write, symlink externo ou varredura recursiva. Workflows ficam indisponíveis até que o `workspace.json` de política seja criado/revisado e validado.
- Skills locais do monorepo: `station-mobile`, `station-web`, `station-server`, em `.agents/skills`, acompanhadas por AGENTS.md raiz e por área. O manifesto pode permitir esses IDs; não precisa torná-los obrigatórios por padrão.
- Nomes CLI: `station new`, `station inspect` e `station workflow run`; o modo interno continua `existing`. Os nomes preservam a semântica aprovada e seguem a hierarquia observada em CLIs maduras.
- A autorização pontual para iniciar Git aplica-se somente ao repositório Station desta implementação, não a `station new` nem a workspaces gerados.

## 15. Pesquisa, proveniência e validação contínua

A implementação deve pesquisar de forma abrangente as decisões de engenharia que ainda possam mudar, em vez de simplesmente repetir as recomendações deste plano. “Abrangente” significa cobrir cada ponto de decisão abaixo com fontes atuais e verificar a disponibilidade local separadamente; não significa pesquisar indiscriminadamente. Decisões de produto já aprovadas permanecem fechadas salvo contradição concreta. A pesquisa é somente leitura: sem login, envio de dados do projeto, criação de recursos, chamadas a APIs/modelos ou efeitos externos.

Antes de escolher ou fixar biblioteca, versão, API ou comportamento dependente de versão, o executor deve:

1. nomear a dúvida concreta e indicar qual decisão ela pode alterar;
2. consultar documentação oficial, especificação ou repositório original atual; usar uma segunda fonte primária se a primeira não resolver a dúvida;
3. comparar somente alternativas tecnicamente viáveis que possam mudar a decisão, registrando limitações e motivo da escolha;
4. verificar separadamente se ferramenta/dependência existe localmente, sem inferir isso da documentação externa;
5. registrar URL/identificador, fato extraído, decisão influenciada, data da consulta, alternativas/limitações e status verificado/não verificado no relatório/ledger; atualizar este plano ou README para decisões duradouras;
6. quando fonte confiável não existir ou não resolver, marcar `unverified`, não inventar resultado e avançar apenas por escolha conservadora que não altere materialmente o escopo.

Pontos obrigatórios antes das fases correspondentes: política de suporte/toolchain Go; versão, licença e API do Cobra; dialeto JSON Schema e compatibilidade/licença/versão do validador Go; descoberta de `AGENTS.md` e `.agents/skills` pelo Codex; limites de segurança das APIs Go de filesystem/processos; e documentação vigente de ferramentas Expo/web caso integração real passe a ser considerada. A pesquisa sobre Expo/Vite não pode introduzir scaffold de rede no `station new` sem decisão do design. A CLI Station não exige Node.

Proveniência:

- **Do design aprovado (não reabrir por pesquisa):** CLI local/Codex-first; sem APIs próprias de IA/Project Store; workflows e ordem; stack aprovada; resultados allow/ask/deny; read-only para externos; app Tiimo somente como dogfooding posterior.
- **Das instruções do usuário:** validar workspace, workflows, skills, comandos, paths e outputs; testar por camadas; pesquisar CLI; produzir instruções/skills para o monorepo; commits locais atômicos.
- **Do estado local observado no planejamento:** apenas design.md inicialmente; sem Git antes da autorização posterior para criá-lo. Toolchains e dependências devem ser rechecadas imediatamente antes de fixação.
- **De fontes primárias consultadas:** Draft 2020-12, recursos/documentação Go e Expo/React/Vite no plano anterior; hierarquia de comandos Cobras/GitHub CLI/Docker; convenções atuais Codex de AGENTS.md e `.agents/skills`. Links e efeito estão discriminados na seção 16.
- **Decisões internas ainda não impostas por uma fonte externa:** modelo de dados workspace.json, handoff, saída JSON, auditoria e precedência de políticas. Devem ser justificados contra design.md e provados por testes; não são fatos sobre o host.

Comparação de nomenclatura CLI pesquisada e adotada:

| Candidato anterior | Evidência de convenção | Decisão e motivo |
|---|---|---|
| `station run <workflow>` | GitHub CLI agrupa execução em `gh workflow run`; Cobra define comando-pai/filho. | Alterado para `station workflow run <workflow-id>`, recurso seguido da ação, mais extensível e encontrável na ajuda. |
| `station existing` | `existing` é o ID do modo aprovado, mas descreve estado em vez de ação. CLIs usam verbos como `inspect`, `view`, `list`. | Alterado para `station inspect`; continua roteando para o modo interno `existing`, sem mudar o fluxo de produto. |
| Verificação de workflow após a edição do agente | GitHub CLI documenta subcomandos `gh attestation verify` e `gh release verify`; usam recurso + verbo `verify`. | Adicionar `station workflow verify <id> <run-id>` para validar outputs pós-handoff sem chamar IA; a operação será local, sem a busca remota que alguns comandos gh fazem. |
| `station new`, `station validate` | Docker documenta `docker init`; CLIs maduras usam verbos curtos para criar/verificar. | Mantidos: verbos diretos e escopo claro. |
| `--format json`, `--help`, `--version` | Docker `inspect` e `gh release verify` aceitam `--format json`; Kubernetes usa `-o`/`--output json`; Cobra documenta `--help` e `--version`. | Manter `--format json` nos comandos com saída estruturada, `--help` e `--version`; não adicionar alias `-o`/`--output` na V1. São opções de uso comum e testáveis na interface Cobra escolhida. |

Arquitetura, workflows e precedência de políticas não foram “pesquisados para confirmar”: são derivadas da especificação ou decisões de implementação deste plano. O desenho enumera as origens com transparência, e a fase de pesquisa deve validar os elementos técnicos que ainda podem mudar sem reabrir requisitos aprovados.

| Área questionada | De onde veio a decisão | Nível de pesquisa/verificação |
|---|---|---|
| Arquitetura Station/agente, CLI local, sem Project Store/model API | `design.md` | Derivada da especificação; decisão de produto não reaberta por pesquisa. |
| Comandos e flags | IDs de modo `new`/`existing` vêm da especificação; nomes de terminal são proposta técnica. | Pesquisa feita em 2026-09-20 em Cobra, GitHub CLI, Docker e kubectl; formas selecionadas são precedentes compatíveis, não norma universal. Revalidar a interface nos testes e trocar nomes técnicos fracos antes de implementar sem mudar semântica do produto. |
| Workflows e responsabilidades | Nomes/ordem vêm do `design.md`; I/O e arquivos de instrução são detalhes de implementação. | Sem pesquisa externa para mudar produto. Bootstrap de `inspect` foi resolvido como inventário read-only superficial na seção 4. |
| Skills e instruções locais | O contrato de allow/required skills vem da especificação; os três IDs locais são decisão desta implementação. | Convenções Codex pesquisadas em 2026-09-20; `.agents/skills` é descobrível, mas arquivo local não prova que host carregou a skill. |
| `workspace.json` e validação | Campos conceituais vêm da especificação; Draft 2020-12, biblioteca Go candidata e comportamento do `$schema` foram consultados em fontes primárias. | Pesquisa técnica direcionada; a dependência foi verificada upstream, não instalada no ambiente. |
| Políticas, segredos e auditoria | Resultados allow/ask/deny, governança e read-only externo vêm da especificação. | Prioridade, log e padrões mínimos são decisões internas sujeitas a teste. Limite de enforcement pelo host é inferência arquitetural explícita, não alegação de pesquisa externa. |
| Monorepo e scaffold | Mobile/Expo, web/React, TypeScript e server/Go vêm do design. Expo/Vite são recomendações técnicas consultadas; `shared` fica opcional por não ser requisito. | Compatibilidade e instruções oficiais foram pesquisadas, mas não houve comparação de ferramentas nem instalação local. |
| Repositórios externos | Read-only por padrão vem diretamente do `design.md`. | Decisão de produto aprovada, não reaberta por pesquisa. |
| Contexto e handoffs | Sem Context Engine e com handoffs por artefatos vêm do `design.md`; estrutura de `ContextPacket`, hashes e snapshots é proposta operacional. | Não houve pesquisa externa para o formato exato do pacote. |
| Testes e comandos seguros | TDD é instrução do usuário; biblioteca Go e escolha de framework serão validadas antes de implementação. | Docs oficiais Go foram consultados no plano original; refrescar APIs mutáveis e manter testes locais sem depender de rede. |
| App de dogfooding | Uso posterior do app inspirado em Tiimo vem do `design.md`. | Sem especificação funcional ou pesquisa de produto nesta etapa, intencionalmente. |
| Linguagem/performance | Go recomendado por CLI local/Codex-first e binário executável; Node segue alternativa viável. | Fontes Go confirmam compilação, não superioridade de performance. Nenhum benchmark comparável; desempenho não é critério de aceitação. |

## 16. Referências externas consultadas

As referências da coluna Data foram consultadas somente para leitura. As consultas não enviaram dados do projeto, não fizeram login, não criaram recursos nem alteraram o diretório-alvo. Referências não estabelecem disponibilidade local; essa foi verificada separadamente.

| Fonte | Informação extraída | Decisão técnica influenciada | Data | Alternativas ou limitações |
|---|---|---|---|---|
| [JSON Schema — Specification](https://json-schema.org/specification) | A especificação lista Draft 2020-12 como versão atual consultada; Core e Validation são partes separadas. | O schema versionado do workspace usa Draft 2020-12. | 2026-09-19 | A versão efetiva e a compatibilidade do validador Go devem ser fixadas/testadas durante a implementação; a referência não prova biblioteca local instalada. |
| [JSON Schema — dialect and vocabulary declaration](https://json-schema.org/understanding-json-schema/reference/schema) | `$schema` declara o dialeto usado por um documento que é schema JSON Schema; a recomendação é colocá-lo na raiz do schema. | A declaração Draft 2020-12 fica em `internal/assets/workspace.schema.json`, não é reutilizada como se fosse o caminho do validador dentro do manifesto. | 2026-09-19 | A fonte define a semântica do padrão, não associa automaticamente o manifesto ao schema da Station em editores. |
| [VS Code — Editing JSON](https://code.visualstudio.com/Docs/languages/json) | VS Code permite associar um documento a um schema pelo `$schema` no próprio JSON, mas diz explicitamente que essa sintaxe é específica do VS Code e pode fazer a validação do documento falhar. | Não adicionar `$schema` ao workspace.json como mecanismo operacional de validação; a CLI usa seu schema local fixado. Uma futura integração de editor exigiria decisão separada. | 2026-09-20 | Pode ser útil para autocomplete no VS Code, mas não é requisito Codex-first, nem padrão portátil; a alternativa documentada é mapeamento de schema na configuração do editor. |
| [Go jsonschema — releases](https://github.com/santhosh-tekuri/jsonschema/releases), [tag v6.0.3 `go.mod`](https://raw.githubusercontent.com/santhosh-tekuri/jsonschema/v6.0.3/go.mod), [licença](https://github.com/santhosh-tekuri/jsonschema/blob/v6.0.3/LICENSE), [loader](https://raw.githubusercontent.com/santhosh-tekuri/jsonschema/v6.0.3/loader.go), [Compiler API](https://raw.githubusercontent.com/santhosh-tekuri/jsonschema/v6.0.3/compiler.go) e [API versionada](https://pkg.go.dev/github.com/santhosh-tekuri/jsonschema/v6@v6.0.3) | v6.0.3 é a release mais recente consultada (2026-08-06), suporta Draft 2020-12, Apache-2.0 e `go 1.21`. A API recomenda declarar `DefaultDraft`; `AssertFormat` fica desligado por padrão em 2020-12. Metaschemas JSON Schema canônicas são embutidas no pacote; o loader padrão só abre URLs `file:`, e `UseLoader(nil)` desabilita carregamento adicional. | Fixar provisoriamente `github.com/santhosh-tekuri/jsonschema/v6` v6.0.3 e Go >=1.26 (piso para cobrir as duas séries Go atualmente suportadas, não exigência da biblioteca). Pré-carregar o schema embutido com `AddResource`, declarar `DefaultDraft(Draft2020)` e usar `UseLoader(nil)` para manter `$ref` externos indisponíveis. A V1 não usará o keyword `format`; se isso se tornar requisito depois, habilitar `AssertFormat()` explicitamente e testar. | 2026-09-20 | Não foi instalado/testado no target; conformidade do upstream é declaração do projeto e não auditoria independente. O README tagueado ainda identifica o pacote como 6.0.2, então fatos específicos de v6.0.3 foram conferidos nos metadados/releases, API e código tagueados. `FileLoader` não restringe leituras locais; o plano evita depender dele. Node/Ajv continua uma alternativa tecnicamente viável. |
| [Ajv — JSON Schema](https://ajv.js.org/json-schema) | Ajv documenta suporte a Draft 2020-12 via export separado; uma instância 2020-12 não combina diretamente com drafts anteriores. | Confirma que a escolha de Go não é imposta por incapacidade de Node/Ajv; Node segue tecnicamente viável se outros critérios prevalecerem. | 2026-09-20 | A documentação não compara cold-start nem o desempenho integral da Station, e Ajv não foi instalado/testado localmente. |
| [Go — release history](https://go.dev/doc/devel/release) e [Go toolchain security advisory GO-2026-4970](https://pkg.go.dev/vuln/GO-2026-4970) | A política oficial suporta cada major até existirem duas versões major mais novas; 1.27 e 1.26 são as séries suportadas na consulta. A release history lista Go 1.27.1 e 1.26.8 em 2026-09-01. A advisory relata escape de `os.Root` corrigido em Go 1.26.5 e 1.27.0-rc.2. | Definir Go >=1.26 como mínimo de linguagem e testar sempre com releases de manutenção corrigidas (no baseline consultado, 1.26.8 e 1.27.1); o ambiente local tem 1.27.1. Documentar que a diretiva `go 1.26` não expressa piso de patch. | 2026-09-20 | O piso envelhece a cada release; reavaliar antes de implementação/distribuição. O operador que compila deve manter a toolchain no patch seguro vigente. A Station empacotada como binário não exige toolchain Go para cada usuário final. |
| [Go — compile and install](https://go.dev/doc/tutorial/compile-install) | `go build` compila pacotes e dependências em executável. | Favorece Go para distribuição/invocação da CLI sem exigir runtime Node instalado para executar o binário. | 2026-09-20 | A documentação não mede cold-start, memória ou velocidade relativa a Node; um binário Go não é prova de superioridade de performance no workload Station. |
| [Go — PerformanceMonitoring](https://go.dev/wiki/PerformanceMonitoring) | A orientação do projeto Go é comparar desempenho com uma baseline e evitar números isolados. | Não atribuir fator de aceleração no plano; se performance virar critério de aceite, medir comandos representativos em comparação controlada. | 2026-09-20 | Não há benchmark público primário encontrado que compare implementações equivalentes desta CLI; tempo de processos de build/test pode dominar o fluxo. |
| [Go `testing`](https://go.dev/doc/tutorial/add-a-test) | A biblioteca padrão `testing` e `go test` executam testes identificados por convenção `_test.go`. | Testes da Station usam os recursos nativos Go, sem framework de teste adicional. | 2026-09-20 | Não substitui testes de aceitação do binário nem garante cobertura por si só. |
| [Go `os/exec`](https://pkg.go.dev/os/exec) | A biblioteca padrão não invoca shell nem expande glob/pipeline implicitamente; `CommandContext` permite cancelar processo por contexto; resolução de executável por PATH evita entradas relativas ao cwd via `ErrDot`. | Para comandos explicitamente autorizados, usar `CommandContext`, executável e argumentos separados, cwd declarado, ambiente deliberado e nunca concatenar entrada em shell. | 2026-09-20 | O subprocesso herda os privilégios do usuário e não é sandbox; processos filhos e writes arbitrários não ficam contidos pela política da CLI. Não sobrescrever `ErrDot` sem justificativa. Há diferenças de plataforma a testar. |
| [Go `flag`](https://pkg.go.dev/flag) | `flag.FlagSet` oferece conjuntos independentes de flags e documenta seu uso para subcomandos. | Avaliado como alternativa padrão simples; não selecionado após pesquisa de ergonomia e hierarquia CLI. | 2026-09-20 | Evitaria dependência adicional, mas exigiria implementar/testar help, erros, parsing aninhado e uso por comando. Cobra será revalidado na implementação. |
| [Cobra — releases](https://github.com/spf13/cobra/releases), [v1.10.2 `go.mod`](https://raw.githubusercontent.com/spf13/cobra/v1.10.2/go.mod), [licença](https://raw.githubusercontent.com/spf13/cobra/v1.10.2/LICENSE.txt), [guia de comandos](https://cobra.dev/docs/how-to-guides/working-with-commands/) e [guia v1.10.2](https://github.com/spf13/cobra/blob/v1.10.2/site/content/user_guide.md) | v1.10.2 está marcada Latest na consulta, Apache-2.0 e `go 1.15` em seu módulo. A documentação demonstra `AddCommand`, `RunE`, argumentos/help; Cobra imprime erros/usage por padrão, e `SilenceErrors`/`SilenceUsage` permitem output controlado pela aplicação. | Fixar provisoriamente Cobra v1.10.2; usar árvore de subcomandos, help padrão, validação posicional e `RunE`. A CLI deve centralizar erro/código de saída sem duplicar texto do framework e manter os nomes `new`, `inspect`, `validate`, `workflow run` e `workflow verify` conforme a tabela de convenções. | 2026-09-20 | Piso Go da dependência não define a compatibilidade mínima do produto; conferir grafo final de dependências. Cobra não decide os nomes de produto e não é comparação de latência/manutenção frente a `flag` ou outros parsers. |
| [GitHub CLI — `gh workflow run`](https://cli.github.com/manual/gh_workflow_run) e [`gh repo`](https://cli.github.com/manual/gh_repo) | Manual usa subcomandos agrupados por recurso e o padrão `gh workflow run <id>`; `gh repo create` também agrupa ação por recurso. O primeiro aciona GitHub Actions remotamente. | Adotar `station workflow run <id>` e a hierarquia recurso→ação; usar somente a forma de navegação, não o comportamento externo de GitHub CLI. | 2026-09-20 | É exemplo de convenção, não norma universal; workflow do GitHub é uma operação remota mutável e não é analogia para permissões de Station. |
| [GitHub CLI — `gh attestation verify`](https://cli.github.com/manual/gh_attestation_verify) e [`gh release verify`](https://cli.github.com/manual/gh_release_verify) | CLI oficial usa `verify` como verbo filho após o recurso (`attestation verify`, `release verify`); `gh release verify` também oferece `--format {json}`. Algumas variantes buscam dados externos por padrão. | Substancia a forma `station workflow verify` e uma alternativa ao formato JSON escolhido; apenas a nomenclatura e precedência de flag são aproveitadas, Station verifica arquivos locais e não faz fetch externo. | 2026-09-20 | Precedente de forma, não de semântica/permissões; comportamento remoto de gh não é adotado. |
| [Docker CLI](https://docs.docker.com/reference/cli/docker/), [`docker inspect --format`](https://docs.docker.com/reference/cli/docker/inspect/), [GitHub CLI — formatação](https://cli.github.com/manual/gh_help_formatting), [`gh release verify`](https://cli.github.com/manual/gh_release_verify), [Kubernetes — kubectl output](https://kubernetes.io/docs/reference/kubectl/) e [Cobra — convenções](https://cobra.dev/docs/explanations/philosophy/) | Docker `inspect` lista `json` como formato válido de `--format`; `gh release verify` documenta `--format {json}`; kubectl usa `-o`/`--output json`; Cobra documenta `--help` e `--version` como flags padrão. | Manter a flag longa `--format json` da Station, sem alias curto, junto de `--help`/`--version` convencionais. É consistência deliberada, não afirmação de padrão universal. | 2026-09-20 | Ferramentas diferem: kubectl prefere `--output`, e `gh --json` seleciona campos em alguns comandos. As fontes não determinam usabilidade da Station; confirmar com testes de help, argumentos e JSON determinístico. |
| [Codex — instruções AGENTS.md](https://developers.openai.com/pt-BR/docs/agent-configuration/agents-md) | Codex combina instruções por diretório, com o arquivo mais próximo vindo depois; se não encontrar uma raiz de projeto, verifica apenas o diretório atual. Limite combinado documentado: 32 KiB por padrão. | Gerar AGENTS.md raiz conciso e instruções menores nas pastas mobile/web/server; como a Station não inicializa Git, orientar início no root e leitura explícita do AGENTS da área antes do trabalho. | 2026-09-20 | Comportamento do Codex; nenhum comando com modelo foi executado para provar o carregamento na sessão local. Outros hosts podem diferir. |
| [Codex — criação e descoberta de skills](https://developers.openai.com/pt-BR/docs/build-skills) | Skill é diretório com `SKILL.md` obrigatório contendo `name`/`description`; descrições direcionam acionamento implícito e precisam ser concisas/específicas. Codex procura `.agents/skills` localmente no repositório, do diretório atual em direção à raiz. | Gerar `.agents/skills/station-mobile`, `station-web`, `station-server` como instruções sob demanda de stack; testar cenários com/sem skill e iniciar Codex no root do workspace. | 2026-09-20 | Documentação oficial Codex; não estabelece que outro agente reconheça a estrutura nem que arquivo local significa carregamento ou invocação. |
| [Go `path/filepath`](https://pkg.go.dev/path/filepath), [`os.Root`](https://pkg.go.dev/os#Root) e [GO-2026-4970](https://pkg.go.dev/vuln/GO-2026-4970) | `filepath.IsLocal` é apenas léxico e não considera symlinks. `os.Root` (Go 1.24+) ancora operações em uma árvore e impede links simbólicos que saiam dela; ainda permite cruzar filesystem/bind mounts e device files. GO-2026-4970 descreve um escape corrigido em Go 1.26.5 e 1.27.0-rc.2. | Validar path lexicalmente e fazer operações de workspace por handle `os.Root`, sem resolver um path e reabri-lo fora do handle; usar raiz externa separada exclusivamente para leitura. Rejeitar escritas na raiz externa. | 2026-09-20 | `os.Root` não é sandbox contra processos, mounts ou device files e tem diferenças/plataformas com garantias menores; exigir toolchain de manutenção corrigida e cobrir symlink/traversal em testes. Não tratar `EvalSymlinks` seguido de open como proteção contra TOCTOU. |
| [Go `embed`](https://pkg.go.dev/embed) | `//go:embed` incorpora arquivos do diretório do pacote ou subdiretórios ao programa no build; a documentação registra restrições de path. | Embutir schema e templates necessários para a CLI funcionar fora do repositório/cwd; `station new` materializa cópias sob a raiz do novo workspace. | 2026-09-20 | Conteúdo embutido só muda com novo build; testar consistência entre asset embutido e cópia gerada, e não embutir segredos/dados transitórios. |
| [Expo — Create a project](https://docs.expo.dev/get-started/create-a-project/) e [create-expo-app](https://docs.expo.dev/more/create-expo/) | Expo recomenda create-expo-app, requer Node LTS e documenta `--no-install`; o scaffold pode criar arquivos/configuração. | Manter scaffold externo explícito e separado da criação contratual; `station new` não executa gerador nem instala dependências implicitamente. | 2026-09-19 | Comando/templates são mutáveis e Expo não foi verificado no PATH; nenhuma integração ou conta externa é presumida. |
| [React — Installation](https://react.dev/learn/installation) | A documentação consultada informa que Create React App está deprecated e não impõe nesta página um único bundler. | Não recomendar Create React App; Vite permanece sugestão pragmática, não requisito aprovado pelo produto. | 2026-09-19 | A fonte não compara bundlers nem escolhe Vite. |
| [Vite — Getting Started](https://vite.dev/guide/) | Documenta template `react-ts`, uso em monorepo e comandos dev/build, com requisitos de Node. | Vite é uma opção de scaffold web React/TypeScript a confirmar no design do projeto dogfooding e a executar somente por comando declarado. | 2026-09-19 | Documentação mutável; Vite não foi encontrado localmente e não foi instalado. A pesquisa valida compatibilidade documentada, não superioridade sobre outras ferramentas. |
| [Go — Create a module](https://go.dev/doc/tutorial/create-module) e [Go Modules Reference](https://go.dev/ref/mod) | `go mod init` cria módulo com module path; a identidade final depende do projeto. | Não inventar module path para um scaffold de servidor; exigir valor explícito antes de gerar `go.mod`. | 2026-09-19 | O path depende do repositório/projeto que ainda não existe; a documentação não define o valor deste workspace. |

## 17. Revisão própria do plano

Revisão concluída após escrever o documento:

- Cobertura: estado atual, CLI, workspace.json, comandos, cinco workflows, skills, comandos, paths, outputs, governança, monorepo, externos read-only, contexto, testes, dogfooding, fases, riscos e referências estão mapeados.
- Placeholders: não há TBD/TODO ou decisão disfarçada de valor final; `<workflow-id>`, `<command-id>` e similares são metavariáveis intencionais nos exemplos. Decisões dependentes de contexto estão explicitadas na lista de abertas com tratamento operacional.
- Consistência: nomes brainstorm, grill-me, design, build, review, paths docs/station/*, mobile/web/server e exit codes são usados de forma consistente; `shared` está marcado opcional. A arquitetura limita Cobra à CLI e evita outros frameworks; workflows, commands, `--format` e `workflow verify` usam os nomes pesquisados.
- Escopo: não há implementação de código, app de tarefas, chamada a modelo, instalação de dependência ou alteração externa nesta etapa. O esqueleto de workspace.json está marcado como ilustrativo, não como fixture válido.
- Dependências: a ausência de Git, TypeScript, lint, formatador, Expo e Vite foi registrada; Go 1.27.1 e Cobra v1.10.2 foram localmente verificados, enquanto jsonschema v6.0.3 não está no cache local consultado. A disponibilidade de rede para obter a dependência será confirmada separadamente durante implementação.
- Proveniência: requisitos do design, instruções dos prompts, observações locais, pesquisa externa e propostas de implementação estão distinguidos. A comparação CLI inclui Cobra, GitHub CLI, Docker e kubectl e registra a variação `--format`/`--output`; não há alegação de benchmark comparativo Go/Node.
- Segurança: segredo, traversal, shell, auditoria, provider e write externo têm testes e critérios observáveis; o uso de `os.Root` exige toolchain com patch corrigido e continua explicitamente sem alegação de sandbox.

O plano foi revisado e atualizado conforme as decisões mais recentes do usuário; a própria mensagem que pediu essas alterações também autorizou prosseguir autonomamente com a implementação depois de finalizá-las. Nenhuma decisão de produto aprovada foi reaberta. O bootstrap read-only do modo existing está resolvido na seção 4.
