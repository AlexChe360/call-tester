package modem

import (
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
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
	mu       sync.Mutex
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

// SendSMS отправляет SMS сообщение на любой номер
func (c *Controller) SendSMS(phoneNumber, message string) error {
	log.Printf("[%s] Отправка SMS на %s: \"%s\"", c.Name, phoneNumber, message)

	c.mu.Lock()
	defer c.mu.Unlock()

	// 1. Устанавливаем текстовый режим и кодировку GSM
	if _, err := c.sendCommandOK("AT+CMGF=1", 2*time.Second); err != nil {
		return fmt.Errorf("ошибка установки текстового режима: %w", err)
	}

	// 2. Устанавливаем кодировку символов (GSM 7-bit)
	if _, err := c.sendCommandOK("AT+CSCS=\"GSM\"", 2*time.Second); err != nil {
		log.Printf("[%s] Предупреждение: не удалось установить кодировку GSM: %v", c.Name, err)
	}

	// 3. Указываем номер получателя (убедимся, что номер в правильном формате)
	// Номер должен быть в международном формате с +
	if !strings.HasPrefix(phoneNumber, "+") {
		phoneNumber = "+" + phoneNumber
	}

	cmd := fmt.Sprintf("AT+CMGS=\"%s\"", phoneNumber)
	log.Printf("[%s] Отправка команды: %s", c.Name, cmd)

	if _, err := c.sendCommand(cmd, 5*time.Second); err != nil {
		// Ждём приглашение ">"
		time.Sleep(500 * time.Millisecond)
	}

	// 4. Отправляем текст сообщения + Ctrl+Z (0x1A)
	// Обрезаем сообщение если слишком длинное (160 символов для GSM 7-bit)
	if len(message) > 160 {
		message = message[:157] + "..."
		log.Printf("[%s] Сообщение обрезано до 160 символов", c.Name)
	}

	data := []byte(message + string(rune(26)))
	n, err := c.port.Write(data)
	if err != nil {
		return fmt.Errorf("ошибка отправки текста SMS: %w", err)
	}
	log.Printf("[%s] Отправлено %d байт SMS", c.Name, n)

	// 5. Ждём подтверждения с увеличенным таймаутом
	resp, err := c.readResponse(45 * time.Second)
	if err != nil {
		return fmt.Errorf("ошибка ожидания подтверждения SMS: %w", err)
	}

	log.Printf("[%s] Ответ после отправки SMS: %s", c.Name, strings.TrimSpace(resp))

	if strings.Contains(resp, "+CMGS:") {
		log.Printf("[%s] SMS успешно отправлена", c.Name)
		return nil
	}

	if strings.Contains(resp, "ERROR") || strings.Contains(resp, "+CMS ERROR") {
		// Извлекаем код ошибки
		errorCode := "unknown"
		if strings.Contains(resp, "+CMS ERROR:") {
			parts := strings.Split(resp, ":")
			if len(parts) > 1 {
				errorCode = strings.TrimSpace(parts[1])
			}
		}
		return fmt.Errorf("SMS не отправлено: ошибка %s", errorCode)
	}

	return nil
}

// CheckSMSRegistration проверяет готовность модема к отправке SMS
func (c *Controller) CheckSMSRegistration() error {
	// Проверяем регистрацию в сети для SMS
	resp, err := c.sendCommand("AT+CREG?", 5*time.Second)
	if err != nil {
		return err
	}

	if !strings.Contains(resp, ",1") && !strings.Contains(resp, ",5") {
		return fmt.Errorf("модем не зарегистрирован в сети")
	}

	// Проверяем доступность сервиса SMS
	resp, err = c.sendCommand("AT+CSMS?", 5*time.Second)
	if err == nil && strings.Contains(resp, "+CSMS:") {
		log.Printf("[%s] SMS сервис доступен", c.Name)
	}

	return nil
}

// SetupInternet настраивает интернет-соединение для Quectel EC25 и SIM7600
func (c *Controller) SetupInternet(apn, user, password string) error {
	log.Printf("[%s] Настройка интернета (APN: %s)", c.Name, apn)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Проверяем регистрацию в сети
	resp, err := c.sendCommand("AT+CREG?", 5*time.Second)
	if err != nil {
		return fmt.Errorf("ошибка проверки регистрации: %w", err)
	}

	if !strings.Contains(resp, ",1") && !strings.Contains(resp, ",5") {
		return fmt.Errorf("модем не зарегистрирован в сети: %s", resp)
	}

	// Для Quectel EC25 используем команды QICSGP/QIACT
	if strings.Contains(c.Model, "Quectel") {
		// Настраиваем APN контекст 1
		cmd := fmt.Sprintf("AT+QICSGP=1,1,\"%s\",\"%s\",\"%s\",1", apn, user, password)
		if _, err := c.sendCommandOK(cmd, 5*time.Second); err != nil {
			return fmt.Errorf("ошибка настройки APN: %w", err)
		}

		// Активируем PDP контекст
		if _, err := c.sendCommandOK("AT+QIACT=1", 30*time.Second); err != nil {
			return fmt.Errorf("ошибка активации PDP контекста: %w", err)
		}

		// Получаем IP адрес
		ipResp, err := c.sendCommand("AT+QILOCIP", 5*time.Second)
		if err != nil {
			return fmt.Errorf("ошибка получения IP: %w", err)
		}

		// Парсим IP из ответа
		lines := strings.Split(ipResp, "\r\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, ".") && !strings.Contains(line, "AT+QILOCIP") && !strings.Contains(line, "OK") {
				log.Printf("[%s] Получен IP: %s", c.Name, line)
				break
			}
		}
	} else if strings.Contains(c.Model, "SIM7600") {
		// Для SIM7600 используем команды CGDCONT/CGACT
		cmd := fmt.Sprintf("AT+CGDCONT=1,\"IP\",\"%s\"", apn)
		if _, err := c.sendCommandOK(cmd, 5*time.Second); err != nil {
			return fmt.Errorf("ошибка настройки PDP контекста: %w", err)
		}

		// Активируем контекст
		if _, err := c.sendCommandOK("AT+CGACT=1,1", 30*time.Second); err != nil {
			return fmt.Errorf("ошибка активации контекста: %w", err)
		}

		// Получаем IP
		ipResp, err := c.sendCommand("AT+CGPADDR=1", 5*time.Second)
		if err != nil {
			return fmt.Errorf("ошибка получения IP: %w", err)
		}
		log.Printf("[%s] Ответ CGPADDR: %s", c.Name, ipResp)
	} else {
		return fmt.Errorf("неподдерживаемая модель модема: %s", c.Model)
	}

	log.Printf("[%s] Интернет подключён успешно", c.Name)
	return nil
}

