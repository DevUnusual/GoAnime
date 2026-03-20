# Plano de Melhoria do Sistema de Logging — GoAnime

Data: 2026-03-20
Branch base: `dev` (commit ef7d8d3)
Referência: `docs/problemaDoLogging.md`, NotebookLM (33 fontes sobre logging/observability)

---

## Visão Geral

O GoAnime possui um logger centralizado (`internal/util/logger.go`) baseado em
`charmbracelet/log`, com suporte a console colorido e arquivo de debug. Porém,
múltiplos arquivos ignoram o logger e usam `log.Fatal`, `log.Printf` e
`fmt.Println` diretamente, causando inconsistência, perda de logs e crashes
inesperados.

### Decisão Arquitetural: Manter charmbracelet/log

O NotebookLM recomenda uma **abordagem híbrida**: manter `charmbracelet/log`
como handler visual para a CLI (experiência humana no terminal) e usar a
interface `log/slog` internamente para estruturação. No entanto, como o GoAnime é
uma CLI interativa focada em UX de terminal (menus, prompts, TUI), a migração
para `slog` traria complexidade desnecessária neste momento.

**Decisão:** Manter `charmbracelet/log` como base e aplicar melhorias
incrementais de qualidade. Considerar `slog` como handler no futuro se houver
necessidade de saída JSON (CI/CD, automação).

---

## Fases de Implementação

### Fase 0 — Pré-requisitos
> **Dependência: PR #129** (estado atual: OPEN, não merged)

- [ ] Fazer merge da PR #129 (`bugfix/replace-fatal-with-error-returns`) em `dev`
- [ ] Confirmar que os 6 `log.Fatalln` e 8 `util.Fatal` foram removidos/substituídos

**Após o merge, verificar:**
```bash
grep -rn "log\.Fatal\|log\.Fatalln" internal/ --include="*.go" | grep -v "_test.go"
grep -rn "util\.Fatal(" internal/ --include="*.go" | grep -v "_test.go"
```

---

### Fase 1 — Deprecar `util.Fatal` no Logger (P0)
> **Branch:** `refactor/deprecate-util-fatal`
> **Arquivos:** `internal/util/logger.go`
> **Estimativa:** Pequena

**Objetivo:** O logger não deveria ter o poder de matar o processo. Isso é
responsabilidade do caller.

