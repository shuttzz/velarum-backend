# GDD — Velarum (nome de trabalho)

> **Game Design Document consolidado.** Documento vivo de referência reunindo todas as decisões tomadas até aqui. Última atualização: 2026-06-02.
>
> ⚠️ **"Velarum" é um NOME DE TRABALHO** — ver seção [16. Status de Propriedade Intelectual](#16-status-de-propriedade-intelectual-nomes). Verificar formalmente (INPI/USPTO/EUIPO) antes do lançamento comercial.

---

## Índice
1. Visão geral
2. Mundo e narrativa
3. Facções
4. Core loop e pilares
5. Recursos e economia
6. Cidade única e slots
7. Progressão de eras
8. Mapa do mundo e expansão PvE
9. Combate
10. Alianças e cooperação
11. PvP e stakes
12. Lifecycle, temporadas e endgame
13. Monetização
14. Arte e pipeline visual
15. Stack técnica e infraestrutura
16. Status de propriedade intelectual (nomes)
17. Escopo do MVP
18. Decisões em aberto

---

## 1. Visão geral

**Gênero:** Estratégia de navegador persistente, multiplayer, assíncrona (PBBG/MMORTS), server-authoritative. Inspiração de gênero: Forge of Empires, Travian, OGame — com identidade própria.

**Pitch:** O jogador lidera uma civilização que **evolui através de 7 eras, da Antiguidade ao espaço**, num mundo original e fictício deixado *inacabado* por entidades primordiais (os Tecelões do Vazio). A progressão pelas eras é o coração do jogo. O endgame é cooperativo: "completar o Fio" que leva ao espaço.

**Tom:** Épico e acessível, estilo *Civilization* — otimista, convidativo, público amplo, **sem grimdark e sem humor explícito**.

**Plataforma:** Navegador (web), com app via PWA/Capacitor possível depois.

**Modelo de mundo:** Temporadas de 90-120 dias. Cidade única inviolável (modelo Forge of Empires).

**Princípio comercial:** Free-to-play, **não pay-to-win duro** — avançar nunca depende de pagar; monetização tentadora mas justa.

---

## 2. Mundo e narrativa

**Velarum** é um mundo criado *incompleto* pelos **Tecelões do Vazio** (forças primordiais cegas, não deuses), que "costuravam" a realidade com fios de Luz Bruta e desapareceram antes de terminar. Deixaram:
- As **Lacunas** — zonas onde a física falha levemente (rios que sobem, vegetação geométrica, minerais que emitem som). Estética: "glitches poéticos", belos e estranhos, nunca aterrorizantes.
- O **Fio Solto** — uma costura dourada visível nas estrelas, que parece continuar além do espaço conhecido. Cada civilização lê o Fio à sua maneira; segui-lo é o motor da história.
- Os **Nós dos Tecelões** — ruínas que só "se abrem" a civilizações suficientemente desenvolvidas, revelando fragmentos do próximo passo. São o gancho que conecta cada era a algo maior.

**Geografia:** três continentes (Aurond, leste dourado; Breval, norte vulcânico; Sorenta, oeste tropical/marítimo) ao redor do Mar das Velas.

**As 7 eras** (espinha central):
1. **Primeiros Fogos** (pedra) — espanto, sobrevivência vira conquista.
2. **Metais e Marés** (bronze/ferro) — orgulho, nascimento das cidades-estado.
3. **Grandes Rotas** (clássica/exploração) — aventura, o mundo encolhe.
4. **Engrenagens** (industrial) — vertigem, vapor e ferro.
5. **Raio e Voz** (moderno) — conexão, eletricidade e comunicação.
6. **Ascensão** (espacial inicial) — maravilha, primeiros voos e a lua artificial Velos.
7. **Era do Fio** (interestelar) — épico, seguir o Fio às estrelas.

Cada era se desbloqueia ao abrir um Nó, que revela um fragmento da linguagem/tecnologia dos Tecelões e antecipa a próxima era.

**Tempero narrativo do combate:** as batalhas são determinísticas — vendido como "o resultado já está tecido; comandantes habilidosos leem o Fio".

---

## 3. Facções

Cinco facções jogáveis com **mecânicas distintas** (assimetria real, não "+X%"). Aliança com facções diversas é mais eficiente que monocultura — incentivo de design para cooperação.

| Facção | Identidade | Mecânicas-chave |
|--------|-----------|-----------------|
| **Aurenthos** | Arquitetos da Memória | Construções duráveis (resistem a saque); compartilhar pesquisa com aliados; visão da composição inimiga antes do combate; Relicário (mantém produção de edifícios antigos sem ocupar espaço). |
| **Brevali** | Forjadores | Cadeia de produção (Energia→Componentes→uso); equipamento militar persistente por unidade; exército menor mas de elite; única que sintetiza Fio Bruto sem Nó. |
| **Sorenthai** | Navegadores | Âncora móvel (reposiciona a cidade no mapa 1×/era); Postos de Observação (inteligência de mapa); rota comercial rápida com aliados; diplomacia acelerada. *(Revisado: não funda colônias — modelo de cidade única.)* |
| **Kethari** | Nômades Comerciantes | Cidade nômade (move 1×/ciclo, barato); mercado/inteligência econômica; **multiplicador por diversidade de facção na aliança** (forte só em grupo). |
| **Valdruun** | Intérpretes das Lacunas | Salto de pesquisa (1×/era ignora pré-requisito a custo triplo); tropas de elite únicas ("Fragmentos") limitadas; dobro de Fio Bruto nos Nós; vê Lacunas instáveis no mapa. |

---

## 4. Core loop e pilares

**Sessão curta (5-15 min, 2-4×/dia):**
1. Coletar produção acumulada (lazy evaluation).
2. Revisar notificações (construção pronta, tropa chegou, ataque, aliança).
3. Reabastecer filas (construção, pesquisa, recrutamento).
4. 1-2 ações ativas (explorar/atacar/contribuir ao Nó/motivar aliados).
5. Coordenar com a aliança.

**Hooks de retorno:** construção termina em X h; tropa chega em Y min; Nó precisa de contribuição; evento temporário; relatório de ataque.

**Pilares:** progressão por eras; cidade que evolui visualmente; cooperação que destrava o endgame; PvP com stakes sem trauma; sessões curtas competitivas (quem joga pouco e bem compete).

---

## 5. Recursos e economia

**3 recursos âncora universais** que re-skinizam por era (mesma função, nome/visual muda):
- **Matéria** (estrutura/construção) — Pedra → Mármore → Aço → Liga Cristalina.
- **Energia** (produção/movimento) — Lenha → Carvão → Petróleo/Eletricidade → Plasma.
- **Conhecimento** (pesquisa) — Pergaminhos → Manuscritos → Sinal → Dados Quânticos.

Ao avançar de era, uma "Forja de Era" transmuta recursos antigos nos novos (continuidade, sem desperdício).

**Fio Bruto** — recurso cooperativo, só vem dos Nós dos Tecelões e eventos. Necessário para ativar Nós (avanço de era / endgame).

**Produção:** passiva, por **lazy evaluation** (estado calculado sob demanda a partir de timestamps). Armazenamento limitado, com parcela sempre "abrigada" contra saque.

**Princípio econômico fundamental (decisão firme):** **espaço/edifícios NUNCA são paywall.**
- Cada era define um **conjunto de edifícios** construíveis, com **dependências/pré-requisitos** entre eles (árvore).
- Edifícios econômicos (pedreira, mina, etc.) têm **quantidade pré-estabelecida** construível por todos, que **cresce conforme as eras**.
- Todo jogador consegue construir tudo que precisa **sem pagar nada**.

---

## 6. Cidade única e slots

Modelo **Forge of Empires**: o jogador tem **uma cidade permanente e inviolável** (nunca é tomada). Cresce densificando + ganhando espaço.

**Slots de construção:**
- Abrem ao **avançar de era** (de graça) — cada era nova libera mais slots.
- Complementados por meios **gratuitos** in-game (pesquisa, conquista de províncias PvE, marcos de aliança).
- **Nunca vendidos por dinheiro.**

**Evolução visual:** a cidade muda de estética automaticamente a cada era (da terra batida da Era 1 à arquitetura translúcida pulsando com fios de luz na Era 7). Edifícios de eras anteriores podem ser mantidos ou substituídos.

---

## 7. Progressão de eras

Para avançar de era, o jogador cumpre:
- **A)** Pesquisar **≥75%** das tecnologias da era atual (árvore com ramificações/escolhas).
- **B)** Construir e ativar o **Marco da Era** (monumento único; exige os 3 recursos + um insumo de província PvE daquela era).
- **C)** Ter conquistado **≥60% das províncias** do anel da era atual (vincula progresso vertical ao horizontal).
- **D)** Contribuir **≥5%** ao Nó da era — **recomendado, não obrigatório** (dá +10% de eficiência por 30 dias; não bloqueia o avanço, para não punir jogadores por política de aliança).

