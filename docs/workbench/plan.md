# Plano de build — consulta da wiki `kb-ai`

## Premissas aprovadas para os pontos em aberto

- Ferramenta Pi única `wiki`, com `action: index | search | show`; `query` obrigatório só em `search`, `path` obrigatório só em `show`, e combinações inválidas rejeitadas. `decideToolCall` permitirá `wiki` somente quando o opt-in estiver ativo.
- Campos ausentes serão `(ausente)`. `quality` conterá apenas sinais encontrados (`stale (venceu AAAA-MM-DD)`, `draft`, `divergência`) ou `ok`. Um `stale_after` inválido será apresentado como `stale_after inválido`, sem inferir vencimento ou atualidade.
- Configuração `kbAi` malformada ou com chave desconhecida interrompe o carregamento do projeto. Wiki corretamente habilitada mas ausente, sem índice ou ilegível apenas avisa e não interrompe a execução.
- Mantém-se o escopo do design: somente leitura de `wiki/`, sem `raw/`, sem skills e sem mudanças em ferramentas de filesystem ou sandbox.

## Lotes (ordem de dependência)

1. **Serviço comum, configuração/localização e fronteira de leitura** — implementar `wiki.ts`: resolver diretório (env > config do usuário > padrão), canonicalizar, enumerar somente Markdown dentro da raiz, validar symlinks/caminhos e extrair frontmatter. Testes unitários com fixtures para precedência, caminhos internos/externos e campos.
   - Verificação: `cd host && npm test`; testes novos do serviço passam, com fixtures de `raw/`, symlink e caminhos malformados.
2. **Índices, busca, show e qualidade** — completar operações comuns e formatação determinística compartilhada, incluindo os critérios de qualidade aprovados; provar ordenação, formato fixo, ausência de metadados inventados, datas e conteúdo original.
   - Verificação: `cd host && npm test`; testes do serviço provam resultados e salvaguardas.
3. **CLI `wb wiki` e ajuda** — despachar index/search/show sem exigir projeto, validar argumentos e cobrir ajuda para `wb wiki --help` e `wb help wiki`.
   - Verificação: `cd host && npm test`; testes CLI cobrem três modos, erros de uso e texto da ajuda.
4. **Opt-in e validação de `.workbench.json`** — aceitar apenas `kbAi.wiki: true|false`, erro para tipo/chaves inválidas; propagar opt-in ao host sem afetar runs não habilitados.
   - Verificação: `cd host && npm test`; testes de configuração e regressão comprovam erro estrito e ausência de opt-in por padrão.
5. **Ferramenta Pi e integração** — registrar ferramenta `wiki` só com opt-in; validar parâmetros condicionais, executar as mesmas operações comuns, produzir aviso estruturado em falha opcional e permitir ferramenta em `decideToolCall` apenas após registro opt-in.
   - Verificação: `cd host && npm test`; testes provam bloqueio sem opt-in, acesso aos três modos com opt-in e equivalência do formato.
6. **Documentação e regressão final** — documentar linha manual de opt-in no `AGENTS.md`, sua limitação fora do `wb`, revisar integração e ausência de recursos de skills.
   - Verificação: `cd host && npm test` e `cd host && npm run typecheck`; registrar resultados efetivos abaixo.

## Progresso

1. **Concluído — serviço comum e fronteira.** Criado `host/src/wiki.ts`, com precedência de configuração, canonicalização, leitura limitada a Markdown dentro da raiz (não segue links para fora), validação de `show` e parser limitado. Teste antes da implementação: `cd host && npm test` falhou como esperado porque o módulo ainda não existia; também revelou uma falha preexistente do teste de hooks por gitfile inválido. Após a implementação, os testes de wiki passaram.
2. **Concluído — índices/busca/show e qualidade.** Operações compartilhadas retornam índices ordenados e registros fixos; qualidade diferencia stale, draft, divergência e metadados ausentes. Verificação: `cd host && npm test`; testes da wiki passaram (incluindo fronteira, ordem e formato), com a única falha global no teste de hooks descrita abaixo.
3. **Concluído — CLI e ajuda.** `wb wiki`, busca, `show`, `wb wiki --help` e `wb help wiki` implementados; show preserva conteúdo original e a wiki roda sem validar projeto. Teste `cd host && npm test`: teste CLI wiki passou; falha global única no teste de hooks.
4. **Concluído — configuração de projeto.** `kbAi.wiki` aceita booleano; objeto malformado, tipo errado e chave interna desconhecida são erro. Testes de configuração incluídos. Verificação `cd host && npm test`: teste de configuração passou; falha global única no teste de hooks.
5. **Concluído — ferramenta Pi.** Ferramenta única `wiki` registra só com opt-in; `decideToolCall` permite `wiki` apenas nesse caso; valida ação e combinação de parâmetros e retorna aviso sem conteúdo se a wiki estiver indisponível. Testes comprovam registro condicional e decisão. Verificação `cd host && npm test`: testes de extensão passaram; falha global única no teste de hooks.
6. **Concluído — documentação e regressão.** README documenta opt-in manual no `AGENTS.md` e sua limitação fora do `wb`. Verificações finais:
   - `cd host && npm run typecheck` — passou (`tsc --noEmit`, sem erros).
   - `cd host && npm test` — 78 testes, 77 passaram e 1 falhou. Todos os testes novos de wiki passaram. A falha é `host/test/hooks.test.ts:200` (`wb hooks pela CLI`): `gitfile does not point to a valid repository` para `.worktree/.../.git`, incompatibilidade/estado do checkout sem relação com as mudanças; demais testes passaram.

Arquivos de implementação/documentação alterados: `host/src/wiki.ts`, `host/src/cli.ts`, `host/src/project-config.ts`, `host/src/host.ts`, `host/src/extension.ts`, `host/README.md`; testes em `host/test/wiki.test.ts` e `host/test/cli.test.ts`. Pendente: corrigir/restaurar o gitfile/checkout para repetir a suíte integral sem a falha de infraestrutura. Não houve contradição com o design.