- [ ] Adicionar `// Deprecated: use Error/Errorf + return error instead` ao `util.Fatal`
- [ ] Remover a função `util.Fatal` do logger (se a PR #129 já eliminou todos os callers)
  - Se restar algum caller, deprecar com aviso no godoc e remover na próxima versão
- [ ] Remover o case `log.FatalLevel` do `writeToFile()` (passa a ser tratado como Error)
- [ ] Garantir que nenhum import de `log` (stdlib) existe nos arquivos que usam `util.*`

**Teste:** `grep -rn "util\.Fatal(" internal/ --include="*.go"` deve retornar 0 resultados

---

### Fase 2 — Migrar `log.Printf/Println` para Logger Centralizado (P1)
> **Branch:** `refactor/migrate-stdlib-log-to-util`
> **Arquivos:** 3 arquivos, ~25 chamadas
> **Estimativa:** Média

Cada chamada deve ser analisada para determinar o nível correto:
- Erros de operação → `util.Errorf()`
- Avisos/condições inesperadas → `util.Warnf()`
- Status informativo → `util.Infof()`

| Arquivo | Chamadas | Ação |
|---|---|---|
| `internal/playback/series.go` | 14 | Migrar para `util.Warnf`/`util.Errorf` conforme contexto |
| `internal/playback/movie.go` | 10 | Migrar para `util.Warnf`/`util.Errorf` conforme contexto |
| `internal/tracking/local.go` | 1 | Migrar para `util.Errorf` |

- [ ] Migrar cada chamada individualmente (revisar nível de severidade caso a caso)
- [ ] Remover `import "log"` dos 3 arquivos após migração
- [ ] Rodar `go vet ./...` e `staticcheck ./...` para validar

---

### Fase 3 — Migrar `fmt.Print*` usados como logging (P1)
> **Branch:** `refactor/migrate-fmt-print-logging`
> **Arquivos:** 3 arquivos, ~12 chamadas
> **Estimativa:** Pequena

**Cuidado:** NÃO migrar `fmt.Print*` que são saída de UI legítima (menus,
prompts, mensagens para o usuário). Migrar **apenas** as que representam logging
de erros/avisos disfarçado.

| Arquivo | Chamadas | Ação |
|---|---|---|
| `internal/player/player.go:403` | 1 | `fmt.Println("error closing mpv socket")` → `util.Errorf(...)` |
| `internal/player/playvideo.go` | 4 | `fmt.Printf("Error ...")` → `util.Errorf(...)` |
| `internal/downloader/downloader.go` | 7 | Erros → `util.Errorf`, HEAD warnings → `util.Warnf` |

- [ ] Migrar as 12 chamadas identificadas no `problemaDoLogging.md`
- [ ] Manter `fmt.Print*` de UI intacto
- [ ] Rodar `go vet ./...` e `staticcheck ./...`

---

### Fase 4 — Rotação de Logs (P2)
> **Branch:** `feature/log-rotation`
> **Arquivos:** `internal/util/logger.go`
> **Estimativa:** Pequena

O diretório de logs cresce indefinidamente. Implementar limpeza automática.

- [ ] No `initFileLogger()`, antes de criar o novo arquivo, limpar logs antigos
- [ ] Política: remover arquivos `goanime_*.log` com mais de 7 dias
- [ ] Manter no máximo 20 arquivos de log (limpar os mais antigos se exceder)
- [ ] Usar `os.ReadDir` + `os.Stat` para checar idade dos arquivos
- [ ] Logar "Cleaned N old log files" em debug quando houver limpeza

**Implementação sugerida:**
```go
func cleanOldLogs(logDir string, maxAge time.Duration, maxFiles int) {
    entries, err := os.ReadDir(logDir)
    if err != nil {
        return
    }
    // Filtrar apenas goanime_*.log
    // Ordenar por ModTime
    // Remover os que excedem maxAge ou maxFiles
}
```

---

### Fase 5 — Logging Estruturado com Key-Value Pairs (P2)
> **Branch:** `feature/structured-logging`
> **Arquivos:** `internal/util/logger.go` + callers
> **Estimativa:** Média-Grande

**Nota:** O `charmbracelet/log` já suporta key-value pairs nativamente via
`Logger.Info("msg", "key", value)`. A infraestrutura já existe — falta adotar
nos callers.

- [ ] Identificar os logs mais importantes (erros de rede, falhas de playback,
      tracking) e adicionar contexto estruturado:
  ```go
  // Antes
  util.Errorf("Failed to download video: %v", err)
  // Depois
  util.Error("Failed to download video", "err", err, "url", videoURL, "episode", epNum)
  ```
- [ ] Substituir prefixos `[TRACE]` e `[PERF]` em `Debugf` por key-value:
  ```go
  // Antes
  util.Debugf("[PERF] Operation took %v", duration)
  // Depois
  util.Debug("Operation completed", "type", "perf", "duration", duration)
  ```
- [ ] Padronizar idioma: **inglês** para logs internos, **PT-BR** para mensagens de UI

---

### Fase 6 — Eliminar Import `log` (stdlib) Residual (P2)
> **Branch:** Pode ser incluído em qualquer fase anterior
> **Estimativa:** Trivial

- [ ] Buscar todos os imports de `"log"` em arquivos que usam `util.*`
- [ ] Remover imports desnecessários
- [ ] Adicionar regra ao linter (`golangci-lint`) para proibir import de `"log"`
      em `internal/` (opcional, via `depguard`)

```bash
grep -rn '"log"' internal/ --include="*.go" | grep -v "_test.go" | grep -v "charm.land/log"
```

---

## Ordem de Execução Recomendada

```
Fase 0 (merge PR #129)
  ↓
Fase 1 (deprecar util.Fatal)
  ↓
Fase 2 + Fase 3 (podem ser paralelas — migrar log.Printf e fmt.Print)
  ↓
Fase 4 (rotação de logs — independente)
  ↓
Fase 5 (structured logging — depende das fases 2+3 estarem merged)
  ↓
Fase 6 (cleanup — pode ser incluída em qualquer fase)
```

---

## Critérios de Sucesso

Ao final de todas as fases:

1. **Zero `log.Fatal`/`log.Fatalln`** em `internal/` (exceto testes)
2. **Zero `util.Fatal`** no codebase — função removida do logger
3. **Zero `log.Printf`/`log.Println`** em `internal/` (exceto testes)
4. **Zero `fmt.Print*` usado como logging** — apenas UI legítima mantém `fmt`
5. **Rotação automática** de logs com mais de 7 dias
6. **Logs críticos** (erros de rede, playback, tracking) possuem contexto
   estruturado (key-value)
7. **Todos os checks passam:** `go vet`, `staticcheck`, `gosec`, `golangci-lint`

---

## Consideração Futura: Migração para `log/slog`

Se no futuro houver necessidade de:
- Saída JSON para pipelines de CI/CD
- Integração com ferramentas de observabilidade (Grafana Loki, OpenTelemetry)
- Múltiplos handlers simultâneos (console + arquivo + remote)

O `charmbracelet/log` pode atuar como **handler do `log/slog`**, permitindo
migração gradual sem quebrar a UX do terminal. Documentar essa possibilidade
mas não implementar agora — evitar over-engineering.

---

## Referências

- Logger centralizado: `internal/util/logger.go`
- Análise de problemas: `docs/problemaDoLogging.md`
- PR #129: `bugfix/replace-fatal-with-error-returns`
- Development guide: `docs/Development.md`
- NotebookLM: 33 fontes sobre logging best practices, structured logging, log rotation