Avançar abre: novos edifícios, unidades, recursos de era, ramo da árvore, e o próximo anel do mapa.

---

## 8. Mapa do mundo e expansão PvE

**Mapa hexagonal persistente e COMPARTILHADO por servidor/world** (modelo Travian/OGame/RoK-assíncrono — **revisão 2026-06-04**, ver nota abaixo). Todos os jogadores do mesmo world ocupam o **mesmo mapa**: veem vizinhos, território de aliança, e o conteúdo PvE nas **mesmas posições**. Tudo **assíncrono** (marchas = timers; **sem tempo real / sem WebSocket**). Camadas:
- **Cidades dos jogadores** — em tiles reais do mapa compartilhado (posição = `coord_x/coord_y`). **Inviolável** (nunca tomada), mas saqueável (§11).
- **Conteúdo PvE** (acampamentos/regiões de NPC) — em **tiles compartilhados** (todos veem). Combate **resolvido individualmente por jogador** (modelo RoK: sem "roubo de kill"); conquistar uma região dá ao conquistador espaço de cidade, recursos, depósito passivo, ranking. Dificuldade cresce com a **distância da origem do world / faixa de era**.
- **Territórios de Aliança** — disputa PvP coletiva (§10); colorem/controlam regiões do mapa.
- **Nós dos Tecelões** — 1 por era, pontos fixos disputados.

