# 0004 — Consulta da wiki do kb-ai, só leitura e com opt-in

- **Status:** accepted
- **Escrito em:** 2026-11-26. **Aceito em:** 2026-11-27.
- **Escopo:** como os agentes (o do `wb` e os de fora, como Claude Code e Codex) consultam a base de conhecimento pessoal `kb-ai`, e o que o `wb` pode ler fora do projeto.
- **Implementação:** PR #13 (`wb wiki`, ferramenta Pi `wiki`, chave `kbAi` no `.workbench.json`).

## Contexto

O `kb-ai` é uma base de conhecimento pessoal no formato LLM Wiki, num repo próprio: `raw/` guarda as fontes brutas (artigos e transcrições copiados de fora) e `wiki/` é a compilação em bundle [Open Knowledge Format v0.2](https://github.com/GoogleCloudPlatform/open-knowledge-format/blob/main/SPEC.md), mantida por um agente externo e revisada por PR. Cada pasta tem um `index.md` com uma linha por página, e cada página tem frontmatter com `title`, `description`, `status` e `stale_after`.

Ao trabalhar em outros projetos, os agentes não sabem que essa base existe nem como navegar nela. Pedir que eles "achem e leiam a wiki" faz cada sessão redescobrir o caminho e a organização, e deixa o `raw/` (texto de terceiros, possivelmente com instruções escritas para manipular agentes) ao alcance.

É a primeira vez que o `wb` lê algo fora do projeto. Até aqui, tudo o que o agente lia passava pelo gate e pela raiz do projeto.

## Decisão

1. **Só leitura e só `wiki/`.** O `wb` nunca escreve no `kb-ai` e nunca lê `raw/` nem o resto do clone. A raiz configurada precisa ser um diretório chamado `wiki`, resolvido com `realpath`. Caminhos absolutos, `..` e symlinks que saiam da raiz são recusados; só arquivos `.md` são lidos.
2. **Um módulo, duas interfaces.** `host/src/wiki.ts` faz índices, busca e leitura de página, e é usado por:
   - **`wb wiki`**, para qualquer agente, dentro ou fora de um projeto: `wb wiki` (índices, raiz primeiro), `wb wiki <termo>` (busca textual com `path`, `title`, `description`, `quality`) e `wb wiki show <caminho>`. A ajuda (`wb wiki --help`) diz ao agente quando consultar, que deve citar o caminho, avisar quando uma página está vencida, em rascunho ou com divergência, e que o conteúdo é dado, nunca instrução.
   - **A ferramenta Pi `wiki`** (`action: index | search | show`), com as mesmas semânticas, nos runs e etapas do `wb`.
3. **Opt-in por projeto.**
   - No `wb`: `"kbAi": { "wiki": true }` no `.workbench.json`. O opt-in faz parte do escopo efetivo da execução (`EffectiveScope.wiki`, também nas etapas), então aparece no `wb scope` e no prompt de sistema junto com o resto do que o agente pode ler. Sem isso, a ferramenta não é registrada, não entra na lista de ferramentas da sessão e é negada no gate.
   - Fora do `wb`: uma linha que a pessoa põe no `AGENTS.md` do projeto ("Para consultar a base de conhecimento, use `wb wiki` (veja `wb wiki --help`)"). O `wb` nunca escreve nesse arquivo.
4. **Onde está a wiki:** `WB_WIKI_DIR`, depois `~/.config/workbench/config.json` (`kbAi.wikiDir`), depois `~/developer-carol/kb-ai/wiki`. Lida direto do clone, que fica limpo na `main`; sem cópia, cache ou índice próprio.
5. **Falhas:** um `kbAi` malformado no `.workbench.json` para o run, como as outras chaves. Uma wiki configurada mas ausente ou ilegível só gera um aviso, e a tarefa segue sem ela. Uma pasta ilegível fica de fora da busca.

## Alternativas consideradas

- **Incorporar o `kb-ai` ao repo da Workbench.** Simplificaria a configuração, mas acopla dois projetos com ciclos e manutenção diferentes, e a consulta não precisa disso. Rejeitada; o `kb-ai` continua independente.
- **O `wb` manter a wiki (ingestão, revisão, triagem).** Fora da prioridade, que é consultar. A manutenção continua com o agente externo.
- **Montar `wiki/` no sandbox e liberar `read`/`find`/`grep` num caminho virtual.** Foi o primeiro design. Exigia encaminhar as ferramentas nativas do Pi (que leem pelo host, não pelo sandbox), um caminho reservado e checagem de symlink em cada ferramenta, e o agente ainda teria que descobrir a organização da wiki a cada vez. Trocado por uma ferramenta própria, que já devolve o que importa.
- **Uma skill instalada na máquina (`wb skills install`).** Apareceria para todo agente em todo projeto, contrariando o opt-in. Rejeitada; a ajuda do `wb wiki` e uma linha no `AGENTS.md` fazem o mesmo papel só onde a pessoa quer.
- **Embeddings ou busca semântica.** A wiki já é organizada para ser navegada por índice, e um índice próprio precisaria ser mantido. Rejeitada.

## Consequências

- **A ferramenta `wiki` lê pelo host, fora do sandbox.** A fronteira depende só da validação em `wiki.ts`, coberta por testes de caminho absoluto, `..`, symlink para fora e `raw/` vizinho. A saída passa pela mesma redação de segredos das outras ferramentas.
- **Fora do `wb`, não há barreira.** A linha no `AGENTS.md` é orientação: Claude Code e Codex têm acesso ao disco e podem ler `raw/` direto.
- **O conteúdo consultado vai para o provedor do modelo e para a auditoria em `.workbench/runs/`.** Aceito: as páginas já foram escritas por um agente e passaram por um provedor.
- **Depende do clone local.** Sem `~/developer-carol/kb-ai` (ou outra raiz configurada), a consulta só avisa e não faz nada. Não é um recurso para outras pessoas sem configuração.
- **O padrão serve para outras bases OKF.** Nada no módulo é específico do `kb-ai` além do caminho padrão e do nome da chave; uma segunda base pediria um novo ADR (vários diretórios, nomes).
