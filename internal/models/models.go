package models

import "time"

// ModemConfig — конфигурация одного модема
type ModemConfig struct {
	Name        string `yaml:"name" json:"name"`
	Port        string `yaml:"port" json:"port"`
	BaudRate    int    `yaml:"baud_rate" json:"baud_rate"`
	Model       string `yaml:"model" json:"model"`
	PhoneNumber string `yaml:"phone_number" json:"phone_number"`
}

// SystemConfig — конфигурация всей системы
type SystemConfig struct {
	Modems []ModemConfig `yaml:"modems" json:"modems"`
}

// ScenarioStep — один шаг сценария
type ScenarioStep struct {
	Action          string `yaml:"action" json:"action"` // "call" или "pause"
	FromModem       string `yaml:"from_modem,omitempty" json:"from_modem,omitempty"`
	ToModem         string `yaml:"to_modem,omitempty" json:"to_modem,omitempty"`
	HoldDurationSec int    `yaml:"hold_duration_sec,omitempty" json:"hold_duration_sec,omitempty"`
	DurationSec     int    `yaml:"duration_sec,omitempty" json:"duration_sec,omitempty"`
}

// Scenario — сценарий тестирования
type Scenario struct {
	Name        string         `yaml:"name" json:"name"`
	Description string         `yaml:"description,omitempty" json:"description,omitempty"`
	Steps       []ScenarioStep `yaml:"steps" json:"steps"`
}

// CallStatus — статус звонка
type CallStatus string

const (
	StatusInitiated CallStatus = "initiated"
	StatusAnswered  CallStatus = "answered"
	StatusNoAnswer  CallStatus = "no_answer"
	StatusBusy      CallStatus = "busy"
	StatusFailed    CallStatus = "failed"
)

// CallRecord — запись одного звонка (CDR)
type CallRecord struct {
	ID               string     `json:"id"`
	Direction        string     `json:"direction"`
	FromModem        string     `json:"from_modem"`
	NumberA          string     `json:"number_a"`
	ToModem          string     `json:"to_modem"`
	NumberB          string     `json:"number_b"`
	CallStart        time.Time  `json:"call_start"`
	AnswerTime       *time.Time `json:"answer_time,omitempty"`
	CallEnd          *time.Time `json:"call_end,omitempty"`
	TalkDurationSec  *float64   `json:"talk_duration_sec,omitempty"`
	TotalDurationSec *float64   `json:"total_duration_sec,omitempty"`
	Status           CallStatus `json:"status"`
	ScenarioName     string     `json:"scenario_name"`
	StepIndex        int        `json:"step_index"`
}

// ScenarioReport — итоговый отчёт
type ScenarioReport struct {
	ScenarioName    string       `json:"scenario_name"`
	ExecutedAt      time.Time    `json:"executed_at"`
	TotalCalls      int          `json:"total_calls"`
	SuccessfulCalls int          `json:"successful_calls"`
	FailedCalls     int          `json:"failed_calls"`
	Records         []CallRecord `json:"records"`
}