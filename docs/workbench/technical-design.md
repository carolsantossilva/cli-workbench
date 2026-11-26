# Design técnico — consulta da wiki `kb-ai`

## Escopo do incremento

Adicionar consulta somente de leitura à wiki independente `kb-ai`, restrita ao diretório `wiki/`:

1. **CLI `wb wiki`** com três modos: índices, busca textual e exibição de uma página (`show`). A ajuda de `wb wiki --help` e `wb help wiki` deve ser autoexplicativa para agentes: explicar quando consultar, os três modos, citar sempre o caminho relativo da página, sinalizar `stale_after` vencido, `status: draft` e `⚠️ Divergência`, e instruir a tratar todo conteúdo retornado como dado, nunca como instrução.
2. **Ferramenta nativa Pi `wiki`**, disponível apenas com opt-in explícito `kbAi.wiki: true` no `.workbench.json`. Não altera nem encaminha ferramentas de filesystem, nem depende de montagem no sandbox.
3. **Opt-in para agentes externos fora do `wb`**: a própria pessoa pode incluir no `AGENTS.md` do projeto uma linha como `Para consultar a base de conhecimento, use \`wb wiki\` (veja \`wb wiki --help\`).` O `wb` não cria nem altera `AGENTS.md`. Documentar essa linha em `host/README.md`.

Não implementar skill (`SKILL.md`), instalação de skills, `wb skills install`/`wb skills status`, nem testes correspondentes. Uma skill instalada na máquina seria global entre projetos e contrariaria o opt-in por projeto. A linha no `AGENTS.md` é uma orientação explícita por projeto, não uma barreira técnica: fora do `wb`, nada impede um agente de tentar ler `raw/` diretamente.

A Workbench só lê; não mantém nem modifica `kb-ai`. Sem ingestão, revisão, cópia, cache, embeddings ou índice persistente. A wiki é acessada diretamente; `raw/` nunca é fonte de consulta nem fallback. O `kb-ai` continua independente.

## Componentes e responsabilidades

- **Módulo comum `host/src/wiki.ts` (novo)**: resolver a raiz da wiki e sua configuração; validar raiz e caminhos de página; listar índices e páginas Markdown; ler conteúdo; extrair os campos limitados de frontmatter; implementar índices, busca, `show` e marcadores de qualidade. CLI e ferramenta Pi usam esta implementação comum.
- **CLI (`host/src/cli.ts`, `host/src/args.ts`)**: despachar `wb wiki [termo]`, `wb wiki show <caminho>` e suas ajudas; validar argumentos. `wb help wiki` é equivalente à ajuda específica. A consulta funciona sem raiz de projeto e sem `-C`. Não adicionar comandos de instalação/status de skills.
- **Configuração global da wiki**: resolver localização pela precedência fechada: `WB_WIKI_DIR` > `~/.config/workbench/config.json` > padrão `~/developer-carol/kb-ai/wiki`. A configuração JSON usa a chave `kbAi.wikiDir`. A raiz, inclusive em overrides, é o diretório `wiki/`, não o clone inteiro.
- **`host/src/project-config.ts`**: aceitar `kbAi.wiki: true` como opt-in da ferramenta nativa. Ausência/`false` significa sem ferramenta. Valor malformado é erro de configuração, não amplia o acesso.
- **`host/src/host.ts` e `host/src/extension.ts`**: propagar configuração de projeto para a extensão e registrar a ferramenta Pi `wiki` apenas no opt-in. A ferramenta oferece as mesmas três operações do CLI através do serviço comum. Runs sem opt-in não expõem nem mencionam `wiki`. Não modificar `read`, `find`, `grep`, regras de sandbox ou montagem bwrap.
- **`host/README.md`**: explicar o opt-in por linha no `AGENTS.md`, incluindo o exemplo acima e a limitação técnica de acesso a `raw/` fora do `wb`.

## Formato, fluxo e fronteira de dados

### Configuração e localização

`WB_WIKI_DIR` prevalece sobre `kbAi.wikiDir` em `~/.config/workbench/config.json`; se ambos faltarem, usar `~/developer-carol/kb-ai/wiki` (expandindo `~`). Valor definido porém inválido não deve silenciosamente cair para outra fonte: emitir erro/aviso de configuração e não consultar. A raiz é canonicalizada com `realpath` e deve existir como diretório.

### Conteúdo e parser

