# UptimeWatch — Мониторинг доступности

Проект №08. Самостоятельное Go-приложение. Сборка: `./cmd/uptimewatch`.
Общие пакеты находятся в `internal/platform`, поэтому сохраняйте структуру репозитория.

## Реализовано в v0.1

HTTP(S)-проверки; интервалы и пороги; несколько воркеров через SKIP LOCKED; история задержек; один открытый инцидент; автоматическое закрытие; пауза монитора; отчёт доступности.

Данные: `monitors, checks, incidents`. Схема: `../../migrations/uptimewatch_001.sql`.
Все записи изолированы по tenant; роль определяется серверным API-токеном.

## Запуск

Из корня репозитория:

```bash
python3 scripts/setup.py  # один раз для всего набора
PROJECT=uptimewatch docker compose up --build -d app
python3 scripts/demo.py uptimewatch
```

Для JobRunner запускайте ещё `--profile jobs`; точные команды профилей — в корневом README.
Демо создаёт тестовые записи. Остановить: `docker compose down`.

## API

Все пути требуют `Authorization: Bearer <token>`. Ошибки возвращаются как JSON.
`Idempotency-Key` необязателен, но рекомендуется для команд создания и отправки.
Повтор того же запроса с тем же ключом возвращает сохранённый ответ; изменение запроса даёт 409.

| Метод | Путь |
|---|---|
| POST | `/monitors` |
| GET | `/monitors` |
| PATCH | `/monitors/{id}` |
| GET | `/monitors/{id}/checks` |
| GET | `/incidents` |
| GET | `/status` |

## Примеры тел запросов

`POST /monitors`:

```json
{
  "url": "https://example.com",
  "interval_seconds": 60,
  "threshold": 3
}
```

`PATCH /monitors/{id}`:

```json
{
  "paused": true
}
```

## Проверка

```bash
TEST_DATABASE_URL='postgres://portfolio:portfolio-local-only@localhost:5432/portfolio?sslmode=disable' \
  go test -count=1 -v ./tests -run '^TestUptimeWatch$'
```

Интеграционные тесты пишут данные в указанную БД. Используйте отдельную тестовую базу.
Полный сценарий проверки находится в `../../tests/integration_test.go`.

## Границы версии и дальнейшая работа

TCP-проверки, плановые окна обслуживания, ClickHouse-история и доставка инцидентов через NotifyHub. Публичного веб-интерфейса нет.

Это реализованное ядро проекта, а не заявление о завершении всех расширений исходной идеи.
Подробнее об ограничениях окружения проверки — `../../VALIDATION.md`.
