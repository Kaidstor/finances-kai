set shell := ["bash", "-cu"]

version := `git describe --tags --always --dirty 2>/dev/null || echo dev`
ldflags := "-s -w -X github.com/Kaidstor/finances-next/cli/internal/command.version=" + version

# Список рецептов
default:
    @just --list

# Сборка в ./bin/finances-kai
build:
    CGO_ENABLED=0 go build -trimpath -ldflags "{{ldflags}}" -o bin/finances-kai .

# Тесты и статический анализ
test:
    go vet ./...
    go test ./...

# Установка в ~/.local/bin
install: build
    mkdir -p ~/.local/bin
    install -m 0755 bin/finances-kai ~/.local/bin/finances-kai
    @echo "готово: $(~/.local/bin/finances-kai version)"

# Подключить скилл агенту (симлинк в ~/.claude/skills)
install-skill:
    mkdir -p ~/.claude/skills
    ln -sfn "$(pwd)/skills/finances-kai" ~/.claude/skills/finances-kai
    @echo "скилл подключён: ~/.claude/skills/finances-kai"

# Кросс-компиляция под linux/amd64 (для сервера)
build-linux:
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "{{ldflags}}" -o bin/linux-amd64/finances-kai .

fmt:
    go fmt ./...

clean:
    rm -rf bin