**Névoa / exploração (planejado, decisão 2026-06-07):** o mapa-mundo começa coberto por **névoa (fog of war)** por jogador; **batedores** (ver §9/espionagem) **exploram e revelam** a névoa, além de espionar. Detalhes do rework de batedores (imortais, sem treino, gate por era/nível, teto 5) em memória `design-batedores-nevoa` — é um épico próprio, árvore a definir.

**Escala:** **sharding por world/servidor** — cada world tem N jogadores (milhares); abre-se um novo world quando lota. Dentro de um world, mapa 100% compartilhado. Viável para dev solo em Go+Postgres+Redis pois NÃO há tempo real (o caro do mundo compartilhado é o realtime/broadcast, que o design assíncrono elimina). Grid com índice espacial `(x,y)`; marchas resolvidas por job no `arrive_at`.

> **Nota de revisão (2026-06-04):** a versão anterior instanciava o PvE por jogador ("bolsões de Lacuna") por crer que mundo compartilhado era inviável p/ dev solo. Pesquisa de mercado mostrou que isso só vale para mundo compartilhado **em tempo real**; o **assíncrono** (Travian/OGame rodaram décadas em PHP+MySQL) é barato e é o padrão do gênero — e dá a "sensação de mundo vivo" que o instanciado não dá. Detalhes de como as faixas de dificuldade/era mapeiam no mapa compartilhado: a refinar na implementação.

---

## 9. Combate

**Decisão (2026-06-07): o combate é resolvido por AUTO-RESOLVE + RELATÓRIO** — coração único do sistema, tanto PvE (marcha) quanto PvP (saque). É o que o mundo compartilhado assíncrono exige (o defensor está offline quando atacado → PvP *tem* que auto-resolver) e o que escala para dezenas de conflitos simultâneos (alianças/território). Camadas:

1. **Mapa Vivo (desde o MVP):** exércitos marcham visivelmente pelo mapa como **timers** (não tempo real). Posição importa; vê-se a ameaça chegando; dá para interceptar/reforçar/recuar de forma assíncrona. (Rouba a "alma" do Rise of Kingdoms sem o custo de netcode em tempo real.)

2. **Resolução automática + relatório (PRIMÁRIA):** determinística (seed persistida → auditável/anti-cheat). A **profundidade** mora nas decisões ANTES da batalha: composição e **counters** (lanceiro × arqueiro × cavalaria futura), **defesa** (guarnição + Torre + Muralha), **espionagem** (batedores), capacidade de marcha e **timing**. Terreno/bônus entram como **modificadores no cálculo** — não jogados na mão.

