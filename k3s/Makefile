PROJECT   := teams-tasks
NAMESPACE ?= $(PROJECT)
TAG       ?= v0.0.1
REPLICAS  ?= 3
TIMEOUT   ?= 60s
SERVER_NODE_PORT ?= 30081

SERVER_IMG  := $(PROJECT)/server:$(TAG)
MIGRATE_IMG := $(PROJECT)/migrate:$(TAG)

CHECK_SOCKET := if [ ! -w /run/k3s/containerd/containerd.sock ]; then \
		echo "ERROR: No write access to /run/k3s/containerd/containerd.sock" >&2; \
		echo "Check rights to socket and config /etc/rancher/k3s/config.yaml" >&2; \
		exit 1; \
	fi

LOAD_IMAGE := $(CHECK_SOCKET) && docker save $$image | k3s ctr images import -

# ==============================================================================
# ОСНОВНЫЕ ЦЕЛИ
# ==============================================================================
.PHONY: all build load  update destroy help
.NOTPARALLEL: all .deploy-steps update destroy

all: build load .deploy-steps

build: srv-build mig-build

load: srv-load mig-load

.PHONY: .deploy-steps
.deploy-steps: \
	ns-create \
	env-init \
	db-apply \
	redis-apply \
	srv-stop \
	srv-wait-stop \
	db-wait \
	mig-run \
	mig-wait \
	redis-wait \
	srv-apply

status:
	@kubectl get all -n $(NAMESPACE)

update: \
	srv-build \
	srv-load \
	srv-restart

.PHONY: .destroy-confirm
.destroy-confirm:
	@read -p "Are you sure? This will remove ALL resources in $(NAMESPACE), including the database! (y/N) " confirm; \
	if [ "$$confirm" != "y" ] && [ "$$confirm" != "Y" ]; then \
		echo "Aborted." >&2; \
		exit 1; \
	fi

destroy: .destroy-confirm ns-delete

help:
	@echo "Основные цели:"
	@echo "  all      – сборка + загрузка + полный деплой"
	@echo "  update   – сборка + загрузка + рестарт сервера (без миграций)"
	@echo "  destroy  – уничтожить ВСЕ! (включая данные базы)"
	@echo "  help     - вывод этой справки"
	

# ==============================================================================
# ОКРУЖЕНИЕ
# ==============================================================================
.PHONY: ns-create ns-delete env-init

ns-create:
	kubectl create namespace $(NAMESPACE) 2>/dev/null || true

ns-delete: ## destroy all!
	kubectl delete namespace $(NAMESPACE) --ignore-not-found

env-init:
	kubectl apply -n $(NAMESPACE) -f cfg/configmap.yaml

	kubectl get secret app-secret -n $(NAMESPACE) >/dev/null 2>&1 || \
		kubectl create secret generic app-secret -n $(NAMESPACE) \
		--from-literal=DB_PASSWORD="$$(head -c16 /dev/urandom | base64)" \
		--from-literal=DB_ROOT_PASSWORD="$$(head -c16 /dev/urandom | base64)" \
		--from-literal=REDIS_PASSWORD="$$(head -c16 /dev/urandom | base64)" \
		--from-literal=AUTH_SECRET="$$(head -c32 /dev/urandom | od -An -v -tx1 | tr -d ' \n')"

	kubectl apply -n $(NAMESPACE) -f cfg/db-service.yaml 
	kubectl apply -n $(NAMESPACE) -f cfg/redis-service.yaml 
	env -i SERVER_NODE_PORT=$(SERVER_NODE_PORT) \
			envsubst < cfg/server-service.yaml | kubectl apply -n $(NAMESPACE) -f -

# ==============================================================================
# БАЗА ДАННЫХ
# ==============================================================================
.PHONY: db-apply db-wait db-delete

db-apply:
	kubectl apply -n $(NAMESPACE) -f cfg/db-statefulset.yaml

db-wait:
	kubectl wait -n $(NAMESPACE) --for=create --for=condition=ready pod -l app=db --timeout=$(TIMEOUT)

db-delete: # StatefulSet удаляется, PVC остаётся
	kubectl delete -n $(NAMESPACE) -f cfg/db-statefulset.yaml   

# ==============================================================================
# REDIS
# ==============================================================================
.PHONY: redis-apply redis-wait redis-delete

redis-apply:
	kubectl apply -n $(NAMESPACE) -f cfg/redis-deployment.yaml

redis-wait:
	kubectl wait -n $(NAMESPACE) --for=create --for=condition=ready pod -l app=redis --timeout=$(TIMEOUT)

redis-delete:
	kubectl delete -n $(NAMESPACE) -f cfg/redis-deployment.yaml   

# ==============================================================================
# МИГРАЦИИ
# ==============================================================================
.PHONY: mig-build mig-load mig-run

mig-build:
	docker build -t $(MIGRATE_IMG) --target migrate ./..

mig-load:
	@echo load image: $(MIGRATE_IMG) ...
	@image=$(MIGRATE_IMG); $(LOAD_IMAGE)

mig-run:
	kubectl delete job migrate -n $(NAMESPACE) --ignore-not-found
	env -i MIGRATE_IMG=$(MIGRATE_IMG) \
		envsubst < cfg/migrate-job.yaml | kubectl apply -n $(NAMESPACE) -f -

mig-wait:
	kubectl wait -n $(NAMESPACE) --for=create --for=condition=complete job/migrate --timeout=$(TIMEOUT)

# ==============================================================================
# СЕРВЕР
# ==============================================================================
.PHONY: srv-build srv-load srv-apply srv-stop srv-wait-stop

srv-build:
	docker build -t $(SERVER_IMG) --target server ./..

srv-load:
	@echo load image: $(SERVER_IMG) ...
	@image=$(SERVER_IMG); $(LOAD_IMAGE)
	
srv-apply:
	env -i SERVER_IMG=$(SERVER_IMG) REPLICAS=$(REPLICAS) \
		envsubst < cfg/server-deployment.yaml | kubectl apply -n $(NAMESPACE) -f -

srv-stop:
	kubectl scale -n $(NAMESPACE) deployment/server --replicas=0 2>/dev/null || true

srv-wait-stop:
	kubectl wait -n $(NAMESPACE) --for=delete pod -l app=server --timeout=$(TIMEOUT)

srv-restart:
	kubectl rollout restart -n $(NAMESPACE) deployment/server
