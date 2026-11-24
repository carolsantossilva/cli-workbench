# Workbench — Especificação da V1

- **Status:** V1 do host, conforme o [ADR 0001](docs/adr/0001-workbench-como-host-de-agente.md). Substitui a especificação Codex-first, que está no histórico do git.
- **Público principal:** desenvolvedor individual, numa máquina local.
- **Interface:** CLI (`wb`).
- **Implementação:** [`host/`](host/README.md), em TypeScript sobre o SDK do Pi.

## 1. Visão

A Workbench é **o agente**. Ela roda o loop do modelo, escolhe o contexto de cada etapa e aplica as regras antes de cada ação. Não é só uma orientadora de um agente externo. O raciocínio continua sendo do modelo; o que a Workbench controla é o que ele vê, o que pode fazer e o que fica registrado.

O objetivo de longo prazo é um orquestrador local e genérico. Cada tarefa vai para um agente ou para um workflow, com modelos remotos ou locais, e pode ser controlada de fora do terminal (ver §10).

O filesystem e o git do próprio projeto são a fonte de verdade. Não há banco de dados nem estado de projeto separado.

## 2. Objetivo da V1

Validar um fluxo semiautônomo e controlado para construir e evoluir software num projeto local:

```text
ideia → brainstorm → grill-me → design aprovado → build → review
```

E também tarefas avulsas (`wb run`) sob um sandbox.

Na V1 há dois tipos de autonomia:
- **Ações dentro do sandbox** acontecem sem pedir licença.
- **Ações fora dele** viram pergunta ao usuário (aprovação interativa) ou são negadas. Regras fixas, como arquivos sensíveis, `.git` e a configuração da Workbench, nunca são liberadas.

### Projeto de validação

O primeiro uso real é o dogfooding num app pessoal de tarefas e rotina. A validação mede a Workbench, não o app:
- os workflows produzem os artefatos certos;
- o sandbox segura o que deve segurar;
- a auditoria registra o que foi enviado.

## 3. Modos de uso

- **`wb run <tarefa>`:** uma tarefa avulsa sob um sandbox (`read-only`, `docs-only` ou `workspace-write`).
- **`wb step run <etapa>`:** uma etapa do workflow, com entradas, saída e escopo próprios.
- **`wb step approve <etapa>`, `wb status` e `wb scope`:** aprovação por hash, estado do workflow e escopo efetivo.

O mesmo fluxo vale para um projeto novo e para um repositório existente. Criar um projeto a partir de templates (`wb new`) fica para depois (§10).

## 4. Workflows

| Etapa | Objetivo | Lê | Escreve | Comandos | Pergunta ao usuário |
|---|---|---|---|---|---|
| `brainstorm` | transformar a ideia em entendimento compartilhado: propósito, usuários, resultados e limites | `idea.md` e o projeto | `brief.md` | — | sim |
| `grill-me` | desafiar o brief: ambiguidades, premissas, riscos e casos extremos | `brief.md` | `decisions.md` | — | sim |
| `design` | especificar o próximo incremento o bastante para construir e verificar | `brief.md`, `decisions.md` e o código | `technical-design.md` | — | não |
| `build` | implementar o design aprovado em lotes pequenos, com testes | `technical-design.md` aprovado | `plan.md` e o projeto | sandbox | não |
| `review` | avaliar a mudança contra o design, o plano e os testes | `technical-design.md`, `plan.md` e o diff | `review.md` | sandbox só leitura | não |

- O `build` só roda com o `technical-design.md` aprovado (`wb step approve design`, por sha256). Se o design mudar depois, a aprovação deixa de valer.
- Pesquisa arquitetural ampla fica fora da V1. O design resolve só o necessário para o incremento atual.

## 5. Artefatos

Tudo fica em arquivos legíveis no próprio projeto:

```text
docs/workbench/idea.md, brief.md, decisions.md, technical-design.md, plan.md, review.md
.workbench.json          exceções opcionais: sandbox padrão, globs negados, escrita extra
.workbench/state.json    aprovações (sha256) e últimas execuções
```

Fora do projeto, em `~/.local/state/workbench/runs/<run>.jsonl`, fica a auditoria de cada execução.

Não existe manifesto escrito à mão. O escopo vem de três fontes:
- os sandboxes prontos;
- a detecção automática dos comandos do projeto (`package.json`, `go.mod`, `Cargo.toml`);
- as exceções pequenas do `.workbench.json`.

## 6. Protocolo de contexto

- Cada etapa roda numa sessão nova do modelo, e o contexto passa de uma etapa para a outra só pelos arquivos declarados.
- O agente não relê o repositório inteiro em cada etapa: começa pelos artefatos e vai aos arquivos relevantes, aos diffs e aos resultados de comandos.
- O system prompt de cada execução descreve o escopo efetivo: o que pode ler, escrever e rodar, e o que é sempre negado.

## 7. Governança de dados

O envio de dados ao provedor do modelo é permitido, com controle. Implementado na V1:
- escopo de leitura e escrita por sandbox, aplicado antes de cada ação;
- arquivos sensíveis sempre negados, também mascarados no sandbox do bash;
- segredos redigidos antes do envio;
- auditoria local de cada decisão e de cada envio (modelo, bytes e sha256, nunca o conteúdo);
- três resultados para cada ação: permitir, perguntar ou bloquear.

Previsto e ainda não implementado:
- lista de provedores permitidos por projeto;
- aprovação antes de enviar uma categoria nova de dados.

Mudanças externas não fazem parte da V1: o sandbox não tem rede e a Workbench não faz push, PR nem deploy.

## 8. Princípios de execução

- A Workbench é o agente, sobre o SDK do Pi.
- Modelo configurável (`--model`). O padrão é o login do ChatGPT (`openai-codex/gpt-6-luna`, raciocínio `xhigh`), com o risco de termos de uso registrado no ADR 0001.
- Sem banco de dados próprio. Filesystem e git como fonte de verdade.
- Autonomia delimitada pelo sandbox: o agente faz tudo o que está permitido e só para em exceções, bloqueios ou ações que pedem autorização.
- Construção incremental, com diffs e testes verificáveis.
- Contexto mínimo suficiente para cada etapa.

## 9. Fora do escopo da V1

- macOS e Windows: o sandbox usa `bubblewrap`, que é só do Linux;
- Codex e Claude Code como agentes orientados;
- canal remoto (WhatsApp ou outro);
- interface web ou desktop;
- execução em nuvem;
- vários agentes em paralelo;
- deploy, operação e mudanças em sistemas externos;
- colaboração multiusuário.

## 10. Evolução posterior

1. `wb new` a partir de [`templates/`](templates/), adaptados ao host.
2. macOS, com o sandbox nativo (Seatbelt via `sandbox-exec`), como o Codex faz.
3. Canal remoto, com ADR próprio. Nenhum fluxo pode juntar dados privados, entrada não confiável e saída para fora.
4. Modelos locais.
5. Lista de provedores permitidos e aprovação por categoria de dados (§7).
6. Saída estruturada (`--json`), `--dry-run` e `--quiet` na CLI.
7. Vários agentes ou workflows em paralelo.