3. **Batalha Tática hex (CONSTRUÍDA, PARQUEADA):** motor turno-a-turno em grade hexagonal com Lacunas/terreno já implementado e testado, mas **fora do loop ativo** (entrada escondida na UI). Reservada para **conteúdo PvE especial futuro** (chefes, masmorras, criaturas nomeadas — o jogador *escolhe* comandar na mão por recompensa melhor, à la Heroes of Might & Magic dentro do 4X). **Nunca** no PvP nem no saque do dia a dia. Código mantido dormente no repo.

**Limite militar — capacidade de marcha (decisão 2026-06-07):**
- **Sem teto de POSSE:** o exército total cresce livre (freio = economia + tempo de treino e, no futuro, **upkeep**). A **defesa usa TODA a guarnição em casa**.
- A **ofensa** é limitada pela **capacidade de marcha** (máx. de tropas por expedição), somada ao nº de **marchas simultâneas** (slots de expedição, crescem por era). Assimetria deliberada: defende-se com tudo, ataca-se com o que a marcha comporta.
- A capacidade **sobe por (a) avanço de ERA (bump automático) e (b) PESQUISA** (numa Academia / edifício de pesquisa a definir). A pesquisa aplica-se a **TODAS as marchas de uma vez** (uniforme), inclusive as obtidas depois — **sem** upgrade/custo por marcha individual. **Sem herói/XP** (decisão explícita do dono: a marcha não vira personagem).
- Anti-P2W: capacidade **nunca** sobe por dinheiro. Itens que afetam capacidade/marcha são **PvE-only** (ver §13).

**Tropas:** recrutadas com custo + tempo. Tipos por categoria evoluem por era (infantaria, projétil, cavalaria/veículo, suporte, artilharia/especial de facção). Tropas de era anterior continuam usáveis, mas menos eficientes.

**Modelo rejeitado:** combate de manobra em tempo real (estilo RoK) — descartado por inviabilidade técnica (exigiria WebSocket + tick loop + netcode; custo de infra 8-20×; proibitivo para dev solo).

---

## 10. Alianças e cooperação

**Assíncrono e coordenado (nada em tempo real).**

**Ciclos de Conflito (14 dias):** alianças agrupadas numa Arena por liga disputam ~30 territórios. Cada aliança tem **Sede inviolável**. Membros enviam exércitos (marcha = timer) contra territórios defendidos por NPC + formações pré-configuradas dos defensores; vitórias acumulam "avanços" até o território trocar de dono (modelo Guild Battlegrounds do FoE). Território dá **bônus coletivo** (produção, recurso raro, Pontos de Vitória) — perder = perde o bônus, **nunca a cidade**.

**Nós dos Tecelões:** controlar a região vizinha dá prioridade de ativação; mas o Nó só ativa com **3+ facções diferentes** (4 nas eras 4-6, 5 na era 7) → força coalizão e diversidade.

**A aliança oferece:** cofre coletivo, mapa compartilhado, **Quartel de Aliança** (tropas mais fortes pagas com recurso *coletivo*, não dinheiro), missões semanais, chat/coordenação.

**Solo vs aliança:** jogador solo progride confortavelmente até ~Era 4-6; o endgame (Era 7) exige aliança por design. Cooperação é convidada progressivamente, nunca forçada cedo.

*(Modelo aceito provisoriamente; calibrar tamanho de aliança, frequência de ciclos e defesa com dados reais.)*

---

## 11. PvP e stakes

**Sem conquista de cidade.** Camadas:
- **Saque/pilhagem:** vencer um ataque rouba a produção pendente de **um** edifício (nunca estoque, pesquisa ou progresso). Edifícios "motivados" por aliados ficam imunes (mecânica social). Vítima tem vingança grátis. Escudo de proteção disponível.
- **Guerra de território de aliança** (ver §10).
- **PvP sazonal/eventos** com recompensas.

**Stakes = oportunidade, tempo e posição** — nunca perda existencial. Resolve a crítica de "jogo sem stakes" sem o trauma/toxicidade da conquista.

---

## 12. Lifecycle, temporadas e endgame

**Temporadas de 90-120 dias.** Cada temporada é um mundo novo zerado.

**Endgame — "Rito do Fio Completo":** ativar o Nó da Era 7 exige representação das **5 facções** + coalizão (nenhuma aliança isolada acumula sozinha). Evento ao vivo de 48h. Quem completa encerra a temporada; contribuintes ganham título "Tecelão do Fim".

