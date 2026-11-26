# Brief — Workbench como host de agente baseado no Pi

Status: exploração (brainstorm). Não é design aprovado nem autorização para implementação.
Origem: conversa de 2026-10-17/18 sobre o [Pi](https://pi.dev/) e o modelo do OpenClaw.

## Ideia

Hoje a Workbench é uma CLI que prepara e verifica workflows, mas quem executa é um agente externo (Codex-first). A ideia é a Workbench também *ser* o agente: um host próprio, construído sobre o harness do Pi (MIT, TypeScript, SDK/RPC), em que o `workspace.json` define quais ferramentas, caminhos, comandos e provedores existem em cada etapa. É o que o OpenClaw fez para chat, aplicado ao fluxo de projeto `brainstorm → grill-me → design → build → review`.

## Propósito

Transformar as políticas da Workbench de orientação + verificação posterior em garantias aplicadas no momento da ação. Hoje o README declara que o host não é interceptado e que o postflight é best-effort. Vários requisitos da seção 7 do `design.md` só são cumpríveis por quem controla o loop do agente:

- registro local do que foi preparado ou **enviado** a um provedor;
- lista de provedores permitidos;
- detecção e mascaramento de segredos **antes** do envio;
- `readPaths`/`writePaths`/`allowedCommands` aplicados antes da execução, não só reportados depois;
- contexto mínimo por etapa montado a partir dos `inputs` declarados.

## Usuários

- Principal: o desenvolvedor individual que já é o público da V1.
- Secundário: quem prefere continuar no Codex/Claude Code. Para essa pessoa, a Workbench continua funcionando como hoje, com garantias mais fracas.

## Resultados desejados

1. Uma ação fora da política do workflow ativo não executa: é bloqueada ou exige confirmação, conforme `ask`/`deny`.
2. Cada etapa começa numa sessão nova contendo só a instrução, os inputs permitidos e o handoff anterior.
3. O audit em `.workbench/` registra o que foi enviado a qual provedor/modelo.
4. Os checkpoints de aprovação (design aprovado, plano aprovado) são interações explícitas do host.
5. O núcleo Go atual (`validate`, `workflow run`, `workflow verify`, `--format json`) continua sendo a fonte de verdade das regras e continua utilizável por outros hosts.

## Hipótese de arquitetura (a validar)

```text
workbench (Go)              núcleo agnóstico: contrato, validação, run/verify, audit
   ▲ JSON via subprocess
workbench-agent (TS + Pi)   host oficial: ferramentas restritas, sessão por etapa, TUI
Codex / Claude Code         modo orientado, como hoje
```

## Fora de escopo desta exploração

- Reescrever o núcleo Go em TypeScript.
- A Workbench controlar o Codex ou o Claude Code via RPC.
- Multi-agente, cloud, interface gráfica própria.
- Decidir agora a troca de "Codex-first". Isso exige um ADR depois do grill-me.

## Restrições conhecidas

- O `design.md` diz "Codex-first" e "sem chamadas próprias a modelos ou APIs de IA". O host proposto contraria os dois e precisa de decisão explícita.
- A CLI hoje não depende de Node. O host seria um pacote separado e opcional.
- **Custo e acesso a modelos:** a Anthropic não permite usar créditos do Claude Max em harnesses de terceiros. O uso de Claude no Pi é cobrado pela API ([fonte](https://yage.ai/share/pi-coding-agent-locked-out-en-20260518.html)). Não foi verificado se o login do ChatGPT/Codex funciona no Pi.
- Dependência do roadmap da Earendil, mitigada pela licença MIT (fork possível).
- O Pi não tem permissões nativas e executa bash diretamente. Sem a camada de política, ele é *menos* seguro que o Codex.

## Perguntas em aberto (para o grill-me)

- Qual provedor/plano será usado na prática, e quanto custa por workflow?
- O host substitui o Codex como caminho principal ou é um segundo caminho?
- Como bloquear `bash` arbitrário de forma confiável? Allowlist de comandos declarados apenas, ou bash livre dentro de um sandbox?
- Quem é a fonte de verdade da política em runtime: o host reimplementa a checagem de paths em TS, ou consulta o núcleo Go a cada ação?
- O que é registrado no audit de envio (conteúdo completo, hash, só metadados)?
- O spike confirma que o SDK do Pi permite restringir/substituir as ferramentas embutidas?
