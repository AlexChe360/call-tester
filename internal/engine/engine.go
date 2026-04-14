package engine

import (
	"call-tester/internal/models"
	"call-tester/internal/modem"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

// ModemManager хранит все контроллеры модемов
type ModemManager struct {
	controllers map[string]*modem.Controller
	configs     map[string]models.ModemConfig
}

// NewModemManager инициализирует все модемы из конфига
func NewModemManager(config *models.SystemConfig) (*ModemManager, error) {
	m := &ModemManager{
		controllers: make(map[string]*modem.Controller),
		configs:     make(map[string]models.ModemConfig),
	}

	for _, cfg := range config.Modems {
		log.Printf("Подключаем модем '%s' на %s (%s)", cfg.Name, cfg.Port, cfg.Model)

		ctrl, err := modem.New(cfg.Port, cfg.BaudRate, cfg.Name, cfg.PhoneNumber, cfg.Model)
		if err != nil {
			return nil, fmt.Errorf("модем '%s': %w", cfg.Name, err)
		}

		if err := ctrl.Init(); err != nil {
			ctrl.Close()
			return nil, fmt.Errorf("модем '%s' инициализация: %w", cfg.Name, err)
		}

		// Проверяем сигнал
		rssi, _, err := ctrl.GetSignalQuality()
		if err != nil {
			log.Printf("  [%s] Не удалось проверить сигнал: %v", cfg.Name, err)
		} else {
			log.Printf("  [%s] Сигнал: RSSI=%d (%s)", cfg.Name, rssi, modem.SignalDBm(rssi))
		}

		m.controllers[cfg.Name] = ctrl
		m.configs[cfg.Name] = cfg
	}

	return m, nil
}

// Close закрывает все порты
func (m *ModemManager) Close() {
	for name, ctrl := range m.controllers {
		log.Printf("Закрываем модем '%s'", name)
		ctrl.Close()
	}
}

// Get возвращает контроллер по имени
func (m *ModemManager) Get(name string) (*modem.Controller, error) {
	ctrl, ok := m.controllers[name]
	if !ok {
		return nil, fmt.Errorf("модем '%s' не найден", name)
	}
	return ctrl, nil
}

// Config возвращает конфиг модема по имени
func (m *ModemManager) Config(name string) (models.ModemConfig, error) {
	cfg, ok := m.configs[name]
	if !ok {
		return cfg, fmt.Errorf("конфиг модема '%s' не найден", name)
	}
	return cfg, nil
}

// Engine — движок исполнения сценариев
type Engine struct {
	manager *ModemManager
}

// NewEngine создаёт движок
func NewEngine(manager *ModemManager) *Engine {
	return &Engine{manager: manager}
}

// Execute выполняет сценарий целиком
func (e *Engine) Execute(scenario *models.Scenario) (*models.ScenarioReport, error) {
	log.Printf("=== Запуск сценария: '%s' ===", scenario.Name)

	var records []models.CallRecord

	for idx, step := range scenario.Steps {
		log.Printf("--- Шаг %d/%d ---", idx+1, len(scenario.Steps))

		switch step.Action {
		case "call":
			record, err := e.executeCall(step.FromModem, step.ToModem, step.HoldDurationSec, scenario.Name, idx)
			if err != nil {
				log.Printf("  Ошибка: %v", err)

				// Создаём запись об ошибке
				fromCfg, _ := e.manager.Config(step.FromModem)
				toCfg, _ := e.manager.Config(step.ToModem)
				record = makeFailedRecord(step.FromModem, fromCfg.PhoneNumber,
					step.ToModem, toCfg.PhoneNumber,
					scenario.Name, idx, fmt.Sprintf("error: %v", err))
			}

			log.Printf("  Результат: %s -> %s | %s | %.1fс",
				record.NumberA, record.NumberB, record.Status,
				ptrFloat(record.TalkDurationSec))
			records = append(records, record)
		case "sms":
			var record models.CallRecord
			var err error

			// Поддерживаем оба варианта: to_modem (для модемов из конфига) и to_number (для любых номеров)
			if step.ToNumber != "" {
				record, err = e.executeSMS(step.FromModem, "", step.ToNumber, step.Message, scenario.Name, idx)
			} else if step.ToModem != "" {
				record, err = e.executeSMS(step.FromModem, step.ToModem, "", step.Message, scenario.Name, idx)
			} else {
				err = fmt.Errorf("для SMS нужно указать to_number или to_modem")
				// Создаём запись об ошибке
				fromCfg, _ := e.manager.Config(step.FromModem)
				now := time.Now()
				record = models.CallRecord{
					ID:           uuid.New().String(),
					FromModem:    step.FromModem,
					NumberA:      fromCfg.PhoneNumber,
					CallStart:    now,
					CallEnd:      &now,
					Status:       models.StatusFailed,
					ScenarioName: scenario.Name,
					StepIndex:    idx,
				}
			}

			if err != nil {
				log.Printf("  Ошибка SMS: %v", err)
			} else {
				log.Printf("  SMS: %s -> %s | %s", record.NumberA, record.NumberB, record.Status)
			}
			records = append(records, record)

		case "internet_on":
			record, err := e.executeInternetConnect(step.FromModem, step.APN, step.APNUser, step.APNPassword, scenario.Name, idx)
			if err != nil {
				log.Printf("  Ошибка подключения интернета: %v", err)
			} else {
				log.Printf("  Интернет: %s | %s", record.FromModem, record.Status)
			}
			records = append(records, record)

		case "internet_off":
			record, err := e.executeInternetDisconnect(step.FromModem, scenario.Name, idx)
			if err != nil {
				log.Printf("  Ошибка отключения интернета: %v", err)
			} else {
				log.Printf("  Интернет: %s | %s", record.FromModem, record.Status)
			}
			records = append(records, record)

		case "pause":
			log.Printf("  Пауза %d сек...", step.DurationSec)
			time.Sleep(time.Duration(step.DurationSec) * time.Second)
		}
	}

	successful := 0
	for _, r := range records {
		if r.Status == models.StatusAnswered {
			successful++
		}
	}

	report := &models.ScenarioReport{
		ScenarioName:    scenario.Name,
		ExecutedAt:      time.Now(),
		TotalCalls:      len(records),
		SuccessfulCalls: successful,
		FailedCalls:     len(records) - successful,
		Records:         records,
	}

	log.Printf("=== Сценарий '%s' завершён: %d звонков (%d успешных, %d неудачных) ===",
		scenario.Name, report.TotalCalls, report.SuccessfulCalls, report.FailedCalls)

	return report, nil
}

// executeCall — ключевая логика одного звонка
func (e *Engine) executeCall(fromName, toName string, holdSec int, scenarioName string, stepIdx int) (models.CallRecord, error) {
	fromCfg, err := e.manager.Config(fromName)
	if err != nil {
		return models.CallRecord{}, err
	}
	toCfg, err := e.manager.Config(toName)
	if err != nil {
		return models.CallRecord{}, err
	}

	caller, err := e.manager.Get(fromName)
	if err != nil {
		return models.CallRecord{}, err
	}
	receiver, err := e.manager.Get(toName)
	if err != nil {
		return models.CallRecord{}, err
	}

	record := models.CallRecord{
		ID:           uuid.New().String(),
		Direction:    "outgoing",
		FromModem:    fromName,
		NumberA:      fromCfg.PhoneNumber,
		ToModem:      toName,
		NumberB:      toCfg.PhoneNumber,
		CallStart:    time.Now(),
		Status:       models.StatusInitiated,
		ScenarioName: scenarioName,
		StepIndex:    stepIdx,
	}

	log.Printf("  Звонок: %s (%s) -> %s (%s), удержание %dс",
		fromName, fromCfg.PhoneNumber, toName, toCfg.PhoneNumber, holdSec)

	// --- Горутина: приёмник ждёт входящий ---
	type receiverResult struct {
		caller   string
		answered bool
		err      error
	}
	receiverCh := make(chan receiverResult, 1)

	go func() {
		caller, answered, err := receiver.WaitAndAnswer(30 * time.Second)
		receiverCh <- receiverResult{caller, answered, err}
	}()

	// Даём приёмнику войти в режим ожидания
	time.Sleep(1 * time.Second)

	// --- Звоним ---
	connected, err := caller.Dial(toCfg.PhoneNumber)
	if err != nil {
		record.Status = models.StatusFailed
		now := time.Now()
		record.CallEnd = &now
		dur := time.Since(record.CallStart).Seconds()
		record.TotalDurationSec = &dur
		<-receiverCh // ждём горутину
		return record, nil
	}

	if !connected {
		record.Status = models.StatusNoAnswer
		now := time.Now()
		record.CallEnd = &now
		dur := time.Since(record.CallStart).Seconds()
		record.TotalDurationSec = &dur
		<-receiverCh
		return record, nil
	}

	// Ждём ответа приёмника
	res := <-receiverCh
	if !res.answered {
		record.Status = models.StatusFailed
		now := time.Now()
		record.CallEnd = &now
		dur := time.Since(record.CallStart).Seconds()
		record.TotalDurationSec = &dur
		caller.Hangup()
		return record, nil
	}

	// --- Вызов установлен ---
	now := time.Now()
	record.AnswerTime = &now
	record.Status = models.StatusAnswered

	log.Printf("  Вызов установлен, удерживаем %d сек...", holdSec)
	time.Sleep(time.Duration(holdSec) * time.Second)

	// --- Вешаем трубку ---
	caller.Hangup()
	time.Sleep(500 * time.Millisecond)
	receiver.Hangup()

	endTime := time.Now()
	record.CallEnd = &endTime

	talkDur := endTime.Sub(*record.AnswerTime).Seconds()
	record.TalkDurationSec = &talkDur

	totalDur := endTime.Sub(record.CallStart).Seconds()
	record.TotalDurationSec = &totalDur

	return record, nil
}

// executeSMS — отправка SMS на любой номер
func (e *Engine) executeSMS(fromName, toName, toNumber, message, scenarioName string, stepIdx int) (models.CallRecord, error) {
	fromCfg, err := e.manager.Config(fromName)
	if err != nil {
		return models.CallRecord{}, err
	}

	sender, err := e.manager.Get(fromName)
	if err != nil {
		return models.CallRecord{}, err
	}

	// Определяем номер получателя
	var targetNumber string
	var targetModem string

	if toNumber != "" {
		// Отправка на произвольный номер
		targetNumber = toNumber
		targetModem = "" // не связано с модемом из конфига
		log.Printf("  SMS на произвольный номер: %s", targetNumber)
	} else if toName != "" {
		// Отправка на модем из конфига (для обратной совместимости)
		toCfg, err := e.manager.Config(toName)
		if err != nil {
			return models.CallRecord{}, err
		}
		targetNumber = toCfg.PhoneNumber
		targetModem = toName
		log.Printf("  SMS на модем: %s (%s)", toName, targetNumber)
	} else {
		return models.CallRecord{}, fmt.Errorf("не указан получатель: нужен to_number или to_modem")
	}

	smsRecord := &models.SMSRecord{
		ID:         uuid.New().String(),
		FromModem:  fromName,
		FromNumber: fromCfg.PhoneNumber,
		ToNumber:   targetNumber,
		ToModem:    targetModem,
		Message:    message,
		SentAt:     time.Now(),
		Status:     models.StatusInitiated,
	}

	log.Printf("  SMS: %s -> %s: \"%s\"", fromName, targetNumber, message)

	err = sender.SendSMS(targetNumber, message)
	if err != nil {
		smsRecord.Status = models.StatusFailed
		smsRecord.Error = err.Error()

		return models.CallRecord{
			ID:           smsRecord.ID,
			Direction:    "outgoing",
			FromModem:    fromName,
			NumberA:      fromCfg.PhoneNumber,
			ToModem:      targetModem,
			NumberB:      targetNumber,
			CallStart:    smsRecord.SentAt,
			CallEnd:      &smsRecord.SentAt,
			Status:       models.StatusFailed,
			ScenarioName: scenarioName,
			StepIndex:    stepIdx,
			SMSRecord:    smsRecord,
		}, err
	}

	smsRecord.Status = models.StatusSent
	log.Printf("  SMS успешно отправлена на %s", targetNumber)

	now := time.Now()
	return models.CallRecord{
		ID:           smsRecord.ID,
		Direction:    "outgoing",
		FromModem:    fromName,
		NumberA:      fromCfg.PhoneNumber,
		ToModem:      targetModem,
		NumberB:      targetNumber,
		CallStart:    smsRecord.SentAt,
		CallEnd:      &now,
		Status:       models.StatusSent,
		ScenarioName: scenarioName,
		StepIndex:    stepIdx,
		SMSRecord:    smsRecord,
	}, nil
}

func makeFailedRecord(fromModem, numberA, toModem, numberB, scenario string, step int, status string) models.CallRecord {
	now := time.Now()
	dur := 0.0
	return models.CallRecord{
		ID:               uuid.New().String(),
		Direction:        "outgoing",
		FromModem:        fromModem,
		NumberA:          numberA,
		ToModem:          toModem,
		NumberB:          numberB,
		CallStart:        now,
		CallEnd:          &now,
		TotalDurationSec: &dur,
		Status:           models.CallStatus(status),
		ScenarioName:     scenario,
		StepIndex:        step,
	}
}

// executeInternetConnect — подключение интернета
func (e *Engine) executeInternetConnect(modemName, apn, user, password, scenarioName string, stepIdx int) (models.CallRecord, error) {
	cfg, err := e.manager.Config(modemName)
	if err != nil {
		return models.CallRecord{}, err
	}

	modemCtrl, err := e.manager.Get(modemName)
	if err != nil {
		return models.CallRecord{}, err
	}

	// Используем APN из шага или из конфига
	if apn == "" {
		apn = cfg.APN
		user = cfg.APNUser
		password = cfg.APNPassword
	}

	internetRecord := &models.InternetRecord{
		ID:        uuid.New().String(),
		Modem:     modemName,
		Action:    "connect",
		APN:       apn,
		Timestamp: time.Now(),
		Status:    models.StatusInitiated,
	}

	log.Printf("  Интернет: подключение %s (APN: %s)", modemName, apn)

	err = modemCtrl.SetupInternet(apn, user, password)
	if err != nil {
		internetRecord.Status = models.StatusFailed
		internetRecord.Error = err.Error()

		now := time.Now()
		return models.CallRecord{
			ID:             internetRecord.ID,
			FromModem:      modemName,
			NumberA:        cfg.PhoneNumber,
			CallStart:      internetRecord.Timestamp,
			CallEnd:        &now,
			Status:         models.StatusFailed,
			ScenarioName:   scenarioName,
			StepIndex:      stepIdx,
			InternetRecord: internetRecord,
		}, err
	}

	// Проверяем статус и получаем IP
	connected, ip, _ := modemCtrl.CheckInternetStatus()
	internetRecord.IPAddress = ip
	internetRecord.Status = models.StatusConnected

	if !connected {
		internetRecord.Status = models.StatusFailed
		internetRecord.Error = "не удалось получить IP адрес"
	}

	log.Printf("  Интернет подключён, IP: %s", ip)

	now := time.Now()
	return models.CallRecord{
		ID:             internetRecord.ID,
		FromModem:      modemName,
		NumberA:        cfg.PhoneNumber,
		CallStart:      internetRecord.Timestamp,
		CallEnd:        &now,
		Status:         internetRecord.Status,
		ScenarioName:   scenarioName,
		StepIndex:      stepIdx,
		InternetRecord: internetRecord,
	}, nil
}

// executeInternetDisconnect — отключение интернета
func (e *Engine) executeInternetDisconnect(modemName, scenarioName string, stepIdx int) (models.CallRecord, error) {
	cfg, err := e.manager.Config(modemName)
	if err != nil {
		return models.CallRecord{}, err
	}

	modemCtrl, err := e.manager.Get(modemName)
	if err != nil {
		return models.CallRecord{}, err
	}

	internetRecord := &models.InternetRecord{
		ID:        uuid.New().String(),
		Modem:     modemName,
		Action:    "disconnect",
		Timestamp: time.Now(),
		Status:    models.StatusInitiated,
	}

	log.Printf("  Интернет: отключение %s", modemName)

	err = modemCtrl.DisconnectInternet()
	if err != nil {
		internetRecord.Status = models.StatusFailed
		internetRecord.Error = err.Error()

		now := time.Now()
		return models.CallRecord{
			ID:             internetRecord.ID,
			FromModem:      modemName,
			NumberA:        cfg.PhoneNumber,
			CallStart:      internetRecord.Timestamp,
			CallEnd:        &now,
			Status:         models.StatusFailed,
			ScenarioName:   scenarioName,
			StepIndex:      stepIdx,
			InternetRecord: internetRecord,
		}, err
	}

	internetRecord.Status = models.StatusConnected
	log.Printf("  Интернет отключён")

	now := time.Now()
	return models.CallRecord{
		ID:             internetRecord.ID,
		FromModem:      modemName,
		NumberA:        cfg.PhoneNumber,
		CallStart:      internetRecord.Timestamp,
		CallEnd:        &now,
		Status:         models.StatusConnected,
		ScenarioName:   scenarioName,
		StepIndex:      stepIdx,
		InternetRecord: internetRecord,
	}, nil
}

func ptrFloat(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}
