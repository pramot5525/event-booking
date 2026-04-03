.PHONY: setup start

start: setup
	docker compose up -d --build

stop:
	docker compose down -v

build:
	docker compose build

restart: stop start

logs:
	docker compose logs -f