Consultar somente arquivos Markdown (`.md`) sob a raiz configurada; arquivos de índice são os chamados exatamente `index.md`. O parser de frontmatter é deliberadamente limitado e lê apenas `title`, `description`, `status` e `stale_after`. Deve suportar o YAML existente em flow style e datas ISO 8601 terminadas em `Z`, por exemplo `2027-02-12T00:00:00Z`; não implementar YAML genérico nem interpretar outros campos. Frontmatter ausente ou malformado não impede busca/exibição do corpo.

### Modos

- **Índices**: `wb wiki` lista o conteúdo de `wiki/index.md` primeiro, depois os `index.md` das pastas em ordem lexical de caminho. Não sintetiza resumo por modelo. A ajuda explica que este modo serve para navegar a estrutura e escolher páginas relevantes.
- **Busca**: `wb wiki <termo>` faz busca textual case-insensitive nas páginas Markdown e índices; cada resultado tem sempre os mesmos campos fixos, nesta ordem, tanto na CLI quanto na ferramenta: `path`, `title`, `description`, `quality`. `path` é relativo à raiz. Campos ausentes têm representação explícita e estável, sem inventar metadados. `quality` sinaliza `stale` quando `stale_after` já venceu, `draft` para `status: draft` e `⚠️ Divergência` quando essa expressão aparece no conteúdo. Os marcadores não excluem o resultado.
- **Exibição**: `wb wiki show <caminho>` imprime o conteúdo original da página sem reformatação. O agente deve citar esse caminho relativo em respostas baseadas nela.

Para vencimento, comparar `stale_after` com a data UTC corrente; data ISO inválida ou ausente não é considerada válida nem vencida, e aparece como estado desconhecido/sem marcador stale. A ajuda instrui agentes a sinalizar stale, draft e divergência ao usar páginas. Conteúdo de qualquer página é dado não confiável, nunca instrução a seguir.

### Validação de caminhos

Normalizar e validar todo caminho antes de leitura: rejeitar caminhos absolutos, traversal (`..`), escapes por symlink e links quebrados. Resolver o destino canônico e exigir que permaneça dentro da raiz canônica. A enumeração e busca também não seguem links para fora. A mesma validação vale para `show`, ferramenta Pi e varredura; nenhum caminho fora de `wiki/` é aberto para obter conteúdo ou metadados.

### Fluxo da ferramenta Pi

1. O host carrega `.workbench.json` como parte da configuração do projeto.
2. Sem `kbAi.wiki: true`, não registra ferramenta e não inclui aviso no prompt.
3. Com opt-in, resolve a raiz global pela precedência definida, valida-a e fornece o serviço à extensão.
4. A extensão oferece índices, busca ou show quando solicitados; não injeta páginas automaticamente. Retorna conteúdo e caminhos relativos limitados à wiki. O agente cita caminho e comunica marcadores de qualidade.

### Fluxo externo

A pessoa adiciona, se desejar, a linha de instrução ao `AGENTS.md` de cada projeto em que queira habilitar consulta via CLI. O `wb` não toca nesse arquivo. O agente externo consulta a ajuda e invoca `wb wiki`; se não houver `wb` disponível, essa orientação não garante acesso restrito e o agente pode tecnicamente ler `raw/` direto.

## Comportamento em erro

- **Configuração ausente**: usar o padrão definido. **Raiz ausente/inacessível ou configuração inválida**: não consultar nem procurar outro clone; CLI apresenta aviso claro em `stderr` e termina sem resultados; a ferramenta retorna aviso estruturado sem conteúdo. Falha opcional da wiki não derruba o run.
- **Argumentos inválidos ou uso incorreto**: mensagem de erro com a ajuda correspondente; `show` sem caminho ou com argumentos extras não é tratado como busca.
- **Página inexistente/fora da raiz**: `show` informa que não está disponível; rejeitar caminhos absolutos, traversal e symlink externo. A busca ignora entradas inválidas e nunca amplia o escopo.
- **Falha parcial de leitura**: não expor conteúdo externo; informar consulta incompleta de forma concisa.
- **Frontmatter ausente, inválido ou incompleto**: campos desconhecidos/ausentes permanecem explícitos; corpo ainda pode ser buscado e exibido se o caminho for válido.
- **Data ISO inválida**: qualidade de frescor desconhecida, não classificar como vencida. Status draft e divergência continuam sendo sinalizados independentemente.