**Persiste entre temporadas:** cosméticos, títulos, histórico, bônus de "Memória da Terra" (reconquista mais barata). O mapa reseta.

---

## 13. Monetização

**Modelo:** F2P inclinado ao estilo FoE, **anti-pay-to-win-duro**. Moeda premium **"Crônicas"** (comprável **E** ganhável jogando em quantidade moderada).

**✅ Vender:**
- **Aceleração de progresso civil/econômico** (instant-finish / redução de tempo de construção e pesquisa) — com **CUSTO ESCALANTE** (itens curtos baratos, longos caros; o preço é o freio principal; teto diário fica de reserva).
- **Aceleração de TREINAMENTO de tropas** — com **4 travas**: só acelera produção (nunca deixa tropa mais forte); **capacidade de marcha por era/pesquisa** (pagar treina/repõe mais rápido, **não** coloca mais tropa em campo de uma vez); custo escalante; **nunca** reforço instantâneo em combate ativo.
- **Consumíveis temporários CIVIS** (fila extra de obra/pesquisa/espião por X horas) — vendáveis. **Itens que afetam capacidade ou nº de marchas de TROPAS são PvE-only** (bloqueados em PvP/saque/guerra de aliança) — nunca poder militar comprável em PvP.
- **Conveniência/QoL** via assinatura **"Velarum Plus"** (filas extras, painel, auto-coleta, notificações).
- **Cosmético** (skins de cidade/unidade por era e facção, banners de aliança, títulos) — pilar de receita, catálogo gigante (7 eras × 5 facções).

**❌ Nunca vender:** recursos brutos; poder militar/tropas diretas; tropa/reforço instantâneo em batalha ativa; **espaço/slots de cidade**; nada que trave a progressão atrás de pagamento.

**Por que vender aceleração é seguro aqui (travas estruturais):** capacidade de marcha por era/pesquisa (pagar = treinar/repor mais rápido, **não** despejar mais tropa de uma vez); combate por **composição/counters** (não numérico puro); sem conquista de cidade; **endgame cooperativo** (ninguém compra a vitória sozinho); Crônicas também ganháveis de graça.

---

## 14. Arte e pipeline visual

**Estilo:** 2D **isométrico limpo** com **outline preto uniforme** (truque-chave para coesão de assets de IA). Lacunas como assinatura visual (geometria quebrada, cor vazando). Diferenciação de facção por paleta/silhueta/adornos.

**Maior gargalo do projeto:** volume de arte (7 eras × 5 facções). Estratégia: **modularidade** (1 design base → 35 variantes via IA) + treino de **LoRA/modelo próprio** (~20 "assets âncora" definidos manualmente — única coisa que IA não substitui).

**Stack de IA:** Scenario.gg ou Leonardo.ai (geração + treino de estilo) + **Gemini 2.5 Flash "Nano Banana"** via API (volume + edição por prompt) + ComfyUI local (avançado). Pós-processo automatizável: rembg (remover fundo) → Real-ESRGAN (upscale) → TexturePacker (atlas PixiJS). Animação: **Spine 2D** (rig manual) para unidades.

**Custo:** ~US$ 50-130/mês de ferramentas durante o desenvolvimento.

**Nota:** o assistente de código (Claude) **não gera imagens**, mas pode **automatizar o pipeline** (script Gemini API → processamento → atlas).

---

## 15. Stack técnica e infraestrutura

**Backend: Go** (escolhido por base sólida de longo prazo + scheduler de eventos nativo via goroutines).
- HTTP: chi · DB: PostgreSQL via pgx/sqlc (evitar GORM) · migrations: goose · cache/filas/locks/pub-sub: Redis (go-redis).
- **Arquitetura:** monolito modular; **lazy evaluation** de recursos; **eventos futuros agendados** persistidos no banco e recarregados no boot; app **stateless**; **`world_id` em toda tabela** (sharding por mundo); domínio **puro sem I/O** (`internal/domain`); workers de fila separáveis.
- Comunicação: REST + SSE (WebSocket só p/ chat). Combate tático: thin client, estado em Redis, determinístico.

**Frontend: 2D, PixiJS v8** (cidade/mapa/batalha) + **React** (UI) + Vite + TypeScript. Mobile depois via PWA/Capacitor.

**Contrato front/back:** **oapi-codegen** (gera cliente TS do spec OpenAPI do servidor Go) — adotado desde o início.

