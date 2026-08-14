TMP_DIR     ?= ./tmp
BIN_DIR     ?= ./bin

GOEXE       := $(shell go env GOEXE)
BUILD_FLAGS ?=

# tip: to disable all linters can set LINT=true
LINT ?= golangci-lint

DOCKER_COMPOSE    ?= docker compose
APP_SERVICE       := server
DB_SERVICE        := db
MIGRATE_SERVICE   := migrate

WAIT_DB_READY     := sh scripts/wait-db-ready.sh
MIGRATE           := sh scripts/migrate.sh
MERGE_CODE        := sh scripts/merge-code.sh

SWAG_SOURCE_DIRS  := ./cmd/$(APP_SERVICE) ./internal/model ./internal/api
SWAG_SOURCES      := $(filter-out %_test.go,$(wildcard $(addsuffix /*.go,$(SWAG_SOURCE_DIRS))))
SWAG_DEST_DIR     := ./pkg/api/docs

# source and dest for merge, patch, etc...
SRC   ?= .
DST   ?= 1

EMPTY :=
SPACE := $(EMPTY) $(EMPTY)
COMMA := ,

# Escape-последовательности терминала
CHECK_COLOR := $(shell [ -n "$(MAKE_TERMOUT)" ] && echo "$(TERM)" | grep -EE "color|256|xterm" >/dev/null && echo "yes")
# use color output (yes|no)
COLOR ?= $(CHECK_COLOR)

E_RSET := $(if $(COLOR),\033[0m)
E_BOLD := $(if $(COLOR),\033[1m)
E_CMD  :=

E_RED    := \033[31m
E_GREEN  := \033[32m
E_YELLOW := \033[33m
E_BLUE   := \033[34m

# Dark Modern Theme (TrueColor)
E_KEY  := $(if $(COLOR),\033[38;2;86;156;214m)
E_VAR  := $(if $(COLOR),\033[38;2;156;220;254m)
E_FUN  := $(if $(COLOR),\033[38;2;230;230;170m)
E_COM  := $(if $(COLOR),\033[38;2;106;153;85m)

.PHONY: all

all: help

# ============================================
# RUN
# ============================================

.PHONY: run docker-run
.NOTPARALLEL: run docker-run

run: generate lint test build db-up migrate-up ## run server (local)
	$(BIN_DIR)/server

docker-run: generate docker-build docker-up ## run server in docker
	$(DOCKER_COMPOSE) logs -f $(APP_SERVICE) $(MIGRATE_SERVICE)

# ============================================
# DOCKER COMMANDS
# ============================================

.PHONY: docker-build docker-up docker-down docker-down-volumes docker-logs
.PHONY: docker-db-up docker-db-down docker-db-down-volumes docker-db-shell

docker-build: ## build docker images
	$(DOCKER_COMPOSE) build

docker-up: ## start all services in docker
	$(DOCKER_COMPOSE) up -d $(APP_SERVICE) 
	@echo "Waiting for services to be ready..."
	@sleep 1
	$(DOCKER_COMPOSE) ps

docker-down: ## stop all docker services
	$(DOCKER_COMPOSE) down

docker-down-volumes: ## stop docker and remove volumes
	$(DOCKER_COMPOSE) down -v

docker-logs: ## show docker logs
	$(DOCKER_COMPOSE) logs -f

docker-db-up:
	$(DOCKER_COMPOSE) up -d $(DB_SERVICE)

docker-db-down: ## stop database container
	$(DOCKER_COMPOSE) down $(DB_SERVICE)

docker-db-down-volumes: ## stop database and remove volumes
	$(DOCKER_COMPOSE) down -v $(DB_SERVICE)

docker-db-shell: ## open psql in db container
	$(DOCKER_COMPOSE) exec $(DB_SERVICE) sh -c 'mariadb -u $$MYSQL_USER -p$$MYSQL_PASSWORD $$MYSQL_DB'

# ============================================
# DEVELOPMENT COMMANDS (local)
# ============================================

.PHONY: init-workspace deps check-goose check-swag check-golangci-lint check-tools build swag-generate go-generate generate test clean merge

init-workspace: ## create/update go.work for local development (includes main module and tests)
	@test -f go.work || go work init
	@go work use .
	@go work use ./tests
	@echo "Workspace ready: main module + tests"

deps: ## update deps
	go mod tidy
	if test -d tests; then cd tests && go mod tidy; fi 

check-goose: ## install goose if need
	@which goose 2>/dev/null || go install github.com/pressly/goose/v3/cmd/goose@v3.27.1

check-swag: ## install swag if need
	@which swag 2>/dev/null || go install github.com/swaggo/swag/cmd/swag@v1.16.6

check-golangci-lint: ## install golangci-lint if need
	@which golangci-lint 2>/dev/null || go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2

check-tools: check-goose check-swag check-golangci-lint ## check all tools


# Находим все поддиректории в cmd, которые потенциально могут быть бинарниками
CMDS := $(wildcard cmd/*)

# Генерируем список целей для бинарников
BINARIES := $(patsubst cmd/%,$(BIN_DIR)/%,$(CMDS))

# Шаблонное правило для сборки любого бинарника
$(BIN_DIR)/%: FORCE
	@mkdir -p $(@D)
	go build $(BUILD_FLAGS) -o $@$(GOEXE) ./cmd/$(notdir $@)

build: $(BINARIES) ## build all binaries

swag-generate: .swag-generate.done ## generate Swagger docs
	
.swag-generate.done: $(SWAG_SOURCES)
	swag fmt  -d $(subst $(SPACE),$(COMMA),$(SWAG_SOURCE_DIRS))
	swag init -d $(subst $(SPACE),$(COMMA),$(SWAG_SOURCE_DIRS)) -o $(SWAG_DEST_DIR)
	@touch $@

go-generate:
	go generate ./...

generate: go-generate swag-generate ## generate all

lint: ## run linters
	$(LINT) run ./...

test: ## run tests
	go test ./internal/...

test-integration: init-workspace ## run integration tests on real database
	go test ./tests/...

clean: ## remove temporary and binary files
	-rm -rf $(BIN_DIR) $(TMP_DIR) .*-generate.done

# ============================================
# DATABASE (local)
# ============================================

.PHONY: db-up db-down db-down-volumes db-shell

db-up: ## start database container for local dev
	@if [ -z "$$($(DOCKER_COMPOSE) ps -q $(DB_SERVICE))" ]; then \
		$(DOCKER_COMPOSE) up -d $(DB_SERVICE) && \
		DOCKER_COMPOSE='$(DOCKER_COMPOSE)' $(WAIT_DB_READY); \
	else \
		echo "Database already running"; \
	fi

db-down: docker-db-down ## alias for docker-db-down

db-down-volumes: docker-db-down-volumes ## alias for docker-db-down-volumes

db-shell: docker-db-shell ## alias for docker-db-shell

# ============================================
# MIGRATIONS (local)
# ============================================

.PHONY: migrate-up migrate-down migrate-status

migrate-up: ## apply all migrations (local)
	$(MIGRATE) up

migrate-down: ## rollback last migration (local)
	$(MIGRATE) down 1

migrate-status: ## show migration status (local)
	$(MIGRATE) status

# ============================================
# UTILS
# ============================================

.PHONY: FORCE patch help 

FORCE:

merge: ## merge code to file for AI review
	@mkdir -p $(TMP_DIR)
	$(MERGE_CODE) $(SRC) > $(TMP_DIR)/$(DST).code 

.NOTPARALLEL: patch
patch: deps generate lint test build ## make precommit patch
	@mkdir -p $(TMP_DIR)
	
	@(set -e; \
	staged_list="$(TMP_DIR)/staged_list.$$$$"; \
	unstaged_list="$(TMP_DIR)/unstaged_list.$$$$"; \
	git diff --staged --name-only -- $(SRC) > "$$staged_list"; \
	git diff --name-only -- $(SRC) > "$$unstaged_list"; \
	intersection=$$(grep -Fxf "$$staged_list" "$$unstaged_list" || true); \
	rm -f "$$staged_list" "$$unstaged_list"; \
	if [ -n "$$intersection" ]; then \
		printf "\n" >&2; \
		printf "$(E_BOLD)WARNING:$(E_RSET) the following files have changes not staged for commit:\n" >&2; \
		printf "  (use \"git add <file>...\" to update what will be committed)\n" >&2; \
		printf "$(E_RED)%s$(E_RSET)\n" $$intersection | sed 's/^/        /' >&2; \
		printf "\n" >&2; \
	fi)
	
	git diff --staged -- $(SRC) > $(TMP_DIR)/$(DST).patch
	@printf "Patch saved to $(TMP_DIR)/$(DST).patch\n"

help: ## show this help
	@printf "$(E_BOLD)Usage:$(E_RSET)\n"
	@printf "  $(E_CMD)make$(E_RSET) [$(E_VAR)VARIABLE$(E_RSET)=value ...] [$(E_FUN)target$(E_RSET) ...]\n"
	@printf "\n$(E_BOLD)Variables:$(E_RSET)\n"
	@awk 'BEGIN {comment=""} \
		/^[a-zA-Z0-9_-]+[[:space:]]*\?=/ { \
			split($$0, a, /\?=/); \
			gsub(/^[ \t]+|[ \t]+$$/, "", a[1]); \
			gsub(/^[ \t]+|[ \t]+$$/, "", a[2]); \
			if ( prev ~ /^#/ ) { \
				gsub(/^[ \t]+|[ \t]+$$/, "", prev); \
				printf "  $(E_VAR)%-14s$(E_RSET) = %-14s $(E_COM)%s$(E_RSET)\n", a[1], a[2], prev; \
			} else { \
				printf "  $(E_VAR)%-14s$(E_RSET) = %-14s\n", a[1], a[2]; \
			} \
		} \
		{ prev=$$0 }' \
		$(MAKEFILE_LIST)
	@printf "\n$(E_BOLD)Targets:$(E_RSET)\n"
	@awk 'BEGIN {FS = ":.*?## "} \
		/^[a-zA-Z0-9_-]+:.*?## / \
		{printf "  $(E_FUN)%-22s$(E_RSET) - %s\n", $$1, $$2}' \
		$(MAKEFILE_LIST)
	@printf "\n$(E_BOLD)Examples:$(E_RSET)\n"
	@printf "  $(E_CMD)make$(E_RSET) $(E_FUN)test$(E_RSET)                     $(E_COM)# run tests$(E_RSET)\n"
	@printf "  $(E_CMD)make$(E_RSET) $(E_FUN)test-integration$(E_RSET)         $(E_COM)# test service on real database$(E_RSET)\n"
	@printf "  $(E_CMD)make$(E_RSET) $(E_FUN)run$(E_RSET)                      $(E_COM)# run server locally$(E_RSET)\n"
	@printf "  $(E_CMD)make$(E_RSET) $(E_FUN)docker-run$(E_RSET)               $(E_COM)# run server in docker$(E_RSET)\n"
	@printf "  $(E_CMD)make$(E_RSET) $(E_FUN)docker-build docker-run$(E_RSET)  $(E_COM)# rebuild docker images and restart$(E_RSET)\n"
