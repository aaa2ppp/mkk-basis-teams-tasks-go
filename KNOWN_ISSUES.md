# Обнаруженные проблемы

## Проблема с `enumer` на Windows

При выполнении `go generate ./...` возникает ошибка:

```
enumer: internal error: package "errors" without types was imported from "..."
```

### Решение (без изменения кода проекта)

Соберите `enumer` вручную с актуальными зависимостями:

```bash
git clone https://github.com/dmarkham/enumer.git
cd enumer
go get -u ./...   # обновляет все зависимости до последних версий
go install
```

После этого команда `enumer` должна корректно работать на Windows.

Проверено на Go v1.27.1, enumer v1.6.3

Рабочий `go.mod`

```
module github.com/dmarkham/enumer

require (
	github.com/pascaldekloe/name v1.0.1
	golang.org/x/tools v0.50.0
)

require (
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
)

go 1.26.0
```
