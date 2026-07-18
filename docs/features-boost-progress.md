# Features Boost

Base develop: `40c02ea5f91cce67d55b11cd8855d27674915b50`
Branch: `features-boost`
Início: `2026-07-17 17:27:45 -03`

## Fases

- [x] Fase 0 — Auditoria e estabilização
- [x] Fase 1 — Motor de deploy robusto
- [x] Fase 2 — Proxy nativo e domínios (base operacional entregue; renovação ACME e propagação DNS seguem observáveis)
- [x] Fase 3 — Controle operacional
- [x] Fase 4 — Logs
- [x] Fase 5 — Monitoramento e saúde (dashboard operacional, saúde do host, agregações e alertas derivados; métricas locais mantidas)
- [x] Fase 6 — Backups (backup local, retenção e agendamento manual/diário/semanal)
- [x] Fase 7 — Notificações (webhook/Discord/Slack configuráveis e eventos operacionais básicos)
- [ ] Fase 8 — Catálogo e produtividade (duplicação de aplicações entregue; importação avançada pendente)
- [ ] Fase 9 — Git público, somente se o restante estiver estável
- [ ] Fase 10 — UX e acabamento
- [ ] Fase 11 — Hardening
- [ ] Fase 12 — Instalação e documentação

## Estado atual

Fases 0–7 têm implementação funcional nesta branch. O motor de deploy assíncrono agora possui histórico, locks por aplicação, cancelamento, recuperação, readiness real, redaction, ações operacionais e logs limitados. O ingress nativo roda no mesmo binário, mantém cache de rotas, proxy HTTP/HTTPS com `autocert`, cadastro/checagem de domínios, proteção anti-SSRF e rede Docker gerenciada.

Cinco arquivos já possuíam alterações locais antes do início desta execução e foram preservados na branch: `cmd/stackhost/applications.go`, `internal/composevalidator/validator.go`, `internal/composevalidator/validator_test.go`, `web/src/style.css` e `web/src/views/ApplicationDetail.vue`.

Baseline validado: `go test ./...`, `go vet ./...`, typecheck, lint, 14 testes frontend, build web e build da imagem baseline. No navegador, uma instância isolada concluiu onboarding, criação de projeto, validação Compose e criação de aplicação.

O build Docker pós-correções passou com a imagem `stackhost:features-boost`. Uma instância temporária aplicou as oito migrations, respondeu a `/health/live`, `/health/ready`, `/api/v1/setup/status` e entregou a SPA em smoke test. A validação completa no navegador interativo ainda não foi repetida.

## Decisões técnicas

- Preservar alterações locais preexistentes e evitar misturá-las sem validação.
- Executar cada vertical com o menor patch compatível com os requisitos.
- Priorizar P0 antes de P1 e não iniciar P2 enquanto houver P0/P1 instável.
- Manter migrations sequenciais em `internal/migrations`, com uma transação por versão e readiness baseado no histórico completo.
- Tratar `applications.status` como estado operacional e `configuration_status` como estado da configuração.
- Rejeitar Compose que atravesse a fronteira do host antes de chamar Docker.
- Usar SSE de forma seletiva no frontend e polling moderado somente como fallback.
- Derivar destinos do ingress a partir das labels/serviços Docker e manter o proxy sem aceitar URL arbitrária do cliente.

## Pendências reais

- Repetir a validação completa no navegador interativo quando necessário; o build Docker e o smoke HTTP já passaram.
- Commits e push concluídos na branch `features-boost`: `86c6cf2` e `d4e462c`.
- ESLint agora possui configuração flat efetiva para arquivos JavaScript; cobertura específica de Vue/TypeScript depende da adição futura dos parsers/plugins correspondentes.
- Remover ou alinhar os arquivos SQL legados de `migrations/`, que não são consumidos pelo binário.
- A leitura de runtime da aplicação agora usa `internal/docker.Reader.RuntimeSnapshot`; o motor assíncrono já não herda `os.Environ()` nem depende de comandos espalhados nos handlers.
- Manifestos Docker/Swarm agora usam filesystem somente leitura, `/tmp` limitado e `no-new-privileges`; o socket continua exigindo acesso efetivo de escrita para operar workloads e permanece um risco explícito de host.
- `go test ./...`, `go vet ./...` e `go mod verify` passaram após alinhar as dependências Go.
- O runtime agora expõe health real do Docker SDK (`healthy`, `starting`, `unhealthy`, `no_healthcheck`) e uma amostra de métricas locais Docker (`/api/v1/applications/:id/metrics`) com retenção de 24 horas e UI resumida. Swarm permanece explicitamente `unknown` para métricas não locais. Notificações têm configuração protegida, teste de webhook, UI e entrega assíncrona para eventos publicados. Aplicações podem ser duplicadas preservando a origem criptografada e iniciando como `not_deployed`.
- Backup local do sistema entregue em `POST/GET /api/v1/backups/system`, com SQLite via `VACUUM INTO`, certificados persistidos em tar.gz, download e remoção protegidos por admin, controles em Configurações, retenção e scheduler interno manual/diário/semanal. Destinos S3 continuam pendentes.
- Ajuste incremental da fase de logs: `tail` agora aceita somente 100, 500 ou 1000; o viewer ganhou busca local, auto-scroll, cópia, download, limpeza visual e indicador de conexão.
- Dashboard operacional entregue em `/api/v1/dashboard`, com saúde do host em `/api/v1/system/health` e `/api/v1/system/resources`, agregação de aplicações/deploys/domínios/backups, alertas derivados e cards responsivos; métricas indisponíveis retornam `null`.

## Último commit validado

`6c8ed5f` — coleta multiplataforma de recursos do host no dashboard; publicado em `origin/features-boost`.

Última validação de código: `gofmt`, `git diff --check`, lint, typecheck, 14 testes frontend, build web, `go test ./...`, `go vet ./...`, `go mod verify`, build Docker pós-scheduler e smoke HTTP da imagem (`ready=200`, migrations 1–9) verdes. A branch foi publicada e permanece sem merge em `develop`.
