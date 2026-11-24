# 0003 — Aprovação do design por PR, com o wb atuando como bot

- **Status:** accepted
- **Escrito em:** 2026-11-24.
- **Aceito em:** 2026-11-24, por carolsantossilva.
- **Escopo:** como o design é aprovado antes do `build`, e com que identidade o `wb` age no GitHub.
- **Implementação:** parcial. A trava de transição (item 5) está na PR #5. O GitHub App e a checagem da PR não foram implementados.

## Contexto

O `build` só roda com o design aprovado. Hoje a aprovação é o `wb step approve`, que grava o sha256 do arquivo em `.workbench/state.json`.

Isso fazia sentido quando só a pessoa rodava o `wb`. Mas o `wb` vai ser rodado por agentes: numa sessão direta (Claude Code, Codex) e no canal remoto (WhatsApp, ADR 0002). Um agente que roda `wb step run` também consegue rodar `wb step approve`, e aí a trava deixa de ser da pessoa.

A revisão de PR do GitHub é o lugar natural para essa aprovação, mas tem duas pegadinhas:
- **O GitHub não deixa ninguém aprovar a própria PR.** Se o agente abre a PR com o login da pessoa no `gh`, a autora é ela, e ela não consegue aprovar.
- **Qualquer sinal feito com o token da pessoa o agente também consegue fazer.** Um merge, um comentário "/aprovar" ou uma label feitos com o mesmo token não provam que foi a pessoa.

## Decisão

1. **O `wb` age no GitHub como um GitHub App próprio**, instalado só nos repositórios do usuário.
   - Ele usa tokens de instalação, que expiram em uma hora. As permissões são só as necessárias: escrever branches, abrir PRs e comentar em issues (ADR 0002). Ele nunca é admin.
   - PRs, issues e comentários feitos pelo `wb` aparecem como o App (`<nome>[bot]`), não como a pessoa.
   - A chave privada do App fica fora dos projetos, num lugar que só o host lê. O sandbox não a enxerga, porque a home fica escondida.

2. **A aprovação do design é um review `APPROVED` na PR do design, feito por um admin do repo.**
   - O `design` termina abrindo (ou atualizando) uma PR com o `technical-design.md`, aberta pelo App.
   - A pessoa revisa e aprova pelo GitHub, com a própria conta.

3. **Antes do `build`, o host confere a aprovação pela API, falhando fechado.**
   - Vale o último review de cada admin. Precisa haver pelo menos um `APPROVED`, e nenhum admin com `CHANGES_REQUESTED` depois dele.
   - O `technical-design.md` no commit aprovado (`commit_id` do review) tem que ter o mesmo sha256 do arquivo na worktree. Se o design mudou depois da aprovação, o `build` volta a ser barrado.
   - Admin é conferido como no ADR 0002: pela API de permissões, não pelo `author_association`.
   - Se algo não puder ser conferido (sem rede, sem permissão, PR não encontrada), o `build` não roda.

4. **Design numa issue (ADR 0002):** a aprovação é um comentário `/aprovar` de um admin, respondendo ao comentário do design. Valem as mesmas regras de autor e último editor do ADR 0002. O sha256 do texto do design é gravado no momento da aprovação; se o texto mudar, a aprovação deixa de valer.

5. **Transição: até o App existir, o `wb step approve` continua, mas só num terminal interativo**, com a palavra "aprovar" digitada.
   - Um agente que roda o `wb` não tem terminal e não aprova por engano.
   - Ela **não** impede um agente decidido, que pode simular um terminal (ex.: com `script`).
   - Depois do App, o `wb step approve` continua só para projetos sem remoto no GitHub.

## Alternativas consideradas

- **`wb step approve` local, sem mais nada (como era).** Qualquer agente que rode o `wb` aprova. Rejeitada.
- **Aprovação pela PR, mas com o token da pessoa.** O GitHub barra a autora de aprovar a própria PR, e qualquer outro sinal (merge, label, comentário) pode ser feito pelo agente com o mesmo token. Rejeitada.
- **Merge da PR como aprovação.** Mesmo problema de token. Além disso, o design teria que entrar na branch base antes do código. Rejeitada.
- **Conta de máquina (um segundo usuário do GitHub).** Funciona, mas ocupa uma conta e usa token de longa duração e 2FA próprio. Um GitHub App tem permissões por repositório e tokens que expiram. Rejeitada em favor do App.

## Consequências

- **Onde a garantia vale:** no sandbox do `wb` (sem rede e sem a home, logo sem o token da pessoa) e no canal remoto, que só tem o token do App. Nesses dois lugares, o agente não consegue aprovar no lugar da pessoa.
- **Onde ela não vale:** numa sessão direta (Claude Code, Codex rodando com acesso à máquina), o agente tem acesso ao `gh` da pessoa e poderia aprovar como ela. Isso é aceito porque a pessoa está acompanhando a sessão. As instruções aos agentes dizem para nunca aprovar.
- **Branch protection passa a funcionar.** Como o autor das PRs é o App, uma regra "exige 1 aprovação" na branch base é satisfeita pela aprovação da pessoa. Antes, num repo de uma pessoa só, essa regra não tinha como ser cumprida.
- O `wb` passa a precisar de rede para o GitHub fora do sandbox: o host fala com a API, o agente continua sem rede.
- Criar e instalar o App é um passo manual de configuração, documentado no README do host quando for implementado.

## Em aberto

Nenhum.
