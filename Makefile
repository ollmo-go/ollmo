.PHONY: help up up-full down logs ps infra server worker web migrate build build-server build-web build-docker clean

help:
	@echo "ollmo - RAG platform"
	@echo ""
	@echo "Targets:"
	@echo "  make up           - Start infrastructure (MySQL/Milvus/MinIO/Redis)"
	@echo "  make up-full      - Start full stack (infra + server + worker + web)"
	@echo "  make down         - Stop all services"
	@echo "  make logs         - Tail infrastructure logs"
	@echo "  make ps           - Show service status"
	@echo "  make server       - Run Go API server locally"
	@echo "  make worker       - Run Asynq worker locally"
	@echo "  make web          - Run Next.js dev server"
	@echo "  make migrate      - Run database migrations"
	@echo "  make build        - Build server binary + web"
	@echo "  make build-docker - Build Docker images for server, worker, web"
	@echo "  make clean        - Remove build artifacts"
	@echo ""
	@echo "Containers: ollmo-mysql, ollmo-redis, ollmo-minio, ollmo-etcd,"
	@echo "            ollmo-milvus, ollmo-asynqmon, ollmo-server, ollmo-worker, ollmo-web"

up:
	docker compose up -d mysql redis minio etcd milvus asynqmon

up-full:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f --tail=100

ps:
	docker compose ps

server:
	cd server && go run . api

worker:
	cd server && go run . worker

web:
	cd web && npm run dev

migrate:
	cd server && go run . migrate

build: build-server build-web

build-server:
	cd server && go build -o bin/ollmo .

build-web:
	cd web && npm run build

build-docker:
	docker compose build server worker web

clean:
	rm -rf server/bin web/.next web/node_modules
