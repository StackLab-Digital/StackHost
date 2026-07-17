# StackHost — Definição do Produto

## 1. Visão

O **StackHost** será uma plataforma open source e self-hosted para instalar, publicar e gerenciar aplicações Docker em uma infraestrutura Docker Swarm.

Ele combina:

* a capacidade operacional do **Portainer**;
* a simplicidade de instalação do **Easypanel**;
* a experiência de deploy e acompanhamento da **Vercel**;
* a automação de infraestrutura do **Coolify**.

A diferença central é que o StackHost será projetado primeiro para usuários leigos.

O usuário não deverá precisar entender:

* Docker Swarm;
* services e tasks;
* overlay networks;
* replicas;
* Traefik;
* labels;
* healthchecks;
* certificados SSL;
* registries;
* rolling updates;
* rollback;
* volumes;
* comandos Docker.

Esses conceitos existirão internamente, mas serão traduzidos em ações simples como:

* Instalar
* Publicar
* Atualizar
* Restaurar
* Reiniciar
* Ver logs
* Adicionar domínio
* Aumentar capacidade

---

## 2. Proposta de valor

> Instale e gerencie aplicações em sua própria VPS com a experiência de uma plataforma cloud, sem pagar mensalidade pela plataforma e sem precisar dominar Docker.

O StackHost transforma uma VPS comum em uma plataforma visual de hospedagem de aplicações.

Depois da instalação inicial, o usuário poderá operar o ambiente sem voltar ao terminal.

---

## 3. Público inicial

O ICP principal será formado por:

* pequenas empresas que hospedam seus próprios sistemas;
* revendedores e instaladores de Chatwoot;
* agências que administram aplicações de clientes;
* usuários de n8n, Evolution API, WAHA, WordPress e Chatwoot;
* desenvolvedores que querem uma experiência semelhante à Vercel em sua própria infraestrutura;
* profissionais que conhecem o básico de VPS, mas não dominam Docker;
* empresas que não querem depender de uma plataforma SaaS de hospedagem.

Cada empresa instalará sua própria instância do StackHost.

O StackHost não será uma plataforma SaaS multiempresa na primeira versão.

---

## 4. Princípios obrigatórios

### Leigos primeiro

Toda funcionalidade deve ter um fluxo simples antes de possuir uma interface avançada.

Uma funcionalidade não estará pronta apenas porque pode ser executada por meio de YAML, terminal ou formulário técnico.

### Complexidade progressiva

A interface terá dois níveis:

#### Modo simples

* linguagem acessível;
* decisões automáticas;
* valores recomendados;
* poucos campos;
* ações orientadas;
* alertas explicando riscos;
* ausência de termos internos do Docker sempre que possível.

#### Modo avançado

* Compose completo;
* variáveis;
* constraints;
* replicas;
* recursos;
* networks;
* secrets;
* configurações de deploy;
* acesso aos detalhes do Swarm.

### Operações seguras por padrão

Antes de alterações críticas, o StackHost deverá:

1. validar o ambiente;
2. verificar espaço em disco;
3. salvar a configuração atual;
4. criar backup quando aplicável;
5. executar rolling update;
6. acompanhar o healthcheck;
7. realizar rollback quando a atualização falhar.

### Sem aprisionamento

Os apps continuarão sendo aplicações Docker normais.

O usuário deverá conseguir exportar:

* arquivos Compose;
* variáveis;
* configurações;
* backups;
* dados;
* instruções de restauração.

---

## 5. Infraestrutura suportada

O StackHost utilizará **Docker Swarm desde a primeira versão**.

Mesmo instalações com apenas uma VPS serão inicializadas como um Swarm de nó único.

Isso permitirá que a mesma instalação evolua para múltiplos nós sem alterar o modelo operacional.

### Primeira versão

* Linux;
* arquitetura AMD64 e ARM64 quando as imagens utilizadas permitirem;
* Swarm de nó único;
* Swarm com múltiplos managers e workers;
* serviços replicados;
* serviços globais;
* overlay networks;
* rolling updates;
* rollback;
* placement constraints;
* secrets e configs do Swarm.

### Fora do escopo inicial

* Kubernetes;
* Docker Desktop;
* Windows Containers;
* Nomad;
* gerenciamento de máquinas virtuais;
* gerenciamento de clusters de terceiros;
* provisionamento completo de VPS em provedores cloud.

