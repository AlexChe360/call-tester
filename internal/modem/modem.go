package modem

import (
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"go.bug.st/serial"
)

const (
	defaultReadTimeout  = 5 * time.Second
	callSetupTimeout    = 60 * time.Second
	incomingCallTimeout = 120 * time.Second
)

// Controller — управление одним модемом
type Controller struct {
	port     serial.Port
	portName string
	Name     string
	Phone    string
	Model    string
}

// New открывает serial-порт и создаёт контроллер
func New(portName string, baudRate int, name, phone, model string) (*Controller, error) {
	mode := &serial.Mode{
		BaudRate: baudRate,
		DataBits: 8,
		StopBits: serial.OneStopBit,
		Parity:   serial.NoParity,
	}

	port, err := serial.Open(portName, mode)
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть %s: %w", portName, err)
	}

	// Таймаут чтения для polling
	port.SetReadTimeout(100 * time.Millisecond)

	log.Printf("[%s] Порт %s открыт (baud: %d)", name, portName, baudRate)

	return &Controller{
		port:     port,
		portName: portName,
		Name:     name,
		Phone:    phone,
		Model:    model,
	}, nil
}

// Close закрывает порт
func (c *Controller) Close() error {
	return c.port.Close()
}

// ---- Низкоуровневые AT-команды ----

// sendCommand отправляет AT-команду и читает ответ
func (c *Controller) sendCommand(cmd string, timeout time.Duration) (string, error) {
	// Очищаем входной буфер
	c.flushInput()

	cmdLine := cmd + "\r"
	log.Printf("[%s] >>> %s", c.Name, cmd)

	_, err := c.port.Write([]byte(cmdLine))
	if err != nil {
		return "", fmt.Errorf("ошибка записи в %s: %w", c.portName, err)
	}

	resp, err := c.readResponse(timeout)
	if err != nil {
		return "", err
	}

	log.Printf("[%s] <<< %s", c.Name, strings.TrimSpace(resp))
	return resp, nil
}

// sendCommandOK отправляет команду и проверяет OK
func (c *Controller) sendCommandOK(cmd string, timeout time.Duration) (string, error) {
	resp, err := c.sendCommand(cmd, timeout)
	if err != nil {
		return "", err
	}

	if strings.Contains(resp, "OK") {
		return resp, nil
	}
	if strings.Contains(resp, "ERROR") {
		return "", fmt.Errorf("команда '%s' вернула ошибку: %s", cmd, strings.TrimSpace(resp))
	}
	return "", fmt.Errorf("команда '%s' — неожиданный ответ: %s", cmd, strings.TrimSpace(resp))
}

// readResponse читает до OK/ERROR/таймаута
func (c *Controller) readResponse(timeout time.Duration) (string, error) {
	var resp strings.Builder
	buf := make([]byte, 4096)
	start := time.Now()

	for {
		if time.Since(start) > timeout {
			if resp.Len() == 0 {
				return "", fmt.Errorf("таймаут чтения из %s (%v)", c.portName, timeout)
			}
			break
		}

		n, err := c.port.Read(buf)
		if err != nil && err != io.EOF {
			// Таймаут — нормально при polling
			if n == 0 {
				time.Sleep(50 * time.Millisecond)
				continue
			}
		}

		if n > 0 {
			resp.Write(buf[:n])
			s := resp.String()

			// Проверяем финальные маркеры
			if strings.Contains(s, "OK\r") || strings.Contains(s, "OK\n") ||
				strings.Contains(s, "ERROR") ||
				strings.Contains(s, "NO CARRIER") ||
				strings.Contains(s, "BUSY") ||
				strings.Contains(s, "NO ANSWER") ||
				strings.Contains(s, "NO DIALTONE") {
				break
			}
		}
	}

	return resp.String(), nil
}

// readURC читает unsolicited result codes (RING, CLIP и т.д.)
func (c *Controller) readURC(timeout time.Duration) string {
	var resp strings.Builder
	buf := make([]byte, 4096)
	start := time.Now()

	for {
		if time.Since(start) > timeout {
			return resp.String()
		}

		n, _ := c.port.Read(buf)
		if n > 0 {
			resp.Write(buf[:n])
			s := resp.String()

			if strings.Contains(s, "RING") ||
				strings.Contains(s, "+CLIP") ||
				strings.Contains(s, "+CLCC") ||
				strings.Contains(s, "NO CARRIER") ||
				strings.Contains(s, "BUSY") {
				break
			}
		} else {
			time.Sleep(50 * time.Millisecond)
		}
	}

	return resp.String()
}

func (c *Controller) flushInput() {
	buf := make([]byte, 4096)
	for {
		n, _ := c.port.Read(buf)
		if n == 0 {
			break
		}
	}
}

// ---- Высокоуровневые операции ----

