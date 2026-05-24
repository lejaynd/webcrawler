# 🕸️ WebCrawler

A **highly concurrent, production-grade website-polite web crawler** written in Go - featuring distributed URL and Content deduplication, persistent storage, content-aware parsing, and an Q&A layer via RAG (Retrieval-Augmented Generation).

---

## 📺 Demo

[Watch Demo](./demo.mp4)
[Google Drive Demo](https://drive.google.com/file/d/14bWrgo_7yaAvnqKklew7PIdW4Rq5CU8V/view?usp=sharing)

> Seed a URL → watch it crawl in real time → ask questions about the crawled content, powered by Groq's LLaMA.

---

## 🏗️ Architecture

### System Architecture (V2)

<p align="center">
  <img src="v2_architecture.png" alt="Web Crawler V2 Architecture" width="900">
</p>

### Core Workflow

<p align="center">
  <img src="core_workflow.png" alt="Core Crawl Workflow" width="850">
</p>

---

## ✨ Features

| Feature | Details |
|---|---|
| ⚡ **High Concurrency** | 10 Crawler workers + 10 Parser workers running as goroutines |
| 🌸 **Bloom Filter** | Probabilistic URL deduplication (10M capacity, 0.01% false positive rate) |
| 🗄️ **S3 Storage** | Raw HTML uploaded to AWS S3; S3 keys used as lightweight references in the parser pipeline |
| 🐘 **PostgreSQL** | Crawl metadata (URL, S3 link, depth, timestamp) persisted in a `crawl_records` table |
| 🔴 **Redis** | Content-hash deduplication via `SET NX` — avoids storing duplicate page text |
| 📬 **Dead Letter Queue** | Failed URLs (after 5 retries with exponential backoff) are moved to an in-memory DLQ |
| 🧠 **RAG Q&A** | Crawled text is accumulated in an in-memory context store and answered via Groq (LLaMA 3.1 8B) |
| 🌐 **REST API + Frontend** | Clean JSON API with a built-in HTML/JS frontend served on `:8080` |
| 🔒 **Lease Manager** | Prevents duplicate concurrent crawls of the same URL; auto-recovers expired leases |

---

## 🧩 Component Breakdown

```
cmd/
└── main.go              ← Entry point; wires all components together

internals/
├── scheduler/           ← Central brain: routes URLs, enforces depth/domain limits, bloom filter checks
├── crawlerFrontier/     ← Priority queue of URLs ready to be fetched
├── crawler/             ← Fetches HTML, uploads to S3, inserts SQL record, enqueues to parser
├── parser/              ← Downloads HTML from S3, extracts links & text, deduplicates via Redis
├── parserQueue/         ← Channel connecting crawler → parser workers
├── buffer/              ← Overflow buffer for URLs exceeding depth/domain limits
├── dlq/                 ← Dead Letter Queue for permanently failed tasks
├── lease/               ← In-flight tracking and expired lease recovery
├── llm/                 ← GroqClient (LLaMA via Groq API) + ContextForLLM accumulator
├── db/                  ← S3Store (upload/download/update) + SQLStore (Postgres)
├── api/                 ← HTTP server: /api/crawl, /api/status, /api/ask
├── models/              ← Shared data types (CrawlTask, ParseTask)
└── utils/               ← URL normalization helpers

frontend/
└── index.html           ← Single-page UI (seed URL input → live status → Q&A interface)
```



## ⚙️ Configuration

All configuration is managed via a `.env` file in the project root.

```env
# AWS S3
AWS_ACCESS_KEY_ID=your_key_id
AWS_SECRET_ACCESS_KEY=your_secret_key
AWS_REGION=ap-south-1
S3_BUCKET=your-s3-bucket-name

# PostgreSQL
DATABASE_URL=postgres://postgres:postgres@localhost:5432/webcrawler?sslmode=disable

# Groq API (https://console.groq.com/)
GROQ_API_KEY=your_groq_api_key
```

### Scheduler Defaults (tunable in `scheduler.go`)

| Parameter | Default | Description |
|---|---|---|
| `MaxDepth` | `5` | Max link-following depth from seed URL |
| `MaxPerDomain` | `400` | Max pages crawled per domain |
| `CrawlDelay` | `10s` | Delay before a URL is dispatched to the frontier |

---

## 🚀 Getting Started

### Prerequisites

- **Go 1.21+**
- **Docker & Docker Compose** (for Redis)
- **PostgreSQL** running locally (or via Docker)
- **AWS account** with an S3 bucket
- **Groq API key** — free at [console.groq.com](https://console.groq.com)

### 1. Clone the repository

```bash
git clone https://github.com/lejaynd/webcrawler.git
cd webcrawler
```

### 2. Set up your environment

```bash
cp .env.example .env
# Fill in your AWS credentials, S3 bucket, and Groq API key
```

### 3. Start Redis

A `docker-compose.yml` is included for convenience:

```bash
docker compose up -d redis
```

> **Note:** If you already have PostgreSQL running locally on port 5432, that's all you need. Otherwise, start it with `docker compose up -d postgres`.

### 4. Build and run

```bash
# Build the binary
go build -o webcrawler ./cmd/main.go

# Run the crawler
./webcrawler
```

Or run directly:

```bash
go run ./cmd/main.go
```

The server starts on **http://localhost:8080**.

---

## 🌐 API Reference

### `POST /api/crawl`
Start a crawl session.

```json
{
  "seedURL": "https://example.com",
  "duration": 60
}
```

| Field | Type | Description |
|---|---|---|
| `seedURL` | string | The starting URL for the crawl |
| `duration` | int | Crawl duration in seconds (min: 10) |

**Response:**
```json
{ "status": "crawling" }
```

---

### `GET /api/status`
Check crawl status.

**Response:**
```json
{
  "status": "crawling",
  "pagesCrawled": 47,
  "remaining": 23
}
```

Status values: `idle` → `crawling` → `ready`

---

### `POST /api/ask`
Ask a question about crawled content (only available when `status == "ready"`).

```json
{ "question": "What are the main topics discussed on this site?" }
```

**Response:**
```json
{
  "answer": "The site primarily covers...",
  "citations": [
    "https://example.com/page1",
    "https://example.com/about"
  ]
}
```

---

## 🧱 Tech Stack

| Layer | Technology |
|---|---|
| Language | Go 1.25 |
| HTTP Server | `net/http` (stdlib) |
| HTML Parsing | `golang.org/x/net/html` |
| Deduplication | Bloom Filter (`bits-and-blooms/bloom`) + Redis (`go-redis/v9`) |
| Object Storage | AWS S3 (`aws-sdk-go-v2`) |
| Relational DB | PostgreSQL (`lib/pq`) |
| LLM / RAG | Groq API — LLaMA 3.1 8B Instant |
| Config | `godotenv` |

---

## 📂 Project Structure

```
WebCrawler/
├── cmd/
│   └── main.go
├── internals/
│   ├── api/
│   ├── buffer/
│   ├── crawler/
│   ├── crawlerFrontier/
│   ├── db/
│   ├── dlq/
│   ├── lease/
│   ├── llm/
│   ├── models/
│   ├── parser/
│   ├── parserQueue/
│   ├── scheduler/
│   └── utils/
├── frontend/
│   └── index.html
├── docker-compose.yml
├── .env
├── go.mod
└── go.sum
```

---

## 🤖 How the RAG Q&A Works

1. As pages are parsed, the first **X characters** of unique text content are extracted and added to an in-memory `ContextForLLM` store, tagged with the source URL. (for demo purposes and to limit the context size which Groq Llama 3.1 8B API could handle , I set x = 550)
2. When a user asks a question via `/api/ask`, all accumulated context is formatted with source attribution and sent to **Groq's LLaMA 3.1 8B** model.
3. The LLM is instructed to answer using **only** the crawled context and to return structured citations.
4. The frontend renders the answer with clickable source links.

---

## 📝 License

MIT
