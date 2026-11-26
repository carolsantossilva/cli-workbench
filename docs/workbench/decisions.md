# Decisões e questões em aberto — kb-ai / Workbench

As decisões D1–D5 abaixo registram as respostas fornecidas pela pessoa usuária para esta etapa; não são inferências a partir do brief. D2 permanece deliberadamente sem decisão.

## Decisões registradas

### D1 — Prioridade
- **Pergunta:** Qual problema deve ser priorizado: consultar a wiki durante o trabalho em outros projetos, manter/atualizar o `kb-ai`, ou ambos?
- **Resposta (da pessoa usuária):** Prioridade é consultar a wiki durante o trabalho em outros projetos.
- **Opções não escolhidas como prioridade:** manutenção/atualização do `kb-ai` pela Workbench; priorizar ambos igualmente.
- **Consequência:** orientar a exploração para consumo da wiki por workflows em outros projetos. Isso não define ainda como o agente encontra, consulta ou utiliza os documentos, nem impede avaliar outras possibilidades no futuro.

### D2 — Independência do repositório
- **Pergunta:** O `kb-ai` deve continuar independente ou ser incorporado ao Workbench?
- **Resposta (da pessoa usuária):** O `kb-ai` continua um repositório independente; a pessoa aceitaria incorporá-lo ao Workbench somente se houver bons argumentos. Não autorizou nem decidiu a incorporação.
- **Argumentos a favor de incorporar:** descoberta e configuração potencialmente mais simples; workflows e mudanças relacionadas poderiam ser versionados em conjunto; menos dependência de localizar/configurar um repositório externo.
- **Argumentos contra incorporar:** acoplamento entre projetos com ciclos, permissões e manutenção distintos; risco de alterar o processo próprio do `kb-ai`; maior exposição da base e possível complicação de atualizar/usar a wiki a partir de vários projetos; a prioridade atual pode ser atendida por consumo somente de leitura sem absorção.
- **Opções não escolhidas:** tratar a incorporação como decidida ou como requisito. Não há decisão entre manter a separação como princípio definitivo e reconsiderar após comparação concreta.
- **Consequência:** manter a independência como estado atual e não planejar migração sem apresentar argumentos e obter decisão explícita.

### D3 — Operações da Workbench
- **Pergunta:** Que operações a Workbench pode realizar sobre o `kb-ai`?
- **Resposta (da pessoa usuária):** Somente leitura.
- **Opções não escolhidas:** ingestão, edição, triagem, revisão, geração de contribuições ou qualquer outra escrita pelo Workbench.
- **Consequência:** qualquer integração a explorar deve ser não modificadora em relação ao `kb-ai`; o que conta como leitura (incluindo cache, indexação ou cópia local) ainda precisa de definição.

### D4 — Manutenção
- **Pergunta:** O processo atual de manutenção por agente externo deve continuar, ser complementado ou substituído?
- **Resposta (da pessoa usuária):** Continua como está, feita pelo agente externo; a Workbench não mantém a wiki.
- **Opções não escolhidas:** transferir ou complementar a manutenção com fluxos da Workbench.
- **Consequência:** não atribuir à Workbench responsabilidades de ingestão, revisão ou atualização da wiki. A manutenção e o processo existente do `kb-ai` permanecem fora da integração considerada aqui.

### D5 — Limite de conteúdo acessível
- **Pergunta:** Que partes do `kb-ai` podem ser acessadas?
- **Resposta (da pessoa usuária):** Acesso somente a `wiki/` (bundle OKF); nunca a `raw/`.
- **Opções não escolhidas:** consultar fontes brutas ou usar `raw/` como fallback.
- **Consequência:** limitar o acesso ao conteúdo compilado em `wiki/`; confirmar também que ferramentas auxiliares e mecanismos de busca não atravessam esse limite. A resposta não especifica se metadados ou arquivos fora desses diretórios são permitidos.

## Questões em aberto

1. **Forma de consulta:** Como o agente deve consultar a wiki durante o trabalho em outro projeto — busca por texto, navegação/leitura de arquivos, recuperação automática de contexto ou uma combinação? Qual é a interação mínima útil?
2. **Quando e em quais projetos:** A consulta deve estar disponível em todos os projetos e workflows ou ser opt-in? Quem decide quando a wiki é relevante, e como evitar injetar contexto irrelevante?
3. **Fronteira de acesso efetiva:** Como garantir tecnicamente que o acesso fique restrito a `wiki/`, sem leitura acidental de `raw/` por links simbólicos, caminhos alternativos, índices, scripts ou permissões herdadas? Há outros dados do `kb-ai` que também devem ser negados?
4. **Atualização e cópias:** A Workbench lê a wiki diretamente do repositório independente ou de uma cópia/snapshot? Como detectar atualizações, lidar com checkout ausente e definir o comportamento quando a cópia está desatualizada? Cache ou indexação local são aceitáveis dentro de “somente leitura”?
5. **Proveniência e confiança:** Que referências o agente deve apresentar para sustentar respostas (arquivo, seção, identificador OKF ou outro)? Como sinalizar conteúdo sem fonte clara, contraditório ou possivelmente obsoleto, sem recorrer a `raw/`?
6. **Falhas e compatibilidade:** O que fazer se `wiki/` estiver ausente, inválida ou em uma versão/formato OKF incompatível? A consulta deve falhar fechada, avisar e prosseguir sem a wiki, ou haver outro comportamento? Quais versões precisam ser suportadas?
7. **Limites de dados entre projetos:** Que conteúdo da wiki pode ser incluído no contexto enviado a agentes de outros projetos? Há restrições de confidencialidade, retenção em logs ou exposição a ferramentas/modelos que precisam ser definidas antes de habilitar a consulta?

## Limitação desta etapa

Não foi possível fazer perguntas interativas com `ask_user`: essa ferramenta não está disponível neste sandbox. As respostas fornecidas no pedido foram registradas; as questões acima ficam em aberto, sem presumir respostas.
