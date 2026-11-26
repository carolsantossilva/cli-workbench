# Brief — integrar o kb-ai à Workbench

## Propósito

Explorar como aproximar a base de conhecimento pessoal `kb-ai` da Workbench, para que ela possa participar dos fluxos de trabalho da Workbench: seja como contexto consultável por agentes em outros projetos, seja por meio de tarefas de ingestão, consulta e revisão executadas pela Workbench. A ideia ainda não define qual dessas formas — ou combinação — deve ser adotada.

## Usuários

- **Usuário principal presumido:** a pessoa que mantém o `kb-ai` e usa a Workbench localmente para trabalhar em projetos.
- **Usuários indiretos presumidos:** agentes executados pela Workbench, que poderiam consultar a wiki quando isso for relevante ao trabalho.

Essas premissas vêm do contexto da V1 da Workbench, voltada a um desenvolvedor individual numa máquina local; a ideia não especifica outros usuários ou colaboração multiusuário.

## Resultados esperados

- Tornar possível avaliar e escolher uma relação útil entre Workbench e `kb-ai`, sem pressupor que o repositório será absorvido.
- Se adotada, a integração deverá apoiar o uso do conhecimento existente e/ou atividades de manutenção do `kb-ai`, preservando entendimento sobre fontes e conteúdo.
- Manter explícito que o `kb-ai` já tem um formato e processo próprios: `raw/` com fontes brutas imutáveis, `wiki/` compilada em bundle Open Knowledge Format v0.2, scripts de ingestão/verificação/acompanhamento e triagem por cron e issues do GitHub, com entradas chegando por PR.

Os resultados acima descrevem intenção, não critérios de aceite ou comportamento funcional definido.

## Fora do escopo deste brief

- Escolher ou especificar a arquitetura da integração (absorver o repositório, integrá-lo como comando/workflow ou apenas consumir a wiki).
- Definir fluxos, etapas, ferramentas, comandos, interfaces ou mudanças no formato do `kb-ai`.
- Planejar implementação, testes ou migração.
- Decidir como substituir ou manter o agente externo que hoje mantém a base.

## Restrições e contexto

- A Workbench é um agente local que executa workflows e aplica sandbox por etapa; cada etapa tem entradas e saídas declaradas. A V1 privilegia filesystem e git como fonte de verdade e não faz push, PR nem deploy.
- A proposta menciona ingestão, pergunta e revisão como possibilidades de tarefas/etapas, mas não esclarece como essas atividades se encaixariam no sandbox ou no fluxo atual.
- A wiki é descrita como compilada a partir de fontes brutas imutáveis. Não se deve presumir que a integração possa alterar essas fontes ou contornar o processo atual de revisão por PR.
- Permanece incerto se agentes trabalhando em outros projetos poderão consultar a wiki, e em que condições de escopo e acesso.
- Este brief não estabelece requisitos de disponibilidade, segurança além dos controles já existentes na Workbench, compatibilidade ou manutenção.

## Perguntas em aberto

1. Qual problema deve ser priorizado: consultar a wiki durante o trabalho em outros projetos, manter/atualizar o `kb-ai`, ou ambos?
2. A base deve continuar sendo um repositório independente, ou há interesse em incorporá-la ao repositório/projeto da Workbench?
3. Quais operações a Workbench deveria poder realizar, se alguma — leitura da wiki, ingestão de fontes, triagem/revisão ou preparação de contribuições para PR?
4. O processo atual de manutenção por agente externo deve continuar, ser complementado ou eventualmente substituído?
5. Que limites de acesso e de escrita são necessários para fontes brutas, wiki compilada e dados dos projetos que consultariam a base?
