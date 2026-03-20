# Análise do Logging — GoAnime

Data da análise: 2026-03-20
Branch analisada: `dev` (commit ef7d8d3)

---

## Contexto

A aplicação possui um logger centralizado em `internal/util/logger.go` baseado no
`charm.land/log/v2`, com suporte a:
- Log em arquivo (debug mode) com sessão isolada por execução
- Console colorido via lipgloss
- Níveis: Debug, Info, Warn, Error, Fatal
- Funções: `util.Debug`, `util.Info`, `util.Warn`, `util.Error`, `util.Fatal` e variantes `f`

Apesar disso, diversas partes do código ignoram o logger centralizado e usam
`log.Fatalln`, `log.Printf`, `fmt.Println` diretamente, causando inconsistência,
perda de logs em arquivo e crashes desnecessários.

---

## Problemas encontrados

### 1. Chamadas `log.Fatal/Fatalln` (stdlib) — crasham o app

> **NOTA:** Os 4 casos em `appflow/anime_data.go` e os 2 em `playback/common.go`
> já foram corrigidos na **PR #129** (`bugfix/replace-fatal-with-error-returns`).
> Não repetir essa correção.

| Arquivo | Linha | Chamada | Status |
|---|---|---|---|
| `appflow/anime_data.go` | 25 | `log.Fatalln("Failed to search...")` | **Corrigido na PR #129** |
| `appflow/anime_data.go` | 39 | `log.Fatalln("Failed to search...")` | **Corrigido na PR #129** |
| `appflow/anime_data.go` | 203 | `log.Fatalln("...does not have episodes...")` | **Corrigido na PR #129** |
| `appflow/anime_data.go` | 227 | `log.Fatalln("...does not have episodes...")` | **Corrigido na PR #129** |
| `playback/common.go` | 140 | `log.Fatalln(util.ErrorHandler(err))` | **Corrigido na PR #129** |
| `playback/common.go` | 144 | `log.Fatalln("Error converting...")` | **Corrigido na PR #129** |

### 2. `util.Fatal` — 8 chamadas restantes (todas em `player/player.go`)

A função `util.Fatal` chama `Logger.Fatal()` → `os.Exit(1)`. Todas as 8 chamadas
estão dentro de `HandleDownloadAndPlay()` que **já retorna `error`** — são 100%
substituíveis por retorno de erro.

| Linha | Chamada | Contexto |
|---|---|---|
| 652 | `util.Fatal("Failed to get current user:", err)` | Caminho principal |
| 706 | `util.Fatal("Failed to create download directory:", err)` | Caminho principal |
| 747 | `util.Fatal("Failed to download video:", dlErr)` | Dentro de goroutine |
| 762 | `util.Fatal("Error running progress bar:", err)` | Caminho principal |
| 820 | `util.Fatal("Failed to download video:", dlErr)` | Dentro de goroutine |
| 836 | `util.Fatal("Error running progress bar:", err)` | Caminho principal |
| 889 | `util.Fatal("Failed to download video:", err)` | Dentro de goroutine |
| 902 | `util.Fatal("Error running progress bar:", err)` | Caminho principal |

> **NOTA:** Os 3 casos em goroutines (747, 820, 889) também foram corrigidos na
> **PR #129**, que propaga o erro via campo `m.err` no model. Os 5 restantes no
> caminho principal (652, 706, 762, 836, 902) também foram corrigidos na PR #129.
> Verificar se o merge da PR cobre tudo antes de agir.

### 3. `log.Printf/Println` (stdlib) — 25 chamadas em 4 arquivos

Bypass total do logger centralizado. Não escrevem no arquivo de log, sem
formatação consistente, sem nível de severidade.

| Arquivo | Qtd | Exemplos |
|---|---|---|
| `playback/series.go` | 14 | Erros de navegação, status de quit, troca de anime |
| `playback/movie.go` | 10 | Erros de playback, troca de anime, replay |
| `tracking/local.go` | 1 | Erro fechando rows do DB |

### 4. `fmt.Print*` como logging disfarçado — 12 misuses em 3 arquivos

Mensagens de erro impressas diretamente no stdout/stderr sem passar pelo logger.

