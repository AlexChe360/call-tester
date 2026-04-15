# Makefile для call-tester

BINARY = call-tester
RPI_HOST = pi@192.168.0.109
RPI_DIR = /home/pi/call-tester

# Сборка локально (на RPi)
build:
	go build -o $(BINARY) ./cmd/call-tester

# Кросс-компиляция на маке для RPi 5 (ARM64 Linux)
build-rpi:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(BINARY)-linux-arm64 ./cmd/call-tester
	@echo "Готово: $(BINARY)-linux-arm64"

# Деплой на RPi (с мака)
deploy: build-rpi
	@echo "📦 Деплой на RPi..."
	ssh $(RPI_HOST) "mkdir -p $(RPI_DIR)/scenarios $(RPI_DIR)/reports"
	scp -r $(BINARY)-linux-arm64 $(RPI_HOST):$(RPI_DIR)/$(BINARY)
	scp -r config.yaml $(RPI_HOST):$(RPI_DIR)/
	scp -r scenarios/*.yaml $(RPI_HOST):$(RPI_DIR)/scenarios/ 2>/dev/null || true
	scp -r setup_rpi.sh $(RPI_HOST):$(RPI_DIR)/ 2>/dev/null || true
	ssh $(RPI_HOST) "chmod +x $(RPI_DIR)/$(BINARY)"
	@echo ""
	@echo "✅ Деплой завершён. На RPi выполните:"
	@echo "  cd $(RPI_DIR) && ./call-tester check"

# Принудительная пересборка и деплой
rebuild: clean build-rpi deploy

# === Команды для выполнения на RPi через SSH ===
check:
	@echo "🔍 Проверка модемов на RPi..."
	ssh $(RPI_HOST) "cd $(RPI_DIR) && ./$(BINARY) check"

run-scenario:
	@echo "📞 Запуск теста на RPi..."
	ssh $(RPI_HOST) "cd $(RPI_DIR) && ./$(BINARY) run scenarios/full_scenario.yaml -o reports/"

# Получение отчётов с RPi
fetch-reports:
	@echo "📥 Скачивание отчётов с RPi..."
	@mkdir -p reports
	scp $(RPI_HOST):$(RPI_DIR)/reports/*.json reports/ 2>/dev/null || echo "Нет JSON отчётов"
	scp $(RPI_HOST):$(RPI_DIR)/reports/*.csv reports/ 2>/dev/null || echo "Нет CSV отчётов"
	@echo "✅ Отчёты скопированы в ./reports/"

# Просмотр последних отчётов
show-reports:
	@echo "📊 Последние отчёты на RPi:"
	ssh $(RPI_HOST) "cd $(RPI_DIR) && ls -la reports/ | tail -5"

clean:
	rm -f $(BINARY) $(BINARY)-linux-arm64

.PHONY: build build-rpi deploy check run test-call clean