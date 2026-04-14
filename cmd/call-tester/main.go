package main

import (
	"call-tester/internal/engine"
	"call-tester/internal/models"
	"call-tester/internal/modem"
	"call-tester/internal/report"
	"flag"
	"fmt"
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	configPath := "config.yaml"

	// Глобальный флаг -c для конфига
	for i, arg := range os.Args {
		if arg == "-c" && i+1 < len(os.Args) {
			configPath = os.Args[i+1]
			// Убираем -c и значение из аргументов
			os.Args = append(os.Args[:i], os.Args[i+2:]...)
			break
		}
	}

	switch os.Args[1] {
	case "check":
		config := mustLoadConfig(configPath)
		cmdCheck(config)

	case "run":
		runFlags := flag.NewFlagSet("run", flag.ExitOnError)
		output := runFlags.String("o", "reports", "директория для отчётов")
		runFlags.Parse(os.Args[2:])

		if runFlags.NArg() < 1 {
			fmt.Println("Использование: call-tester run <scenario.yaml> [-o reports/]")
			os.Exit(1)
		}

		config := mustLoadConfig(configPath)
		scenario := mustLoadScenario(runFlags.Arg(0))
		cmdRun(config, scenario, *output)

	case "call":
		callFlags := flag.NewFlagSet("call", flag.ExitOnError)
		duration := callFlags.Int("d", 10, "длительность удержания (сек)")
		output := callFlags.String("o", "reports", "директория для отчётов")
		callFlags.Parse(os.Args[2:])

		if callFlags.NArg() < 2 {
			fmt.Println("Использование: call-tester call <from_modem> <to_modem> [-d 10] [-o reports/]")
			os.Exit(1)
		}

		config := mustLoadConfig(configPath)
		scenario := &models.Scenario{
			Name: fmt.Sprintf("single_%s_%s", callFlags.Arg(0), callFlags.Arg(1)),
			Steps: []models.ScenarioStep{
				{
					Action:          "call",
					FromModem:       callFlags.Arg(0),
					ToModem:         callFlags.Arg(1),
					HoldDurationSec: *duration,
				},
			},
		}
		cmdRun(config, scenario, *output)

	case "sms":
		smsFlags := flag.NewFlagSet("sms", flag.ExitOnError)
		output := smsFlags.String("o", "reports", "директория для отчётов")
		smsFlags.Parse(os.Args[2:])

		if smsFlags.NArg() < 3 {
			fmt.Println("Использование: call-tester sms <from_modem> <to_number> <message> [-o reports/]")
			fmt.Println("Пример: call-tester sms ec25_1 +77009999999 \"Hello world\"")
			os.Exit(1)
		}

		config := mustLoadConfig(configPath)
		fromModem := smsFlags.Arg(0)
		toNumber := smsFlags.Arg(1)
		message := smsFlags.Arg(2)

		// Создаём одношаговый сценарий
		scenario := &models.Scenario{
			Name: fmt.Sprintf("sms_%s_to_%s", fromModem, toNumber),
			Steps: []models.ScenarioStep{
				{
					Action:    "sms",
					FromModem: fromModem,
					ToNumber:  toNumber,
					Message:   message,
				},
			},
		}
		cmdRun(config, scenario, *output)

	case "example-config":
		cmdExampleConfig()

	case "example-scenario":
		cmdExampleScenario()

	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`call-tester — тестирование звонков, SMS и интернета

Использование:
  call-tester [-c config.yaml] <command> [args]

Команды:
  check                          Проверить подключение всех модемов
  run <scenario.yaml> [-o dir]   Запустить сценарий
  call <from> <to> [-d sec]      Одиночный тестовый звонок
  sms <from> <number> <msg>      Отправить SMS на любой номер
  example-config                 Показать пример конфигурации
  example-scenario               Показать пример сценария`)
}

// ---- Команды ----

