.PHONY: setup start

start: setup
	docker compose up -d

stop:
	docker compose down

build:
	docker compose build

restart: stop start

logs:
	docker compose logs -f