---

## 6. Instalação

A instalação será dividida em duas etapas.

### Etapa 1 — Bootstrap por terminal

O usuário copiará um único comando, semelhante a:

```bash
curl -fsSL https://get.stackhost.dev | sudo sh
```

O instalador deverá:

* detectar o sistema operacional;
* validar memória, CPU e disco;
* instalar ou atualizar Docker;
* inicializar o Docker Swarm;
* criar as redes internas;
* instalar o StackHost;
* instalar o proxy reverso;
* gerar segredos criptográficos;
* exibir a URL de acesso ao painel.

### Etapa 2 — Onboarding visual

No navegador, o usuário deverá:

1. criar o administrador;
2. definir o nome do servidor;
3. configurar o domínio principal;
4. escolher o método de HTTPS;
5. configurar notificações;
6. configurar backup;
7. instalar o primeiro aplicativo.

Depois desse processo, o terminal deixa de ser necessário para a operação normal.

---

## 7. Modelo de navegação

A interface será organizada nos seguintes conceitos:

### Visão geral

Estado geral da infraestrutura:

* aplicações online;
* aplicações com falha;
* deploys recentes;
* uso de CPU;
* uso de memória;
* uso de disco;
* backups recentes;
* atualizações disponíveis;
* alertas.

### Projetos

Um projeto agrupa aplicações relacionadas.

Exemplos:

* Atendimento
* Automação
* Site institucional
* Ambiente do cliente X

### Aplicações

Uma aplicação é o objeto principal visto pelo usuário.

Exemplos:

* Chatwoot
* n8n
* WordPress
* Evolution API
* API própria
* aplicação Rails

Uma aplicação poderá possuir vários serviços internos, como:

* web;
* worker;
* PostgreSQL;
* Redis;
* Sidekiq;
* proxy;
* cron.

No modo simples, esses serviços aparecem como componentes da aplicação, não como objetos Docker desconectados.

### Ambientes

Cada projeto poderá possuir ambientes como:

* produção;
* homologação;
* desenvolvimento.

Ambientes não serão obrigatórios para instalações simples.

### Servidores

Tela com:

* managers;
* workers;
* disponibilidade;
* uso de recursos;
* labels;
* funções;
* versão do Docker;
* serviços executados;
* ações de manutenção.

### Deploys

Histórico imutável contendo:

* origem;
* versão;
* commit;
* imagem;
* usuário responsável;
* horário;
* duração;
* logs;
* resultado;
* configuração utilizada;
* opção de rollback.

---

## 8. Formas de criar uma aplicação

A primeira versão pública terá quatro fluxos.

### 8.1 Catálogo

Fluxo principal para leigos.

O usuário deverá:

1. escolher o aplicativo;
2. informar nome e domínio;
3. preencher os campos solicitados;
4. revisar a instalação;
5. clicar em “Instalar”.

O StackHost cuidará de:

* serviços;
* banco;
* Redis;
* volumes;
* secrets;
* redes;
* domínio;
* HTTPS;
* healthchecks;
* ordem de inicialização;
* recursos recomendados.

### 8.2 Docker Compose

O usuário poderá:

* colar um Compose;
* enviar um arquivo;
* importar de um repositório;
* editar visualmente;
* visualizar incompatibilidades com Swarm.

O StackHost deverá analisar o arquivo e informar:

* campos não suportados pelo Swarm;
* volumes sem persistência;
* portas expostas;
* secrets escritos diretamente;
* ausência de healthcheck;
* ausência de limites;
* imagens com tags inseguras, como `latest`.

### 8.3 Imagem Docker

O usuário informará:

* imagem;
* tag;
* porta;
* variáveis;
* domínio;
* volumes opcionais.

A aplicação poderá vir de:

* Docker Hub;
* GitHub Container Registry;
* GitLab Registry;
* registry privado;
* registry interno do StackHost.

### 8.4 Repositório Git

O fluxo será semelhante à Vercel:

1. conectar repositório;
2. escolher branch;
3. detectar Dockerfile;
4. configurar variáveis;
5. executar build;
6. publicar imagem;
7. realizar deploy;
8. acompanhar logs.

A primeira versão poderá utilizar:

* URL pública;
* token de acesso;
* chave SSH;
* webhook configurado pelo usuário.

A integração por GitHub App poderá ser adicionada posteriormente sem alterar o modelo de aplicação.

