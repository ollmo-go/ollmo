<div align="center">

# ollmo

**Open LLM Orchestrator**

Your data never leaves — build your own AI productivity tools from your knowledge base.

A production-grade RAG platform with hybrid retrieval, multi-tenancy, and streaming chat.
Built with Go + Fiber (backend) and Next.js App Router + shadcn/ui (frontend).

[Live Demo](https://demo.ollmo.com/) · [Quick Start](#quick-start) · [Documentation](#documentation) · [Roadmap](#roadmap) · [Contributing](#contributing)

</div>

<div align="center">

English | [简体中文](README.zh-CN.md)

</div>

---

![ollmo agent](agent-canvas.png)

![ollmo chat](chat.png)

## Why ollmo

Most AI application platforms bury you in workflows, nodes, plugins — dozens of concepts and hundreds of settings before the AI can actually help. AI should make things simpler, not the other way around.

ollmo converges everything into three steps: **upload documents, ask questions, get answers with sources**. One-command deployment, up and running in ten minutes.

- **Simple** — Does one thing and does it well: turn your documents into an assistant you can talk to.
- **Easy** — Up in ten minutes, no need to learn a system more complex than your business.
- **Restrained** — No concept stacking, no redundant configuration. Complexity stays inside; simplicity stays with you.

## Features

- **Hybrid Retrieval** — Dense vectors on Milvus + lexical matching via MySQL FULLTEXT, fused with RRF, with KB-level rerank configuration.
- **Multi-Tenancy** — Logical isolation across MySQL, Milvus, and MinIO; every table carries `tenant_id`.
- **Document Pipeline** — Upload (drag & drop, paste, multi-file) → parse (MinerU / local parser) → chunk → embed → index, fully async via Asynq + Redis.
- **Streaming Chat** — Real-time SSE streaming with source citations. Doubao-style greeting from agent opening message.
- **Auto-Memory** — Long conversations are automatically summarized in the background (async LLM task); summaries are injected into later sessions for cross-conversation continuity. Toggleable site-wide.
- **Agent Canvas** — Visual agent pipeline (React Flow): intent classification → retrieval → condition branch → LLM → direct reply. Per-KB system prompt, temperature, top_k, rerank toggle, opening message, and LLM selection.
- **Ingestion Pipeline Canvas** — Visual ingestion DAG (React Flow): source → parser → chunker → embedder → sink, with configurable parsing options, chunking strategy, and batch size.
- **GraphRAG** — LLM-powered entity & relation extraction at ingestion; entity matching enriches retrieval context at query time.
- **Configurable Models** — Tenant-level LLM / Embedding / Rerank provider management via web UI, with per-KB overrides. Selection priority: KB config → tenant default → config.yaml.
- **Team & Access Control** — Invitation-based membership with owner / admin / member roles, per-user daily message quotas, KB-level membership (viewer / editor / owner).
- **Admin Console** — Super-admin user management, tenant management with plan & quota control (free / pro / enterprise), system settings (registration toggle, auto-memory), analytics dashboard, and audit logging.
- **External API** — API-key authenticated programmatic access (search, KB management, streaming chat).
- **Internationalization** — Chinese / English UI with server-side message contracts. No locale in URL paths.

## Tech Stack

| Layer | Choice |
|---|---|
| Backend | Go 1.26+ / Fiber / GORM |
| Frontend | Next.js 15 (App Router) / React / shadcn-ui |
| Database | MySQL 8.0 (utf8mb4) — metadata |
| Vector DB | Milvus 2.5+ — dense vector retrieval |
| Object Storage | MinIO (S3 compatible) — original docs, parsed output, thumbnails |
| Task Queue | Asynq + Redis — parsing, embedding, index rebuild, auto-summarization |
| Document Parser | MinerU (containerized, GPU optional) + local fallback parser |
| Auth | Local account (email + password + JWT) |

## Architecture

```
                    ┌─────────────────────────────────────────────────────┐
                    │                   Next.js Web (3001)                 │
                    └────────────────────────┬────────────────────────────┘
                                             │ HTTP / SSE
                    ┌────────────────────────▼────────────────────────────┐
                    │                Go API Server (8080)                 │
                    │  Fiber · JWT auth · Tenant isolation · RESTful API  │
                    └───────┬────────────┬──────────────┬─────────────────┘
                            │            │              │
                   ┌────────▼──┐  ┌──────▼──────┐  ┌───▼────────────┐
                   │  MySQL    │  │   Redis     │  │    MinIO       │
                   │ metadata  │  │ task queue  │  │  file storage  │
                   └───────────┘  └──────┬──────┘  └────────────────┘
                                         │
                                ┌────────▼────────┐
                                │  Asynq Worker   │
                                │ parse · embed · │
                                │   summarize     │
                                └────────┬────────┘
                                         │
                                ┌────────▼────────┐
                                │     Milvus      │
                                │ dense (vectors) │
                                └─────────────────┘
```

## Project Layout

```
ollmo/
├── server/                 # Go backend (API + worker share the same binary)
│   ├── main.go             # Entry point: `go run . api|worker|migrate`
│   ├── internal/           # Business domains (one package per domain)
│   │   ├── agent/          # Agent canvas config & execution
│   │   ├── analytics/      # Usage statistics & activity feed
│   │   ├── apikey/         # API key management & middleware
│   │   ├── audit/          # Audit logging
│   │   ├── auth/           # Authentication (email + password + JWT)
│   │   ├── chat/           # Streaming chat with citations
│   │   ├── doc/            # Document lifecycle, chunking, worker
│   │   ├── embedding/      # Embedding model management
│   │   ├── graph/          # GraphRAG (entity extraction & retrieval)
│   │   ├── invitation/     # Team invitation system
│   │   ├── kb/             # Knowledge base CRUD
│   │   ├── llm/            # LLM model management
│   │   ├── memory/         # Auto-summarization & cross-session context
│   │   ├── pipeline/       # Ingestion pipeline canvas config
│   │   ├── rerank/         # Rerank model management
│   │   ├── search/         # Hybrid retrieval (dense + lexical + rerank)
│   │   ├── site/           # Site-wide settings
│   │   ├── team/           # KB membership & access control
│   │   ├── tenant/         # Tenant, plan & quota management
│   │   ├── user/           # User management
│   │   └── ...
│   ├── pkg/                # Shared infrastructure (db, vector, clients)
│   └── config.yaml
├── web/                    # Next.js frontend
│   ├── app/                # App Router pages ('/' chat, '/dashboard/*' admin)
│   ├── components/         # UI + feature components (agent, pipeline, etc.)
│   ├── lib/                # API client, i18n, utils
│   └── messages/           # i18n message files (zh, en)
├── deploy/                 # Dockerfiles
├── docker-compose.yml      # Full-stack local deployment
└── Makefile                # Dev & build targets
```

## Quick Start

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) + Docker Compose
- [Go](https://go.dev/dl/) 1.26+
- [Node.js](https://nodejs.org/) 20+
- [Make](https://www.gnu.org/software/make/) (optional, for convenience targets)

### 1. Clone & configure

```bash
git clone https://github.com/ollmo-go/ollmo.git
cd ollmo
cp .env.example .env
```

### 2. Start infrastructure

```bash
make up
```

This starts MySQL, Redis, MinIO, Milvus, and the Asynqmon dashboard.

| Service | URL | Credentials |
|---------|-----|-------------|
| MySQL | `localhost:3306` | `ollmo` / `ollmo_dev_pwd` |
| MinIO Console | http://localhost:9001 | `minioadmin` / `minioadmin` |
| Milvus | `localhost:19530` | — |
| Asynqmon UI | http://localhost:8081 | — |

### 3. Run database migrations

```bash
make migrate
```

### 4. Start backend (API + worker)

```bash
# Terminal 1: API server
make server

# Terminal 2: Asynq worker (parsing, embedding, auto-summarization)
make worker
```

### 5. Start frontend

```bash
make web          # http://localhost:3001
```

### Full stack via Docker

```bash
make up-full      # Builds & starts everything (infra + server + worker + web)
```

## Configuration

| Source | Purpose |
|--------|---------|
| `server/config.yaml` | Backend defaults (DB, Redis, MinIO, Milvus, MinerU, JWT, quotas) |
| `.env` | Environment overrides for Docker deployment |
| Web UI → Settings | Per-tenant LLM / Embedding / Rerank models (with default selection) |
| Web UI → KB Settings | Per-KB embedding / rerank model override |
| Web UI → Agent Canvas | Per-KB agent config (system prompt, LLM, retrieval params, opening message) |
| Web UI → Pipeline Canvas | Per-KB ingestion config (parser, chunker, batch size) |
| Web UI → System Settings | Site-wide settings (registration toggle, auto-memory) — super admin only |

Model selection priority: KB-level config → tenant default → config.yaml fallback.

## Development

- All development happens on the `dev` branch. `main` is release-only.
- Modules are organized by **business domain**, not technical layer. Each domain package contains its own `model.go` / `repo.go` / `service.go` / `handler.go`.
- Comments are kept in English and minimal — only where the intent is non-obvious.

```bash
make build         # Build server binary + web
make build-docker  # Build Docker images
make clean         # Remove build artifacts
```

## Documentation

- [Live Demo](https://demo.ollmo.com/) — Try it before you deploy.

## Roadmap

- [x] **Phase 1 — MVP**: KB/document CRUD, document parsing, embedding, hybrid retrieval, streaming chat with citation, local auth + tenant isolation.
- [x] **Phase 2 — Configurability**: Multi LLM/Embedding/Rerank providers, KB editing with re-indexing, rerank, multi chunking strategies, drag & drop upload, i18n.
- [x] **Phase 3 — Intelligence**: Ingestion Pipeline canvas (React Flow), GraphRAG, Agent canvas, Memory, role-based access control, external API, team management.
- [x] **Phase 4 — Production Readiness**: Analytics dashboard, audit logging, conversation management (search / rename / export / pin), auto-memory summarization, super-admin console (users, tenants, plans & quotas, system settings), registration toggle.
- [ ] **Phase 5 — Document Intelligence**:

  ### 5.1 Document Preview & Citation Tracing
  - In-chat citation clicks jump to the source document with the cited passage highlighted
  - Document viewer: PDF render, markdown/text preview, page navigation
  - Chunk-to-source mapping (chunk → document → page/offset)
  - Side-by-side view: chat + source document

  ### 5.2 Advanced Document Parsing
  - Table extraction: preserve table structure as HTML for structured retrieval
  - Image extraction: store images in MinIO, embed captions, support vision LLM Q&A
  - Layout-aware parsing: recognize headers, footers, columns, footnotes
  - OCR improvement: scanned document support with language detection

  ### 5.3 External API Expansion
  - Full CRUD via API key: upload documents, create/manage KBs, list conversations
  - OpenAPI/Swagger documentation at `/api/v1/docs`
  - Rate limiting per API key (requests/min, tokens/day)
  - Webhook callbacks for document processing events (parsed, embedded, failed)

## Contributing

1. Fork the repository and create your branch from `dev`.
2. Ensure `make build` passes before submitting a PR.
3. Keep changes focused — one feature or fix per PR.
4. Follow existing code style: English comments, domain-driven package structure.

## License

Apache License 2.0 with additional conditions. See [LICENSE](LICENSE) for details.

In short:
- ✅ Free for personal use, internal enterprise use, and commercial application development.
- ❌ Operating a multi-tenant SaaS service similar to ollmo requires a commercial license.
- ❌ Removing or modifying the ollmo brand, LOGO, or copyright notices in the console is prohibited.