| Arquivo | Linha | Mensagem | Deveria ser |
|---|---|---|---|
| `player/player.go` | 403 | `fmt.Println("error closing mpv socket")` | `util.Errorf(...)` |
| `player/playvideo.go` | 1207 | `fmt.Printf("Error getting audio tracks: %v")` | `util.Errorf(...)` |
| `player/playvideo.go` | 1286 | `fmt.Printf("Error setting audio track: %v")` | `util.Errorf(...)` |
| `player/playvideo.go` | 1296 | `fmt.Printf("Error getting subtitle tracks: %v")` | `util.Errorf(...)` |
| `player/playvideo.go` | 1361 | `fmt.Printf("Error setting subtitle track: %v")` | `util.Errorf(...)` |
| `downloader/downloader.go` | 312 | `fmt.Printf("Some downloads failed:")` | `util.Errorf(...)` |
| `downloader/downloader.go` | 314 | `fmt.Printf("  - %v\n", err)` | `util.Errorf(...)` |
| `downloader/downloader.go` | 393 | `fmt.Printf("HEAD request failed...")` | `util.Warnf(...)` |
| `downloader/downloader.go` | 397 | `fmt.Printf("HEAD request failed...")` | `util.Warnf(...)` |
| `downloader/downloader.go` | 404 | `fmt.Printf("Warning: Failed to get content length...")` | `util.Warnf(...)` |
| `downloader/downloader.go` | 438 | `fmt.Printf("HTTP download failed...")` | `util.Errorf(...)` |

> **NOTA:** Os `fmt.Print*` restantes (~137 ocorrências) em `player/download.go`,
> `downloader/downloader.go`, `playback/series.go` etc. são **saída de UI legítima**
> (menus, prompts, mensagens de progresso para o usuário) e não devem ser migrados
> para o logger.

### 5. Problemas estruturais do logger

| Problema | Descrição |
|---|---|
| `util.Fatal` existe no logger | Um logger não deveria matar o processo. Deveria ser removido ou depreciado. |
| Sem contexto estruturado | Mensagens são strings planas. Faltam campos como `anime`, `episode`, `source`. |
| Idioma inconsistente | Mistura inglês e português nas mensagens de log. |
| Prefixos hardcoded | `[TRACE]` e `[PERF]` são strings em `Debugf` em vez de campos ou níveis dedicados. |
| Sem rotação de logs | Arquivos em `GetLogDir()` acumulam infinitamente sem limpeza automática. |

---

## Melhorias propostas (por prioridade)

### P0 — Críticas (evitar crashes)

- [ ] ~~Remover `log.Fatalln` em `anime_data.go` e `common.go`~~ → **Feito na PR #129**
- [ ] ~~Remover `util.Fatal` em `player.go`~~ → **Feito na PR #129**
- [ ] Deprecar/remover `util.Fatal` do logger (`logger.go:267`) — manter apenas `util.Error`/`util.Errorf`

### P1 — Consistência do logging

- [ ] Migrar 25 chamadas `log.Printf/Println` → `util.Infof`/`util.Warnf`/`util.Errorf`
  - `playback/series.go` (14 chamadas)
  - `playback/movie.go` (10 chamadas)
  - `tracking/local.go` (1 chamada)
- [ ] Migrar 12 chamadas `fmt.Print*` de erros → funções do logger centralizado
  - `player/player.go` (1 chamada)
  - `player/playvideo.go` (4 chamadas)
  - `downloader/downloader.go` (7 chamadas)
- [ ] Padronizar idioma das mensagens de log (sugestão: EN para logs internos, PT-BR para UI)

### P2 — Qualidade e manutenção

- [ ] Adicionar campos estruturados ao logger (key-value pairs em vez de string formatting)
- [ ] Substituir prefixos `[TRACE]`/`[PERF]` por campos dedicados ou sub-loggers
- [ ] Implementar rotação de logs (limpar arquivos com mais de N dias em `GetLogDir()`)
- [ ] Remover import de `log` (stdlib) dos arquivos que usam o logger centralizado

---

## Referências

- Logger centralizado: `internal/util/logger.go`
- PR #129: `bugfix/replace-fatal-with-error-returns` — corrige Fatal → error returns
- Guia de desenvolvimento: `docs/Development.md`