func cmdCheck(config *models.SystemConfig) {
	fmt.Println("\n=== Проверка модемов ===\n")

	for _, cfg := range config.Modems {
		fmt.Printf("Модем '%s' (%s) на %s ... ", cfg.Name, cfg.Model, cfg.Port)

		ctrl, err := modem.New(cfg.Port, cfg.BaudRate, cfg.Name, cfg.PhoneNumber, cfg.Model)
		if err != nil {
			fmt.Printf("✗ Не удалось открыть порт: %v\n", err)
			continue
		}
		defer ctrl.Close()

		if err := ctrl.Init(); err != nil {
			fmt.Printf("✗ Ошибка инициализации: %v\n", err)
			continue
		}

		rssi, _, err := ctrl.GetSignalQuality()
		signal := "?"
		if err == nil {
			signal = modem.SignalDBm(rssi)
		}

		fmt.Printf("✓ OK (сигнал: %s)\n", signal)
	}
	fmt.Println()
}

func cmdRun(config *models.SystemConfig, scenario *models.Scenario, outputDir string) {
	manager, err := engine.NewModemManager(config)
	if err != nil {
		log.Fatalf("Ошибка инициализации модемов: %v", err)
	}
	defer manager.Close()

	eng := engine.NewEngine(manager)
	rep, err := eng.Execute(scenario)
	if err != nil {
		log.Fatalf("Ошибка выполнения сценария: %v", err)
	}

	jsonPath, err := report.SaveJSON(rep, outputDir)
	if err != nil {
		log.Printf("Ошибка сохранения JSON: %v", err)
	}

	csvPath, err := report.SaveCSV(rep, outputDir)
	if err != nil {
		log.Printf("Ошибка сохранения CSV: %v", err)
	}

	report.PrintSummary(rep)

	fmt.Println("Отчёты сохранены:")
	if jsonPath != "" {
		fmt.Printf("  JSON: %s\n", jsonPath)
	}
	if csvPath != "" {
		fmt.Printf("  CSV:  %s\n", csvPath)
	}
}

func cmdExampleConfig() {
	fmt.Print(`# Конфигурация модемов для call-tester
#
# Определить порты:
#   ls /dev/ttyUSB*
#   Подключайте модемы по одному, чтобы понять какие порты чьи.
#   AT-порт обычно 2-й или 3-й (считая с 0).
#
# SIM7600:  создаёт 5 портов, AT = ttyUSBx+2
# EC25/EP06: создаёт 4 порта, AT = ttyUSBx+2 или x+3

modems:
  - name: sim7600
    port: /dev/ttyUSB2
    baud_rate: 115200
    model: SIM7600E-L1C
    phone_number: "+77001111111"

  - name: ec25_1
    port: /dev/ttyUSB7
    baud_rate: 115200
    model: Quectel EC25-EUX
    phone_number: "+77002222222"

  - name: ec25_2
    port: /dev/ttyUSB11
    baud_rate: 115200
    model: Quectel EC25-EUX
    phone_number: "+77003333333"
`)
}

func cmdExampleScenario() {
	fmt.Print(`# Сценарий тестирования звонков

name: billing_test
description: Базовый тест для сверки с биллингом

steps:
  - action: call
    from_modem: sim7600
    to_modem: ec25_1
    hold_duration_sec: 30

  - action: pause
    duration_sec: 5

  - action: call
    from_modem: ec25_1
    to_modem: sim7600
    hold_duration_sec: 30

  - action: pause
    duration_sec: 5

  - action: call
    from_modem: ec25_2
    to_modem: ec25_1
    hold_duration_sec: 45

`)
}

// ---- Загрузка файлов ----

func mustLoadConfig(path string) *models.SystemConfig {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Не удалось прочитать конфиг %s: %v", path, err)
	}

	var config models.SystemConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		log.Fatalf("Ошибка парсинга конфига: %v", err)
	}

	log.Printf("Загружен конфиг: %d модемов", len(config.Modems))
	return &config
}

func mustLoadScenario(path string) *models.Scenario {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Не удалось прочитать сценарий %s: %v", path, err)
	}

	var scenario models.Scenario
	if err := yaml.Unmarshal(data, &scenario); err != nil {
		log.Fatalf("Ошибка парсинга сценария: %v", err)
	}

	log.Printf("Загружен сценарий '%s': %d шагов", scenario.Name, len(scenario.Steps))
	return &scenario
}
