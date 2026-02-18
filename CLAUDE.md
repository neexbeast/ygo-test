# Destination Data Aggregation API — Claude Code Configuration

## Project Overview

A Go backend service that aggregates real-time travel destination data from multiple
public APIs, stores it in PostgreSQL (JSONB), caches it in Redis, and exposes it via
a bearer-token-protected REST API.

## Project Structure

```
/
├── cmd/server/           # Application entrypoint (main.go)
├── internal/
│   ├── api/              # HTTP handlers, middleware, router setup
│   ├── destination/      # Core business logic (aggregation, fetching)
│   ├── storage/          # PostgreSQL repository
│   └── cache/            # Redis caching layer
├── migrations/           # SQL migration files (001_initial.sql, etc.)
├── ai-logs/              # Claude Code conversation logs (MANDATORY)
├── docker-compose.yml    # PostgreSQL + Redis + server
├── Dockerfile            # Multi-stage build
├── go.mod
└── README.md
```

## Tech Stack

- **Router**: Chi — `github.com/go-chi/chi/v5`
- **Database**: PostgreSQL via `github.com/jackc/pgx/v5/pgxpool`
- **Cache**: Redis via `github.com/redis/go-redis/v9`
- **Logging**: `log/slog` (standard library, structured)
- **Concurrency**: `golang.org/x/sync/errgroup` for parallel API fetching
- **Rate limiting**: `github.com/go-chi/httprate` — 60 req/min per IP
- **Testing**: standard `testing` + `github.com/stretchr/testify`
- **Config**: environment variables only

## Code Style

- **Error handling**: `fmt.Errorf("context doing X: %w", err)` — wrap with context, always check
- **Imports**: stdlib → external → internal (blank line between each group)
- **No GORM** — use raw SQL with `pgxpool` directly
- **No Gin** — use Chi router only
- **Use `any`** instead of `interface{}`
- **Defensive programming**:
  - Always check nil before dereferencing pointers
  - Always check slice/array bounds before indexing
  - Always use `defer recover()` inside goroutines
  - Always pass and check `context.Context`
- **No `fmt.Sprintf`** for simple strings — use concatenation or `strconv`
- **No new string types** — keep string fields as plain `string`
- **JSON tags**: only on structs that marshal/unmarshal external API responses or HTTP responses.
  Internal structs: no json tags.

## AI Logs (NON-NEGOTIABLE — MANDATORY)

After EVERY prompt you send and every response Claude returns, append to `ai-logs/conversation.md`:

```markdown
## Prompt [N] — [HH:MM]

[Exact prompt text]

## Response [N]

[Brief summary of what was done / files changed / decisions made]

---
```

No logs = submission disqualified. Start logging from prompt #1.

## External APIs (Parallel Fetching Required)

Use `errgroup` from `golang.org/x/sync/errgroup` to fetch at least 2 sources concurrently.

| API | Data | Auth |
|-----|------|------|
| OpenWeatherMap | Weather, temperature, humidity, conditions | `OPENWEATHER_API_KEY` |
| OpenTripMap | Points of interest, top attractions | `OPENTRIPMAP_API_KEY` |
| RestCountries | Currency, language, timezone, region | No key needed |
| Teleport | Urban quality scores, cost of living scores | No key needed |

Aggregate all results into a single unified `DestinationData` response.

## REST API Endpoints

```
GET  /api/v1/destinations/:city          — Return cached/stored destination data
POST /api/v1/destinations/:city/refresh  — Fetch fresh data, store + cache it
GET  /api/v1/health                      — Health check (DB + Redis connectivity)
```

- All endpoints except `/api/v1/health` require `Authorization: Bearer <token>`
- Missing/invalid token → `401 Unauthorized`
- Token value set via `BEARER_TOKEN` env variable

## Authentication Middleware

Simple bearer token middleware using `crypto/subtle.ConstantTimeCompare`:
```go
func BearerAuth(token string) func(http.Handler) http.Handler
```
Applied to all routes except `/api/v1/health`.

## PostgreSQL + JSONB

Schema design:
- `destinations` table with structured columns (`id`, `city`, `country`, `fetched_at`, `created_at`)
- `data JSONB` column for all variable/flexible destination data (weather, POI, quality scores)
- JSONB operators in use: `?` (key existence) and `@>` (containment)
- GIN index on `data` column for fast JSONB queries
- Migrations in `migrations/` — numbered, idempotent (`CREATE TABLE IF NOT EXISTS`)

## Redis Caching

- Cache-aside pattern: Redis hit → return; miss → check Postgres → miss → fetch external
- TTL: 1 hour per destination
- Key format: `destination:{city_lowercased}`
- Invalidate on `POST .../refresh`

## Testing Requirements

- **Minimum 80% test coverage** (enforced — evaluators will run `go test -coverprofile`)
- Unit tests for `internal/destination/` and `internal/storage/`
- Integration tests or mocks for `internal/api/` handlers
- Never use `t.Skip()`
- Cover: happy path, error cases, nil inputs, empty data

```bash
go test -coverprofile=coverage.out ./internal/...
go tool cover -func=coverage.out
```

## Environment Variables

```
BEARER_TOKEN=your-secret-token
DATABASE_URL=postgres://postgres:postgres@localhost:5432/destinations?sslmode=disable
REDIS_URL=redis://localhost:6379
OPENWEATHER_API_KEY=xxx
OPENTRIPMAP_API_KEY=xxx
PORT=8080
```

## Key Rules (Non-Negotiable)

- NEVER use GORM or Gin
- ALWAYS update `ai-logs/conversation.md` after every interaction
- ALWAYS use `errgroup` for parallel API calls — this is evaluated
- ALWAYS include at least one JSONB operator query — this is evaluated
- ALWAYS use `defer recover()` in goroutines
- NEVER mention Claude/AI in commits or PRs
