# Bookstore API

A small Go REST API for learning CRUD, PostgreSQL, JWT authentication, and
observability with Prometheus, Grafana, Loki, and Tempo. This stack is intended
for local education, not production deployment.

## Learning steps

1. **REST and PostgreSQL:** CRUD routes store book records in PostgreSQL.
2. **Authentication and authorization:** login returns a signed JWT; reader
   tokens can read, and admin tokens can create, update, and delete.
3. **Metrics:** Prometheus scrapes request counts, status codes, and durations;
   Grafana provisions a starter dashboard.
4. **Logs and traces:** JSON application logs go to stdout and are collected by
   Alloy into Loki; OpenTelemetry HTTP traces are sent through Alloy to Tempo.
   Grafana has all three data sources preconfigured.

## Start the stack

Install Docker Desktop and start its Linux container engine. In this directory,
optionally copy `.env.example` to `.env` and adjust the local development
credentials. Then run:

```powershell
docker compose up --build
```

You can also run `docker compose up --build` from the workspace root; its
top-level `compose.yaml` includes this project's Compose file.

Services:

| Service | Address | Default local login |
| --- | --- | --- |
| Bookstore API | `http://localhost:8080` | JWT login endpoint below |
| Grafana | `http://localhost:3000` | `admin` / `admin` |
| Prometheus | `http://localhost:9090` | none |
| Loki | `http://localhost:3100` | none |
| PostgreSQL | `localhost:5432` | `bookstore` / `bookstore_dev` |

Grafana opens with Prometheus, Loki, and Tempo data sources and a **Bookstore
API** dashboard already provisioned. The defaults are intentionally simple;
change them in `.env` and never use them outside a local learning environment.
The API also has default demo accounts `admin` / `admin_dev` and `reader` /
`reader_dev`.

Stop the containers with `Ctrl+C`, or run `docker compose down`. To erase stored
database, metrics, Grafana, logs, traces, and Alloy data too, run
`docker compose down -v`.

## Try the REST API

Login as the demo admin and keep the token for subsequent requests:

```powershell
$login = Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/v1/auth/login `
  -ContentType 'application/json' `
  -Body '{"username":"admin","password":"admin_dev"}'
$headers = @{ Authorization = "Bearer $($login.access_token)" }
```

Create, list, and read a book:

```powershell
Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/v1/books `
  -Headers $headers -ContentType 'application/json' `
  -Body '{"title":"The Go Programming Language","author":"Alan A. A. Donovan","publication_year":2015}'

Invoke-RestMethod -Uri http://localhost:8080/api/v1/books -Headers $headers
Invoke-RestMethod -Uri http://localhost:8080/api/v1/books/1 -Headers $headers
```

Update and delete a book:

```powershell
Invoke-RestMethod -Method Put -Uri http://localhost:8080/api/v1/books/1 `
  -Headers $headers -ContentType 'application/json' `
  -Body '{"title":"The Go Programming Language, 2nd edition","author":"Alan A. A. Donovan","publication_year":2015}'

Invoke-RestMethod -Method Delete -Uri http://localhost:8080/api/v1/books/1 -Headers $headers
```

`GET /healthz` checks database connectivity. `GET /metrics` is the Prometheus
scrape endpoint. CRUD routes are under `/api/v1/books`; they return JSON and use
standard HTTP status codes (`201` created, `404` not found, and `204` deleted).
Missing or invalid tokens receive `401`; an authenticated reader attempting a
write receives `403`.

## REST concepts in this example

The URL identifies a resource (`/api/v1/books/1` is book 1), and the HTTP
method describes what to do with it:

| Method | Path | Action |
| --- | --- | --- |
| `GET` | `/api/v1/books` | Read the collection |
| `POST` | `/api/v1/books` | Create a book from the JSON body |
| `GET` | `/api/v1/books/{id}` | Read one book |
| `PUT` | `/api/v1/books/{id}` | Replace a book's editable fields |
| `DELETE` | `/api/v1/books/{id}` | Delete one book |

JSON is the representation exchanged between client and server. Each request
carries the information needed for that operation rather than relying on a
previous request's state. PostgreSQL stores records across API restarts.

## Explore observability

- **Metrics and dashboard:** open Grafana at `http://localhost:3000`, then view
  the provisioned **Bookstore API** dashboard. Prometheus is also available
  directly at `http://localhost:9090`.
- **Logs:** in Grafana, open **Explore**, choose Loki, and search the API
  container logs. Application messages are structured JSON and include the
  trace ID when a request has a trace.
- **Traces:** in Grafana, open **Explore**, choose Tempo, and search for the
  `bookstore-api` service. HTTP server spans are instrumented with OpenTelemetry.
- **PromQL example:** `sum(rate(http_server_requests_total[5m]))`

All container logs are collected from Docker by Grafana Alloy. The API exports
traces over OTLP to Alloy, which forwards them to Tempo. For local `go run`
without the Compose stack, tracing is disabled unless
`OTEL_EXPORTER_OTLP_ENDPOINT` is set.

## Run tests locally

With Go installed, run:

```powershell
go test ./...
go vet ./...
```

To run the API directly, start PostgreSQL with `docker compose up db -d`, set
the same database URL and demo credentials in PowerShell, then run `go run .`:

```powershell
$env:DATABASE_URL = 'postgres://bookstore:bookstore_dev@localhost:5432/bookstore?sslmode=disable'
$env:ADMIN_USERNAME = 'admin'
$env:ADMIN_PASSWORD = 'admin_dev'
$env:READER_USERNAME = 'reader'
$env:READER_PASSWORD = 'reader_dev'
$env:JWT_SECRET = 'local-bookstore-jwt-secret-change-this-before-use'
go run .
```