**Infra inicial:** VPS OVH 4GB/4vCore (já do usuário) — suficiente com folga para dev + 4-5 testers; tudo (app + Postgres + Redis + Caddy) na mesma máquina via Docker Compose; Cloudflare free na frente. Custo adicional ~zero. Escala futura: Hetzner dedicado.

---

## 16. Status de propriedade intelectual (nomes)

Checagem de colisão feita em 2026-06-02. **Veredito geral: risco baixo a médio, sem red flags graves.** Nomes não foram copiados de jogos existentes (são neologismos originais em sua maioria). *Lembrete jurídico: copyright NÃO protege nomes; o risco real é marca registrada (trademark), por classe — relevantes: Nice 9, 41, 42.*

| Nome | Risco | Observação / ação |
|------|-------|-------------------|
| **Velarum** (mundo/título) | **MÉDIO** | Existe jogo indie obscuro "VELARUM" (2015, sem marca encontrada) + é palavra latina real (toldo romano). **Verificar formalmente no INPI/USPTO/EUIPO antes do lançamento**; considerar subtítulo diferenciador ou nome alternativo. |
| Aurenthos, Brevali, Sorenthai, Valdruun | BAIXO | Neologismos originais, sem colisão. |
| Kethari | BAIXO-MÉDIO | Só em wikis de fã e startup com grafia diferente (setor distinto). |
| Tecelões do Vazio | BAIXO (PT) | Evitar a versão inglesa "Voidweaver(s)" como marca (existe em Warhammer 40k). |
| Fio Solto | BAIXO (PT) | Evitar "Loose Thread(s)" em inglês como nome de produto (Evil Hat publicou RPG). |
| Lacunas | BAIXO como termo interno | **Não** usar como título do jogo (existe jogo "Lacuna"). |
| Crônicas (moeda) | BAIXO como moeda | **Não** usar "Chronicles" no título (espaço saturado). |
| Nomes de eras | BAIXO | Frases descritivas, sem colisão. |

**To-do de clearance antes do lançamento:** busca formal INPI (Brasil) + USPTO + EUIPO nas classes 9/41/42; checar domínio `.com`/`.com.br` e redes sociais; consultar advogado de PI para o título final.

---

## 17. Escopo do MVP

**Objetivo:** primeira temporada jogável, coerente, com sistemas principais em versão mínima — não polida. Dev solo.

**Gameplay:**
- **3 eras** (Primeiros Fogos, Metais e Marés, Grandes Rotas).
- **5 facções**, cada uma com **2 mecânicas** (não 4).
- Cidade com grid + slots por era; ~40 tipos de edifício nas 3 eras.
- Mapa: anéis 1-2 (províncias era 1-3); Nós das eras 1 e 2.
- **Mapa Vivo desde o MVP** (marcha como timer — versão barata).
- Combate: **auto-resolução sólida** + batalha tática enxuta (opcional); grade 6×6, poucas unidades, 1-2 tipos de tile de Lacuna.
- PvP de saque completo; Ciclo de Conflito simplificado (4 territórios, 2 ligas).
- Alianças: cofre, chat, mapa compartilhado, missões básicas.
- 1 tipo de evento (Expedição dos Tecelões).

**Monetização MVP:** assinatura "Velarum Plus" + Crônicas (comprável/ganhável) + catálogo cosmético inicial + aceleração civil com custo escalante.

**Arte MVP:** ~120-150 assets (priorizar 2 facções × 3 eras completas, demais simplificadas); ~3-4 meses solo com pipeline de IA.

**Prioridade de polimento:** (1) core loop de cidade; (2) saque/defesa; (3) progressão visual de era; (4) aliança básica; (5) batalha tática enxuta.

**Roadmap pós-MVP:** eras 4-7; mecânicas completas das facções; mais eventos; ligas extras; mecânicas avançadas de facção.

---

## 18. Decisões em aberto

- Detalhes finos de aliança (tamanho, frequência de ciclos, mecânica de defesa) — calibrar com dados.
- Existência de vilão/ameaça PvE comum (não definido).
- Teto diário de aceleração (reserva, só se houver abuso).
- Decisão final do nome do mundo/título (Velarum vs alternativa) após clearance formal.
- Preços concretos (Crônicas, assinatura) e catálogo cosmético detalhado.
- Árvore de pesquisa detalhada por era/facção; fórmulas de combate/produção/viagem.
