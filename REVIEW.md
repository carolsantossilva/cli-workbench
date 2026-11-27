# Review

Regras para quem revisa mudanças neste repositório, pessoa ou agente (inclusive o review antes do push). Um achado que quebre uma delas é defeito, não sugestão.

## Escopo e ferramentas do host

- **Toda ferramenta da sessão passa pela tabela `TOOLS` em `host/src/tools.ts`.** É ela que decide se a ferramenta entra na sessão, como o gate decide cada chamada e qual linha aparece na descrição do escopo (system prompt e `wb scope`). Aponte qualquer ferramenta registrada, liberada no gate ou incluída na lista da sessão por outro caminho, como um parâmetro ou opção solta ao lado do escopo.
- **Todo acesso novo do agente faz parte do `EffectiveScope`.** Isso vale para leitura fora da raiz do projeto, rede, um novo tipo de escrita ou uma nova fonte de dados. O acesso precisa ser preenchido em `resolveScope` e em `stepScope`, e aparecer no `describeScope`. Se o `wb scope` não mostra um acesso, ele não está revisado.
- **O que o system prompt diz precisa bater com o que o gate faz.** Se a descrição afirma que algo nunca é liberado, nenhuma ferramenta pode liberar isso, nem como exceção implícita.

## ADRs

- Um ADR `accepted` não é editado. Uma mudança de decisão vira um novo ADR que referencia o anterior (veja `docs/adr/README.md`).