---

## 9. Build e registry

Como Docker Swarm não realiza builds diretamente, o StackHost terá um pipeline próprio.

Fluxo:

1. clonar o repositório;
2. preparar o contexto;
3. executar o build com BuildKit;
4. gerar uma tag imutável;
5. enviar a imagem para um registry;
6. atualizar o serviço no Swarm;
7. monitorar o deploy.

A tag deverá usar commit ou identificador de deploy:

```text
registry.local/projeto/app:git-a81f29c
```

O StackHost poderá instalar um registry privado interno para instalações que não possuam um registry externo.

Credenciais deverão ser armazenadas criptografadas.

---

## 10. Catálogo de aplicações

O catálogo será baseado em arquivos e versionado por Git.

Estrutura inicial:

```text
catalog em arquivos e versionado por/
├── official/
│   ├── chatwoot/
│   ├── n8n/
│   ├── wordpress/
│   └── evolution-api/
├── community/
└── private/
```

Cada aplicativo deverá possuir:

```text
chatwoot/
├── compose.yml
├── stackhost.yml
├── icon.svg
├── README.md
└── versions/
```

### `compose.yml`

Define os serviços Docker utilizando o padrão Compose compatível com Swarm.

### `stackhost.yml`

Define a experiência visual:

* nome;
* descrição;
* categoria;
* documentação;
* versão;
* campos solicitados;
* valores padrão;
* geração de senhas;
* serviços públicos;
* domínios;
* volumes;
* dependências;
* requisitos mínimos;
* políticas de atualização;
* hooks de backup;
* verificações de saúde.

Exemplo conceitual:

```yaml
name: Chatwoot
category: Atendimento
version: 4.15.1

requirements:
  memory: 4096
  cpu: 2
  disk: 20GB

fields:
  - key: domain
    label: Domínio
    type: domain
    required: true

  - key: postgres_password
    label: Senha do banco
    type: password
    generate: true
```

### Catálogo oficial

* mantido pelo projeto;
* revisado;
* testado;
* exibido primeiro;
* identificado como verificado.

### Catálogo comunitário

* distribuído por Git;
* contribuição via pull request;
* selo comunitário;
* aviso sobre nível de revisão;
* possibilidade de avaliações e histórico de compatibilidade.

### Catálogos privados

Empresas poderão adicionar repositórios próprios contendo templates internos.

Isso permitirá que a StackLab, revendedores e agências mantenham instalações padronizadas sem publicar seus templates.

---

## 11. Domínios e HTTPS

O StackHost utilizará Traefik internamente.

O usuário não precisará editar labels.

### Métodos suportados

#### Domínio temporário

Para testes rápidos, a plataforma poderá gerar um endereço baseado no IP usando um serviço compatível com wildcard DNS.

#### Domínio base

O usuário poderá configurar:

```text
*.apps.empresa.com
```

Novas aplicações receberão automaticamente subdomínios como:

```text
chatwoot.apps.empresa.com
n8n.apps.empresa.com
```

#### Domínio próprio

O usuário informa o domínio, e o StackHost:

* identifica o IP esperado;
* mostra o registro DNS necessário;
* verifica a propagação;
* ativa HTTPS;
* acompanha a renovação.

#### Cloudflare

Integração opcional para:

* criar registros DNS;
* ativar ou desativar proxy;
* emitir certificados por DNS challenge;
* configurar wildcard;
* validar a configuração.

Tokens da Cloudflare deverão utilizar permissões mínimas.

---

## 12. Deploy, atualização e rollback

### Atualização assistida

Fluxo padrão:

1. identificar a nova versão;
2. mostrar release notes;
3. verificar compatibilidade;
4. verificar espaço;
5. criar backup;
6. armazenar a configuração atual;
7. iniciar rolling update;
8. monitorar tasks e healthchecks;
9. concluir ou reverter.

### Rollback automático

O rollback será executado quando:

* o healthcheck falhar;
* tasks entrarem repetidamente em estado de erro;
* o tempo máximo for ultrapassado;
* o serviço não alcançar o número mínimo de réplicas;
* um hook de validação retornar erro.

### Atualizações automáticas

Serão opcionais e desativadas por padrão.

Canais possíveis:

* estável;
* patch;
* minor;
* beta;
* versão fixada.

Aplicações stateful e bancos terão políticas mais conservadoras.

