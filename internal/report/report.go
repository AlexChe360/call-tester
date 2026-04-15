package report

import (
	"call-tester/internal/models"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// SaveJSON сохраняет полный отчёт в JSON
func SaveJSON(report *models.ScenarioReport, outputDir string) (string, error) {
	os.MkdirAll(outputDir, 0755)

	filename := filepath.Join(outputDir, fmt.Sprintf("report_%s_%s.json",
		sanitize(report.ScenarioName),
		report.ExecutedAt.Format("20060102_150405")))

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return "", err
	}

	log.Printf("JSON-отчёт: %s", filename)
	return filename, nil
}

// SaveCSV сохраняет CDR в CSV для сверки с биллингом
func SaveCSV(report *models.ScenarioReport, outputDir string) (string, error) {
	os.MkdirAll(outputDir, 0755)

	filename := filepath.Join(outputDir, fmt.Sprintf("cdr_%s_%s.csv",
		sanitize(report.ScenarioName),
		report.ExecutedAt.Format("20060102_150405")))

	f, err := os.Create(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// BOM для корректного отображения UTF-8 в Excel
	f.Write([]byte{0xEF, 0xBB, 0xBF})

	w := csv.NewWriter(f)
	defer w.Flush()

	// Заголовки
	w.Write([]string{
		"ID", "Сценарий", "Шаг",
		"Модем_А", "Номер_А",
		"Модем_Б", "Номер_Б",
		"Направление",
		"Дата_начала", "Время_начала", "Время_ответа", "Время_конца",
		"Длительность_разговора_сек", "Полная_длительность_сек",
		"Статус",
	})

	for _, r := range report.Records {
		answerTime := ""
		if r.AnswerTime != nil {
			answerTime = r.AnswerTime.Format("15:04:05.000")
		}
		endTime := ""
		if r.CallEnd != nil {
			endTime = r.CallEnd.Format("15:04:05.000")
		}
		talkDur := ""
		if r.TalkDurationSec != nil {
			talkDur = fmt.Sprintf("%.1f", *r.TalkDurationSec)
		}
		totalDur := ""
		if r.TotalDurationSec != nil {
			totalDur = fmt.Sprintf("%.1f", *r.TotalDurationSec)
		}

		w.Write([]string{
			r.ID,
			r.ScenarioName,
			fmt.Sprintf("%d", r.StepIndex),
			r.FromModem,
			r.NumberA,
			r.ToModem,
			r.NumberB,
			r.Direction,
			r.CallStart.Format("2006-01-02"),
			r.CallStart.Format("15:04:05.000"),
			answerTime,
			endTime,
			talkDur,
			totalDur,
			string(r.Status),
		})
	}

	log.Printf("CSV-отчёт (CDR): %s", filename)
	return filename, nil
}

// PrintSummary выводит сводку в консоль
// В report.go, в функции PrintSummary
func PrintSummary(report *models.ScenarioReport) {
	fmt.Println("\n╔══════════════════════════════════════════════════════════╗")
	fmt.Printf("║  Отчёт: %-50s ║\n", report.ScenarioName)
	fmt.Printf("║  Выполнен: %-50s ║\n", report.ExecutedAt.Format("2006-01-02 15:04:05"))
	fmt.Println("╠══════════════════════════════════════════════════════════╣")
	fmt.Printf("║  Всего операций: %-41d ║\n", report.TotalCalls)
	fmt.Printf("║  Успешных:       %-41d ║\n", report.SuccessfulCalls)
	fmt.Printf("║  Неудачных:      %-41d ║\n", report.FailedCalls)
	fmt.Println("╠══════════════════════════════════════════════════════════╣")

	// Показываем детали по каждой операции
	for _, r := range report.Records {
		statusChar := "✓"
		if r.Status != models.StatusAnswered &&
			r.Status != models.StatusSent &&
			r.Status != models.StatusConnected {
			statusChar = "✗"
		}

		if r.SMSRecord != nil {
			fmt.Printf("║  %s %s -> %s | %s | -%36s ║\n",
				statusChar, r.NumberA, r.NumberB, r.Status, "")
		} else if r.InternetRecord != nil {
			fmt.Printf("║  %s %s -> интернет | %s | -%31s ║\n",
				statusChar, r.NumberA, r.Status, "")
		} else {
			duration := ""
			if r.TalkDurationSec != nil {
				duration = fmt.Sprintf("%.1fс", *r.TalkDurationSec)
			}
			fmt.Printf("║  %s %s -> %s | %s | %-23s ║\n",
				statusChar, r.NumberA, r.NumberB, r.Status, duration)
		}
	}
	fmt.Println("╚══════════════════════════════════════════════════════════╝")
}

func sanitize(s string) string {
	return strings.ReplaceAll(s, " ", "_")
}