// DisconnectInternet отключает интернет
func (c *Controller) DisconnectInternet() error {
	log.Printf("[%s] Отключение интернета", c.Name)

	c.mu.Lock()
	defer c.mu.Unlock()

	if strings.Contains(c.Model, "Quectel") {
		if _, err := c.sendCommandOK("AT+QIDEACT=1", 10*time.Second); err != nil {
			return fmt.Errorf("ошибка отключения интернета: %w", err)
		}
	} else if strings.Contains(c.Model, "SIM7600") {
		if _, err := c.sendCommandOK("AT+CGACT=0,1", 10*time.Second); err != nil {
			return fmt.Errorf("ошибка отключения интернета: %w", err)
		}
	} else {
		return fmt.Errorf("неподдерживаемая модель модема: %s", c.Model)
	}

	log.Printf("[%s] Интернет отключён", c.Name)
	return nil
}

// CheckInternetStatus проверяет статус интернет-соединения
// CheckInternetStatus проверяет статус интернет-соединения
func (c *Controller) CheckInternetStatus() (connected bool, ipAddress string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if strings.Contains(c.Model, "Quectel") {
		resp, err := c.sendCommand("AT+QIACT?", 5*time.Second)
		if err != nil {
			return false, "", err
		}

		// Парсим +QIACT: <contextID>,<context_status>,<IP_status>,<IP_address>
		// Пример: +QIACT: 1,1,1,"10.69.109.169"
		if strings.Contains(resp, "+QIACT:") {
			lines := strings.Split(resp, "\r\n")
			for _, line := range lines {
				if strings.Contains(line, "+QIACT:") {
					// Извлекаем часть после +QIACT:
					parts := strings.Split(line, ":")
					if len(parts) < 2 {
						continue
					}
					values := strings.Split(strings.TrimSpace(parts[1]), ",")
					if len(values) >= 4 {
						// context_status = values[1] (1 = активен)
						if values[1] == "1" {
							ipAddress = strings.Trim(values[3], "\"")
							return true, ipAddress, nil
						}
					}
				}
			}
		}
		return false, "", nil
	} else if strings.Contains(c.Model, "SIM7600") {
		resp, err := c.sendCommand("AT+CGACT?", 5*time.Second)
		if err != nil {
			return false, "", err
		}

		if strings.Contains(resp, "+CGACT: 1,1") {
			// Получаем IP
			ipResp, err := c.sendCommand("AT+CGPADDR=1", 5*time.Second)
			if err == nil && strings.Contains(ipResp, "+CGPADDR: 1,") {
				parts := strings.Split(ipResp, ",")
				if len(parts) >= 2 {
					ipAddress = strings.TrimSpace(parts[1])
					return true, ipAddress, nil
				}
			}
			return true, "", nil
		}
		return false, "", nil
	}

	return false, "", fmt.Errorf("неизвестная модель модема")
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
