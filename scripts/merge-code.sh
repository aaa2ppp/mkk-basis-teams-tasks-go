#!/bin/sh

# Скрипт для объединения исходных файлов проекта
# Использование: ./merge_code.sh <директория1> <директория2> ...

for dir in "$@"; do
    # Ищем файлы с нужными расширениями, исключая тестовые миграции
    find "$dir" -type f \
        ! -path '*/bak/*' ! -path '*/tmp/*' ! -path '*/external/*' \
        \( -name '*.go' -o -name '*.sql' -o -name '*.js' -o -name '*.sh' -o -name '*.md' \
        -o -name 'Dockerfile*' -o -name '*.y*ml' -o -name 'Makefile*' -o -name '*.example' \
        -o -name go.mod -o -name dev-env -o -name '*.json' \) \
    | sort | while read -r f; do
        # Убираем ./ в начале пути
        f="${f#./}"
        
        # Выводим содержимое с заголовком
        echo "=== $f ==="
        echo
        cat "$f"
        echo
    done
done
