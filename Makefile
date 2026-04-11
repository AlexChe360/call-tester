# Makefile для call-tester

BINARY = call-tester
RPI_HOST = pi@192.168.0.107
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
	ssh $(RPI_HOST) "mkdir -p $(RPI_DIR)/scenarios $(RPI_DIR)/reports"
	scp $(BINARY)-linux-arm64 $(RPI_HOST):$(RPI_DIR)/$(BINARY)
	scp config.yaml $(RPI_HOST):$(RPI_DIR)/
	scp scenarios/*.yaml $(RPI_HOST):$(RPI_DIR)/scenarios/
	scp setup_rpi.sh $(RPI_HOST):$(RPI_DIR)/
	ssh $(RPI_HOST) "chmod +x $(RPI_DIR)/$(BINARY) $(RPI_DIR)/setup_rpi.sh"
	@echo ""
	@echo "Деплой завершён. На RPi:"
	@echo "  cd $(RPI_DIR)"
	@echo "  ./call-tester check"

# Проверка модемов (на RPi)
check:
	./$(BINARY) check

# Запуск сценария (на RPi)
run:
	./$(BINARY) run scenarios/full_matrix.yaml -o reports/

# Тестовый звонок (на RPi)
test-call:
	./$(BINARY) call sim7600 ec25_1 -d 10

clean:
	rm -f $(BINARY) $(BINARY)-linux-arm64

.PHONY: build build-rpi deploy check run test-call clean