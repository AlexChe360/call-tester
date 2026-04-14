package models

import "time"

// ModemConfig — конфигурация одного модема
type ModemConfig struct {
	Name        string `yaml:"name" json:"name"`
	Port        string `yaml:"port" json:"port"`
	BaudRate    int    `yaml:"baud_rate" json:"baud_rate"`
	Model       string `yaml:"model" json:"model"`
	PhoneNumber string `yaml:"phone_number" json:"phone_number"`
	APN         string `yaml:"apn,omitempty" json:"apn,omitempty"`
	APNUser     string `yaml:"apn_user,omitempty" json:"apn_user,omitempty"`
	APNPassword string `yaml:"apn_password,omitempty" json:"apn_password,omitempty"`
}

// SystemConfig — конфигурация всей системы
type SystemConfig struct {
	Modems []ModemConfig `yaml:"modems" json:"modems"`
}

// ScenarioStep — один шаг сценария
type ScenarioStep struct {
	Action          string `yaml:"action" json:"action"` // "call", "pause", "sms", "internet_on", "intermet_off"
	FromModem       string `yaml:"from_modem,omitempty" json:"from_modem,omitempty"`
	ToModem         string `yaml:"to_modem,omitempty" json:"to_modem,omitempty"`
	ToNumber        string `yaml:"to_number,omitempty" json:"to_number,omitempty"`
	HoldDurationSec int    `yaml:"hold_duration_sec,omitempty" json:"hold_duration_sec,omitempty"`
	DurationSec     int    `yaml:"duration_sec,omitempty" json:"duration_sec,omitempty"`
	Message         string `yaml:"message,omitempty" json:"mesage,omitempty"`
	APN             string `yaml:"apn,omitempty" json:"apn,omitempty"`
	APNUser         string `yaml:"apn_user,omitempty" json:"apn_user,omitempty"`
	APNPassword     string `yaml:"apn_password,omitempty" json:"apn_password,omitempty"`
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
	StatusSent      CallStatus = "sent"
	StatusConnected CallStatus = "connected"
)

// SMSRecord — запись об SMS
type SMSRecord struct {
	ID         string     `json:"id"`
	FromModem  string     `json:"from_modem"`
	FromNumber string     `json:"from_number"`
	ToNumber   string     `json:"to_number"`
	ToModem    string     `json:"to_modem,omitempty"`
	Message    string     `json:"message"`
	SentAt     time.Time  `json:"sent_at"`
	Status     CallStatus `json:"status"`
	Error      string     `json:"error,omitempty"`
}

// InternetRecord — запись о подключении интернета
type InternetRecord struct {
	ID        string     `json:"id"`
	Modem     string     `json:"modem"`
	Action    string     `json:"action"`
	APN       string     `json:"apn"`
	IPAddress string     `json:"ip_address"`
	Timestamp time.Time  `json:"timestamp"`
	Status    CallStatus `json:"status"`
	Error     string     `json:"error,omitempty"`
}

// CallRecord — запись одного звонка (CDR)
type CallRecord struct {
	ID               string          `json:"id"`
	Direction        string          `json:"direction"`
	FromModem        string          `json:"from_modem"`
	NumberA          string          `json:"number_a"`
	ToModem          string          `json:"to_modem"`
	NumberB          string          `json:"number_b"`
	CallStart        time.Time       `json:"call_start"`
	AnswerTime       *time.Time      `json:"answer_time,omitempty"`
	CallEnd          *time.Time      `json:"call_end,omitempty"`
	TalkDurationSec  *float64        `json:"talk_duration_sec,omitempty"`
	TotalDurationSec *float64        `json:"total_duration_sec,omitempty"`
	Status           CallStatus      `json:"status"`
	ScenarioName     string          `json:"scenario_name"`
	StepIndex        int             `json:"step_index"`
	SMSRecord        *SMSRecord      `json:"sms_record,omitempty"`
	InternetRecord   *InternetRecord `json:"internet_record,omitempty"`
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