// Init инициализирует модем для голосовых вызовов
func (c *Controller) Init() error {
	log.Printf("[%s] Инициализация (%s)", c.Name, c.Model)

	// Выключаем эхо
	if _, err := c.sendCommandOK("ATE0", defaultReadTimeout); err != nil {
		return err
	}

	// Проверка связи
	if _, err := c.sendCommandOK("AT", defaultReadTimeout); err != nil {
		return err
	}

	// Проверяем регистрацию в сети
	resp, err := c.sendCommand("AT+CREG?", defaultReadTimeout)
	if err != nil {
		return err
	}
	if !strings.Contains(resp, ",1") && !strings.Contains(resp, ",5") {
		log.Printf("[%s] ВНИМАНИЕ: модем не зарегистрирован в сети: %s", c.Name, strings.TrimSpace(resp))
	} else {
		log.Printf("[%s] Зарегистрирован в сети", c.Name)
	}

	// Включаем определитель номера
	if _, err := c.sendCommandOK("AT+CLIP=1", defaultReadTimeout); err != nil {
		return err
	}

	// Расширенные ошибки
	if _, err := c.sendCommandOK("AT+CMEE=2", defaultReadTimeout); err != nil {
		return err
	}

	log.Printf("[%s] Инициализирован", c.Name)
	return nil
}

// GetSignalQuality возвращает RSSI и BER
func (c *Controller) GetSignalQuality() (rssi int, ber int, err error) {
	resp, err := c.sendCommand("AT+CSQ", defaultReadTimeout)
	if err != nil {
		return 0, 0, err
	}

	for _, line := range strings.Split(resp, "\n") {
		if strings.Contains(line, "+CSQ:") {
			parts := strings.Split(strings.TrimSpace(strings.Split(line, ":")[1]), ",")
			if len(parts) >= 2 {
				fmt.Sscanf(parts[0], "%d", &rssi)
				fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &ber)
				return rssi, ber, nil
			}
		}
	}

	return 0, 0, fmt.Errorf("не удалось распарсить CSQ")
}

// SignalDBm конвертирует RSSI в dBm
func SignalDBm(rssi int) string {
	if rssi == 99 {
		return "нет сигнала"
	}
	return fmt.Sprintf("%d dBm", -113+rssi*2)
}

// Dial набирает номер. Возвращает true если соединение установлено.
func (c *Controller) Dial(number string) (bool, error) {
	log.Printf("[%s] Набираем: %s", c.Name, number)

	cmd := fmt.Sprintf("ATD%s;", number) // ; = голосовой вызов
	if _, err := c.sendCommand(cmd, defaultReadTimeout); err != nil {
		// Некоторые модемы не дают OK сразу — это нормально
		log.Printf("[%s] Команда ATD: %v (продолжаем)", c.Name, err)
	}

	// Ждём установления соединения
	start := time.Now()
	for {
		if time.Since(start) > callSetupTimeout {
			log.Printf("[%s] Таймаут установления вызова", c.Name)
			c.Hangup()
			return false, nil
		}

		urc := c.readURC(2 * time.Second)

		if strings.Contains(urc, "NO CARRIER") ||
			strings.Contains(urc, "BUSY") ||
			strings.Contains(urc, "NO ANSWER") ||
			strings.Contains(urc, "NO DIALTONE") {
			log.Printf("[%s] Вызов не состоялся: %s", c.Name, strings.TrimSpace(urc))
			return false, nil
		}

		// Проверяем статус через CLCC
		clcc, err := c.sendCommand("AT+CLCC", defaultReadTimeout)
		if err == nil && strings.Contains(clcc, ",0,0,") {
			// stat=0 означает active call
			log.Printf("[%s] Вызов установлен!", c.Name)
			return true, nil
		}

		time.Sleep(500 * time.Millisecond)
	}
}

// WaitAndAnswer ждёт входящий звонок и отвечает на него.
// Возвращает номер звонящего или "" если таймаут.
func (c *Controller) WaitAndAnswer(timeout time.Duration) (callerNumber string, answered bool, err error) {
	log.Printf("[%s] Ожидаем входящий (таймаут: %v)...", c.Name, timeout)

	start := time.Now()
	for {
		if time.Since(start) > timeout {
			return "", false, nil
		}

		urc := c.readURC(2 * time.Second)

		if strings.Contains(urc, "RING") {
			log.Printf("[%s] RING!", c.Name)

			// Извлекаем номер из +CLIP
			callerNumber = parseCLIP(urc)

			// Отвечаем
			time.Sleep(500 * time.Millisecond)
			resp, err := c.sendCommand("ATA", defaultReadTimeout)
			if err != nil {
				return callerNumber, false, fmt.Errorf("ошибка ATA: %w", err)
			}

			if strings.Contains(resp, "OK") || strings.Contains(resp, "CONNECT") {
				log.Printf("[%s] Ответили на звонок от %s", c.Name, callerNumber)
				return callerNumber, true, nil
			}
		}
	}
}

// Hangup завершает вызов
func (c *Controller) Hangup() {
	log.Printf("[%s] Завершаем вызов", c.Name)
	c.sendCommand("ATH", defaultReadTimeout)
	c.sendCommand("AT+CHUP", defaultReadTimeout)
}

// IsCallActive проверяет наличие активного вызова
func (c *Controller) IsCallActive() bool {
	resp, err := c.sendCommand("AT+CLCC", defaultReadTimeout)
	if err != nil {
		return false
	}
	return strings.Contains(resp, "+CLCC:")
}

// parseCLIP извлекает номер из строки +CLIP: "+77001234567",145,...
func parseCLIP(urc string) string {
	for _, line := range strings.Split(urc, "\n") {
		if !strings.Contains(line, "+CLIP:") {
			continue
		}
		start := strings.Index(line, "\"")
		if start < 0 {
			continue
		}
		rest := line[start+1:]
		end := strings.Index(rest, "\"")
		if end < 0 {
			continue
		}
		return rest[:end]
	}
	return "unknown"
}