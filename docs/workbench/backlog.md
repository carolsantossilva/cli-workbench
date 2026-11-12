# Backlog da Workbench

Lista local e temporária de melhorias candidatas para a Workbench, antes de serem transferidas manualmente para issues. A ordem indica prioridade de discussão, não autorização para implementação. Uma ideia escolhida ainda precisa passar pelo fluxo de design e aprovação da Workbench.

## Templates básicos executáveis para mobile, web e API/server

- **Estado:** ideia não refinada.
- **Intenção:** avaliar a evolução do scaffold de `workbench new` para incluir templates básicos executáveis para o app mobile React Native/Expo, a aplicação web React e a API/server em Go.
- **Contexto:** hoje o scaffold fornece a estrutura do monorepo, contratos e instruções para agentes, mas não gera código-fonte nem projetos executáveis.
- **Em aberto:** o que significa “básico executável” em cada área; frameworks e versões; comandos e verificações; como tratar instalação/download de dependências; module path Go; e se há necessidade de código compartilhado.
- **Próximo passo quando selecionada:** refinar a ideia nos workflows de brainstorm, grill-me e design antes de criar plano ou alterar o scaffold.

## Review em contexto limpo, com rubrica versionada

- **Estado:** ideia não refinada.
- **Intenção:** rodar o workflow `review` sem o histórico do `build`, recebendo só o diff, o `design.md`, os resultados de teste e uma rubrica gerada no workspace. A rubrica teria níveis de severidade (o que bloqueia e o que é só sugestão), um limite de sugestões menores, uma lista "sempre checar" e uma lista "não reportar", além de uma checagem final contra o objetivo do `brief.md`.
- **Contexto:** agentes avaliam com leniência o que eles mesmos produziram. Um avaliador separado, com critérios escritos e limiares, é mais fácil de calibrar do que um gerador autocrítico. Para isso não são necessários vários agentes, só um contexto que não seja reaproveitado.
- **Em aberto:** como o host garante a sessão limpa (nova sessão ou subagente); formato da rubrica no scaffold; se o `verify` deve checar que o review produziu um veredito para cada critério.
- **Referências:** [Harness design for long-running apps](https://www.anthropic.com/engineering/harness-design-long-running-apps); [Why Do Multi-Agent LLM Systems Fail?](https://arxiv.org/abs/2503.13657) (falhas de verificação e o ganho de uma checagem do objetivo de alto nível).
- **Próximo passo quando selecionada:** refinar a ideia nos workflows de brainstorm, grill-me e design.

## Contrato de lote e critérios de aceite verificáveis

- **Estado:** ideia não refinada.
- **Intenção:** antes de cada lote do `build`, registrar critérios de pronto testáveis e fazer o `review` aceitá-los antes da implementação. Os critérios do design ficariam num arquivo estruturado (por exemplo, `acceptance.json`) em que o `build` só pode mudar o status de cada item. O `workflow verify` checaria isso de forma determinística, e cada instrução de workflow ganharia uma condição de término explícita.
- **Contexto:** os modos de falha mais comuns em agentes longos são declarar o trabalho pronto cedo demais e não saber quando terminar. Arquivos estruturados com campos mutáveis restritos são alterados indevidamente com menos frequência do que markdown.
- **Em aberto:** relação com o `plan.md`; quem cria o arquivo (design ou build); como o `verify` compara versões sem depender de git.
- **Referências:** [Effective harnesses for long-running agents](https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents); [Harness design for long-running apps](https://www.anthropic.com/engineering/harness-design-long-running-apps).
- **Próximo passo quando selecionada:** refinar a ideia nos workflows de brainstorm, grill-me e design.

## Guardrails em camadas gerados pelo scaffold

- **Estado:** ideia não refinada.
- **Intenção:** fazer o `workbench new` gerar, a partir do `workspace.json`:
  - hooks do agente host que bloqueiam escrita fora de `writePaths` e a edição dos próprios arquivos de política;
  - um hook de git que repete as checagens de política;
  - um CI que repete as checagens;
  - uma lista de caminhos protegidos que só mudam com aprovação humana explícita.
- **Contexto:** hoje o `verify` é best-effort: ele detecta mudanças fora do escopo depois que acontecem, sem impedi-las. Instruções e skills orientam, mas só verificações determinísticas garantem. É o item "políticas e hooks de governança mais fortes" da evolução posterior do `design.md`, e combina com os templates executáveis acima.
- **Em aberto:** suporte a hooks em cada host (Codex, Claude Code); qual CI gerar; como exceções humanas ficam registradas.
- **Próximo passo quando selecionada:** refinar a ideia nos workflows de brainstorm, grill-me e design.

## Teste de ponta a ponta como usuário no review

- **Estado:** ideia não refinada.
- **Intenção:** declarar comandos de teste E2E (por exemplo Playwright para a web e um equivalente para o app Expo) em `allowedCommands` do `review`, para que a verificação use a aplicação como um usuário, e não só testes unitários.
- **Contexto:** agentes tendem a marcar funcionalidades como prontas sem testá-las de ponta a ponta; com automação de navegador o resultado melhora muito.
- **Em aberto:** depende dos templates executáveis; ferramenta E2E para mobile.
- **Referências:** [Effective harnesses for long-running agents](https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents).
- **Próximo passo quando selecionada:** refinar a ideia nos workflows de brainstorm, grill-me e design.

## Avaliação do próprio Workbench e revisão das etapas a cada modelo

- **Estado:** ideia não refinada.
- **Intenção:** manter um conjunto pequeno de ideias curtas (cerca de 20, além do planner de dogfooding), rodar o fluxo completo a cada mudança de workflow ou de modelo e avaliar os artefatos por rubrica. Remover uma etapa por vez para medir o efeito de cada uma (por exemplo, o `grill-me` ainda muda o resultado?).
- **Contexto:** cada etapa de um harness codifica uma suposição sobre o que o modelo não faz sozinho, e essas suposições vencem conforme os modelos melhoram. Avaliações pequenas já mostram efeitos grandes no início.
- **Em aberto:** onde guardar o conjunto e os resultados; quanto do processo automatizar sem a CLI chamar modelos.
- **Referências:** [How we built our multi-agent research system](https://www.anthropic.com/engineering/multi-agent-research-system); [Harness design for long-running apps](https://www.anthropic.com/engineering/harness-design-long-running-apps).
- **Próximo passo quando selecionada:** refinar a ideia nos workflows de brainstorm, grill-me e design.

## Base de conhecimento OKF como repositório externo

- **Estado:** ideia não refinada.
- **Intenção:** permitir que um workspace declare uma base de conhecimento no formato Open Knowledge Format em `externalRepositories` (somente leitura), com `readPaths` para os workflows de design e review. Os agentes poderiam filtrar conceitos por `type`, `sources`, `verified` e `stale_after`.
- **Contexto:** o OKF v0.2 padroniza proveniência, confiança e validade em frontmatter, o que dá um contrato estável para consumir conhecimento sem adaptador.
- **Em aberto:** se o `inspect` deve entender `index.md` de bundles OKF; como avisar quando o workflow lê conceitos vencidos ou não verificados.
- **Referências:** [Open Knowledge Format, SPEC v0.2](https://github.com/GoogleCloudPlatform/open-knowledge-format/blob/main/SPEC.md).
- **Próximo passo quando selecionada:** refinar a ideia nos workflows de brainstorm, grill-me e design.