Atualizações automáticas de bancos não serão ativadas sem uma política explícita no template.

---

## 13. Backups

O StackHost realizará backups de:

* bancos;
* volumes;
* configurações;
* Compose processado;
* secrets criptografados;
* metadados da aplicação.

### Destinos

* disco local;
* S3;
* Cloudflare R2;
* MinIO;
* Backblaze B2 por API compatível com S3;
* outros provedores S3 compatíveis.

### Tipos

* backup manual;
* backup agendado;
* backup anterior ao deploy;
* backup anterior à atualização;
* backup anterior à remoção.

### Adaptadores iniciais

* PostgreSQL;
* MySQL/MariaDB;
* volumes genéricos;
* arquivos de configuração.

### Restauração

O usuário poderá restaurar:

* aplicação completa;
* apenas banco;
* apenas um volume;
* configuração anterior.

Antes da restauração, a plataforma mostrará:

* data;
* tamanho;
* origem;
* versão da aplicação;
* integridade;
* itens que serão substituídos.

Backups armazenados somente na mesma VPS deverão receber um alerta visível.

---

## 14. Monitoramento

A primeira versão exibirá:

* CPU por servidor e aplicação;
* memória;
* espaço em disco;
* tráfego básico;
* réplicas esperadas e ativas;
* reinicializações;
* estado dos healthchecks;
* tempo de atividade;
* crescimento de volumes;
* erros recentes.

A tela principal deverá responder rapidamente:

* Está funcionando?
* O servidor está sobrecarregado?
* Algum app está reiniciando?
* O disco está próximo de lotar?
* O último backup funcionou?
* Existe alguma atualização arriscada pendente?

O sistema utilizará coleta leve por meio da API do Docker e métricas do host.

---

## 15. Logs e diagnóstico

### Logs

* visualização em tempo real;
* busca;
* filtro por serviço;
* filtro por réplica;
* pausa;
* download;
* intervalo de tempo;
* indicação de erros.

### Diagnóstico orientado

Em vez de exibir somente mensagens técnicas, o StackHost deverá interpretar estados comuns.

Exemplo:

```text
O PostgreSQL está recusando novas conexões.

Possíveis causas:
- limite de conexões atingido;
- aplicação usando conexões demais;
- banco ainda inicializando.

Ações:
- visualizar conexões;
- reiniciar aplicação;
- abrir configurações avançadas.
```

O diagnóstico deverá ser baseado inicialmente em regras determinísticas, não depender obrigatoriamente de IA.

---

## 16. Notificações

A primeira versão terá:

* e-mail;
* webhook genérico.

Eventos notificáveis:

* deploy concluído;
* deploy falhou;
* rollback executado;
* aplicação indisponível;
* disco crítico;
* servidor offline;
* backup falhou;
* backup concluído;
* atualização disponível;
* certificado próximo do vencimento.

O webhook permitirá integração com:

* n8n;
* Chatwoot;
* StackZap;
* Slack;
* Discord;
* Telegram;
* sistemas internos.

---

## 17. Equipes e permissões

Cada instalação pertence a uma única empresa.

Não haverá multiempresa compartilhando o mesmo painel.

### Perfis

#### Administrador

* controle completo;
* servidores;
* usuários;
* segurança;
* backups;
* configurações globais;
* remoção de aplicações.

#### Operador

* deploy;
* atualização;
* rollback;
* reinício;
* backups;
* logs;
* monitoramento.

#### Desenvolvedor

* aplicações autorizadas;
* variáveis;
* builds;
* deploys;
* logs;
* domínio;
* terminal quando permitido.

#### Somente leitura

* dashboard;
* aplicações;
* deploys;
* métricas;
* logs sem dados sensíveis.

Permissões deverão ser aplicáveis por projeto.

---

## 18. Segurança

Requisitos mínimos:

* senhas com hash seguro;
* suporte a autenticação de dois fatores;
* sessões revogáveis;
* auditoria de ações;
* secrets criptografados em repouso;
* mascaramento de variáveis sensíveis;
* proteção contra exposição acidental em logs;
* tokens com escopo;
* acesso ao Docker Socket isolado;
* proteção contra templates maliciosos;
* confirmação reforçada para ações destrutivas.

Toda ação crítica deverá registrar:

