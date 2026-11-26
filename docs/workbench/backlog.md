# Backlog da Workbench

Lista local e temporária de melhorias candidatas para a Workbench, antes de serem transferidas manualmente para issues. A ordem indica prioridade de discussão, não autorização para implementação. Uma ideia escolhida ainda precisa passar pelo fluxo de design e aprovação da Workbench.

## Templates básicos executáveis para mobile, web e API/server

- **Estado:** ideia não refinada.
- **Intenção:** avaliar a evolução dos templates do antigo scaffold `workbench new` (hoje em `templates/`, para um futuro `wb new`) para incluir templates básicos executáveis para o app mobile React Native/Expo, a aplicação web React e a API/server em Go.
- **Contexto:** hoje o scaffold fornece a estrutura do monorepo, contratos e instruções para agentes, mas não gera código-fonte nem projetos executáveis.
- **Em aberto:** o que significa “básico executável” em cada área; frameworks e versões; comandos e verificações; como tratar instalação/download de dependências; module path Go; e se há necessidade de código compartilhado.
- **Próximo passo quando selecionada:** refinar a ideia nos workflows de brainstorm, grill-me e design antes de criar plano ou alterar o scaffold.

## Review com rubrica e checagem do objetivo

- **Estado:** ideia não refinada.
- **Intenção:** dar ao `review` uma rubrica versionada no projeto, com níveis de gravidade (o que bloqueia e o que é só sugestão), um limite de sugestões menores, uma lista "sempre checar" e uma lista "não reportar", e terminar com uma checagem contra o objetivo do `brief.md`. O `review.md` traria um veredito para cada critério da rubrica.
- **Contexto:** o contexto limpo já existe: cada etapa roda numa sessão nova, e o `review` só recebe o que está nos arquivos, com bash só leitura. Falta o critério escrito. Agentes avaliam com leniência o que eles mesmos produziram, e um avaliador com critérios e limiares é mais fácil de calibrar do que um gerador autocrítico.
- **Em aberto:** onde fica a rubrica (em `templates/`, no projeto ou nas duas); se o `review` passa a ler o `brief.md`; se o `wb` confere que o `review.md` tem um veredito por critério; qual modelo faz o review, que pode ser outro que o do `build` (candidato a ADR).
- **Referências:** [Harness design for long-running apps](https://www.anthropic.com/engineering/harness-design-long-running-apps); [Why Do Multi-Agent LLM Systems Fail?](https://arxiv.org/abs/2503.13657) (falhas de verificação e o ganho de uma checagem do objetivo de alto nível).
- **Próximo passo quando selecionada:** refinar a ideia nos workflows de brainstorm, grill-me e design.

## Critérios de aceite verificáveis

- **Estado:** ideia não refinada.
- **Intenção:** o `design` gravaria os critérios de pronto num arquivo estruturado (por exemplo, `docs/workbench/acceptance.json`), aprovado junto com o `technical-design.md`. Durante o `build`, o agente só pode mudar o status de cada critério, e o `review` confere cada um. Cada etapa ganharia uma condição de término explícita.
- **Contexto:** os modos de falha mais comuns em agentes longos são declarar o trabalho pronto cedo demais e não saber quando terminar. Arquivos estruturados com poucos campos mutáveis são alterados indevidamente com menos frequência do que markdown. Como o `wb` é o próprio agente, ele pode impedir a mudança antes de ela acontecer, do mesmo jeito que aplica o sandbox, em vez de só detectá-la depois.
- **Em aberto:** relação com o `plan.md` e os lotes do `build`; se a aprovação do design (hoje o sha256 do `technical-design.md`) passa a cobrir os critérios; como a ferramenta de escrita restringe a mudança ao campo de status.
- **Referências:** [Effective harnesses for long-running agents](https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents); [Harness design for long-running apps](https://www.anthropic.com/engineering/harness-design-long-running-apps).
- **Próximo passo quando selecionada:** refinar a ideia nos workflows de brainstorm, grill-me e design.

## Teste de ponta a ponta como usuário no review

- **Estado:** ideia não refinada.
- **Intenção:** fazer o `review` usar a aplicação como um usuário, e não só rodar testes unitários: Playwright para a web e um equivalente para o app Expo, detectados no projeto como os outros comandos.
- **Contexto:** agentes tendem a marcar funcionalidades como prontas sem testá-las de ponta a ponta, e com automação de navegador o resultado melhora muito. Hoje o `review` já roda os comandos detectados, mas num bash sem rede e com o projeto só leitura, o que um teste E2E costuma não aguentar (servidor local, navegadores instalados na home, arquivos de resultado).
- **Em aberto:** o que liberar no sandbox para isso (porta local, cache de navegadores, uma pasta gravável para resultados); ferramenta E2E para mobile; depende dos templates executáveis.
- **Referências:** [Effective harnesses for long-running agents](https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents).
- **Próximo passo quando selecionada:** refinar a ideia nos workflows de brainstorm, grill-me e design.

## Avaliação do próprio Workbench e revisão das etapas a cada modelo

- **Estado:** ideia não refinada.
- **Intenção:** manter um conjunto pequeno de ideias curtas (cerca de 20), rodar o fluxo completo de etapas a cada mudança de instrução ou de modelo e avaliar os artefatos por rubrica. Remover uma etapa por vez para medir o efeito de cada uma (por exemplo, o `grill-me` ainda muda o resultado?).
- **Contexto:** cada etapa de um harness codifica uma suposição sobre o que o modelo não faz sozinho, e essas suposições vencem conforme os modelos melhoram. Avaliações pequenas já mostram efeitos grandes no início. O host já tem o que isso precisa: roda as etapas com `--no-input`, troca de modelo com `--model` e registra tokens e custo de cada run na auditoria.
- **Em aberto:** onde guardar o conjunto e os resultados; quem avalia os artefatos (outro modelo, a rubrica do review ou uma pessoa); custo de cada rodada.
- **Referências:** [How we built our multi-agent research system](https://www.anthropic.com/engineering/multi-agent-research-system); [Harness design for long-running apps](https://www.anthropic.com/engineering/harness-design-long-running-apps).
- **Próximo passo quando selecionada:** refinar a ideia nos workflows de brainstorm, grill-me e design.

## Base de conhecimento OKF no projeto

- **Estado:** ideia não refinada. A consulta de uma base fora do projeto já existe ([ADR 0004](../adr/0004-consulta-da-wiki-kb-ai.md): `wb wiki` e a ferramenta `wiki`, só leitura e com opt-in); este item é o que falta além dela.
- **Intenção:** reconhecer uma base OKF que é um repo dentro do projeto ([ADR 0002](../adr/0002-projeto-acima-dos-repos.md)), sem configuração, e deixar os agentes filtrarem conceitos por `type`, `sources` e `verified`, além dos avisos de `stale_after` e `status` que já existem.
- **Em aberto:** como o `wb` reconhece o bundle (o `index.md` com `okf_version`); como convivem a base do projeto e a configurada em `kbAi`; como avisar quando uma etapa usa conceitos não verificados.
- **Referências:** [Open Knowledge Format, SPEC v0.2](https://github.com/GoogleCloudPlatform/open-knowledge-format/blob/main/SPEC.md).
- **Próximo passo quando selecionada:** refinar a ideia nos workflows de brainstorm, grill-me e design.
