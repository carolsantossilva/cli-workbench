# Review — consulta da wiki `kb-ai`

## Defeitos verificados

- **Média — `host/src/wiki.ts` — ordem dos índices viola o design.** `wikiIndexes()` recebe a lista de `pageFiles()`, que ordena todos os caminhos com `localeCompare`; não prioriza `index.md` da raiz. Assim, um índice como `conceitos/index.md` pode preceder `index.md`, contrariando o requisito de raiz primeiro. Confirmado pelo fluxo `pageFiles(...).sort(...)` e pelo filtro posterior em `wikiIndexes()`. O teste `host/test/wiki.test.ts` cobre apenas `index.md` e `sub/index.md`, cenário que não revela o defeito. **Ação:** ordenar explicitamente o índice raiz antes dos índices aninhados e acrescentar fixture de pasta cujo nome venha antes de `index.md` na ordem lexical.

## Comportamentos examinados

- **`wb wiki show ../raw/x` terminar com código 1 é coerente com o design.** `validPage()` rejeita traversal e `showWikiPage()` propaga o erro; o `main().catch()` em `host/src/cli.ts` imprime a mensagem e define `process.exit(1)`. O design exige que página fora da raiz seja informada como indisponível/rejeitada, mas não exige sucesso nesse caso. O aviso com término sem resultados aplica-se à wiki ausente/inacessível, não a um caminho de página inválido. O teste unitário confirma a rejeição de `../outside.md`; o código da CLI estabelece o exit code 1.

## Limitações e premissas

- **Inspeção do diff/status indisponível:** `git status --short && git diff` falhou antes de produzir saída: `fatal: gitfile does not point to a valid repository`. O `.git` da worktree aponta para `/home/carogo/developer-carol/workbench/.git/worktrees/docs-kb-ai-no-wb`, que não está válido neste sandbox. Portanto, não foi possível confirmar o conjunto completo de arquivos modificados nem comparar o diff integral; a avaliação acima usa os arquivos disponíveis e os documentos de design/plano/decisões.
- **Testes:** `cd host && npm test` — 78 testes; 77 passaram e 1 falhou. Todos os testes de wiki passaram. A falha foi `host/test/hooks.test.ts:200`, `wb hooks pela CLI: install --local e uso errado (sem rede)`, com `gitfile does not point to a valid repository` no checkout. Isso coincide com a limitação do gitfile descrita no pedido, não demonstrando defeito da mudança. `cd host && npm run typecheck` — passou (`tsc --noEmit`, sem erros).
- O resultado 78/78 informado fora do sandbox não foi reproduzido aqui; no sandbox, o teste de hooks não consegue acessar o repositório git da worktree.

## Correção depois do review

- Ordem dos índices corrigida em `host/src/wiki.ts` (`wikiIndexes` põe `index.md` da raiz primeiro e mantém a ordem lexical do resto). O teste ganhou a pasta `alfa/`, que antes reproduzia o defeito. Fora do sandbox: `npm test` 78/78 + 20 testes Python, `npm run typecheck` sem erros.