* usuário;
* data;
* endereço IP;
* aplicação;
* ação;
* resultado;
* configuração anterior e posterior quando aplicável.

---

## 19. Arquitetura técnica proposta

### Backend

* Go;
* API HTTP;
* WebSocket ou Server-Sent Events para logs e eventos;
* Docker Engine API;
* Docker Swarm API;
* workers internos para deploys, backups e builds.

### Frontend

* Vue 3;
* TypeScript;
* Pinia;
* Vue Router;
* Tailwind CSS;
* componentes acessíveis;
* atualização reativa de estados;
* interface responsiva.

### Persistência

Para reduzir a complexidade da instalação inicial:

* SQLite como padrão;
* volume persistente;
* migrations versionadas;
* possibilidade futura de PostgreSQL externo.

### Serviços internos

* StackHost API;
* StackHost Worker;
* Traefik;
* registry privado opcional;
* serviço de build;
* coletor de métricas;
* scheduler de backups.

### Organização do repositório

```text
stackhost/
├── cmd/
│   ├── server/
│   ├── worker/
│   └── installer/
├── internal/
│   ├── apps/
│   ├── auth/
│   ├── backups/
│   ├── builds/
│   ├── catalog/
│   ├── deployments/
│   ├── docker/
│   ├── domains/
│   ├── metrics/
│   ├── notifications/
│   ├── swarm/
│   └── users/
├── web/
│   └── Vue application
├── catalog/
├── migrations/
├── scripts/
└── docs/
```

---

## 20. Escopo obrigatório do primeiro release público

O primeiro release somente estará completo quando possuir:

### Infraestrutura

* instalação por comando único;
* onboarding visual;
* Swarm de nó único;
* múltiplos nós;
* managers e workers;
* Traefik;
* HTTPS automático.

### Aplicações

* catálogo oficial;
* catálogos comunitários;
* catálogos privados;
* importação de Compose;
* deploy por imagem;
* deploy por Git;
* build de Dockerfile;
* registry.

### Operação

* logs em tempo real;
* status;
* reinício;
* escala de réplicas;
* deploy;
* rolling update;
* rollback manual;
* rollback automático;
* histórico de deploys.

### Dados

* backup local;
* backup S3;
* agendamento;
* retenção;
* restauração guiada;
* backup antes de atualização.

### Administração

* usuários;
* equipes;
* quatro perfis de acesso;
* permissões por projeto;
* auditoria.

### Observabilidade

* CPU;
* memória;
* disco;
* estado dos serviços;
* reinicializações;
* alertas;
* notificações por e-mail e webhook.

---

## 21. O que não entra no primeiro release

* Kubernetes;
* modelo SaaS multiempresa;
* cobrança e assinaturas;
* marketplace pago;
* provisionamento automático em AWS, Hetzner ou DigitalOcean;
* gerenciamento de DNS fora da Cloudflare;
* banco de dados como serviço independente;
* funções serverless;
* edge computing;
* gerenciamento de máquinas virtuais;
* aplicação móvel nativa;
* inteligência artificial como requisito central;
* substituição completa das funcionalidades técnicas do Portainer.

O modo avançado poderá crescer depois, mas não deverá atrasar ou prejudicar a experiência principal para leigos.

---

## 22. Critérios de sucesso

O produto estará cumprindo sua proposta quando um usuário sem conhecimento de Docker conseguir:

1. preparar uma VPS com um comando;
2. acessar o painel;
3. instalar um Chatwoot pelo catálogo;
4. conectar um domínio;
5. ativar HTTPS;
6. visualizar o estado da aplicação;
7. criar um backup externo;
8. atualizar com segurança;
9. acompanhar o deploy;
10. restaurar a versão anterior após uma falha.

Tudo isso sem escrever um Compose, configurar Traefik ou utilizar comandos Docker.

---

## 23. Posicionamento final

O StackHost não será apenas um painel para visualizar containers.

Será uma camada de produto sobre o Docker Swarm.

O Portainer mostra a infraestrutura como ela é.

O StackHost deverá mostrar a infraestrutura como o usuário precisa entendê-la.

> Docker Swarm por baixo. Experiência de cloud por cima. Controle total na infraestrutura do usuário.
# StackHost

StackHost is a small, self-hosted control panel for Docker and Docker Swarm.

Keep changes minimal, preserve existing behavior, avoid destructive Docker operations, and keep the application usable when Docker is unavailable.