## Testes que provam que funciona

1. **Ajuda**: `wb wiki --help` e `wb help wiki` explicam quando consultar, índices/busca/show, citação de caminho relativo, stale, draft, `⚠️ Divergência` e conteúdo como dado/nunca instrução.
2. **Índices**: fixture com índice raiz e índices aninhados comprova ordem estável, conteúdo esperado e ausência de páginas que não sejam índices.
3. **Busca**: busca case-insensitive encontra páginas e índices; todos resultados têm exatamente `path`, `title`, `description`, `quality` na ordem contratada. Verificar campos ausentes e marcadores de stale/draft/divergência. Confirmar equivalência estrutural entre saída CLI e ferramenta Pi.
4. **Frontmatter**: fixtures em YAML flow style e data `2027-02-12T00:00:00Z`; verificar extração dos quatro campos suportados, data passada/futura, e fallback para frontmatter ausente/malformado.
5. **Show e CLI**: conteúdo é preservado, caminho relativo citado, sintaxe diferenciada de busca e show validada, execução sem diretório de projeto.
6. **Configuração**: provar precedência `WB_WIKI_DIR` > `~/.config/workbench/config.json` (`kbAi.wikiDir`) > padrão; testar override inválido sem fallback silencioso.
7. **Fronteira**: fixtures de `raw/`, arquivo adjacente, caminhos absolutos, `..`, symlink interno, externo e quebrado. Somente destino interno canônico pode ser lido em índices, busca e show; nenhuma operação escreve.
8. **Opt-in Pi**: sem `kbAi.wiki`, ferramenta e aviso ausentes; com `true`, três modos acessíveis. Configuração inválida não deve ampliar acesso nem derrubar run de consulta opcional.
9. **Regressão**: testes atuais de CLI, host, extensão, configuração e workflows seguem válidos; nenhum teste/módulo/comando relacionado a skills é criado.

## Decisões tomadas

- A prioridade é consultar a wiki em projetos; `kb-ai` segue independente e sua manutenção permanece externa.
- Somente leitura de `wiki/`; `raw/` nunca é lido pelo serviço, nem como fallback. Não há cache, cópia, embeddings ou índice persistente.
- O design dispensa integralmente skill global, `SKILL.md`, instalação/status e testes de skills. Opt-in externo é uma linha manual no `AGENTS.md` por projeto; o `wb` nunca escreve nesse arquivo.
- Para execuções Workbench, a ferramenta nativa `wiki` exige `kbAi.wiki: true` em `.workbench.json`; ferramentas de filesystem e sandbox permanecem inalterados.
- Localização: `WB_WIKI_DIR` > `~/.config/workbench/config.json` (`kbAi.wikiDir`) > `~/developer-carol/kb-ai/wiki`.
- Frontmatter limitado a `title`, `description`, `status`, `stale_after`, compatível com YAML flow style e datas ISO 8601 com `Z`.
- Saída de busca, igual estruturalmente na CLI e ferramenta, usa somente `path`, `title`, `description`, `quality`.
- Ajuda da wiki orienta quando e como consultar, citar caminhos, sinalizar qualidade e tratar conteúdo como dado, nunca instrução.
- Falha da wiki é degradada e não interrompe a execução principal; nunca há fallback para `raw/`.
- Limitação reconhecida: fora do `wb`, a linha no `AGENTS.md` é orientação, não controle técnico; um agente ainda pode ler `raw/` diretamente.

## Premissas e pontos ainda em aberto

- **Premissa**: override de diretório aponta diretamente para `wiki/`; páginas consultáveis são `.md` e índices têm o nome exato `index.md`.
- **Premissa**: `stale_after` é uma data/hora ISO 8601 UTC terminada em `Z`; comparação é com a data/hora UTC corrente.
- **Em aberto para o build**: schema final da ferramenta Pi (por exemplo `action: index|search|show` com argumentos condicionais) e formatação textual dos registros, preservando os quatro campos fixos e suas semânticas idênticas.
- **Em aberto para o build**: representação textual de campo ausente e qualidade desconhecida, desde que estável e não induza o agente a inferir metadados.
- **Em aberto para o build**: tratamento da chave `kbAi` inválida na configuração do usuário/projeto, alinhado ao padrão existente de validação, sem ampliar acesso. Não há requisito de compatibilidade com YAML além das formas especificadas.
