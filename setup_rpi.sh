#!/bin/bash
# ============================================
# Настройка Raspberry Pi 5 для call-tester (Go)
# ============================================

set -e

echo "=== Проверка USB-модемов ==="
echo ""

echo "USB-устройства (модемы):"
lsusb | grep -iE "quectel|simcom|1e0e|2c7c" || echo "  (не найдены — подключите модемы)"
echo ""

echo "Последовательные порты:"
ls -la /dev/ttyUSB* 2>/dev/null || echo "  (ttyUSB не найдены)"
echo ""

echo "=== Тестируем AT-порты ==="
for port in /dev/ttyUSB*; do
    if [ -e "$port" ]; then
        result=$(timeout 2 bash -c "echo -e 'AT\r' > $port && cat $port" 2>/dev/null | tr -d '\r\n' || true)
        if echo "$result" | grep -q "OK"; then
            vendor=$(udevadm info -a "$port" 2>/dev/null | grep 'ATTRS{idVendor}' | head -1 | awk -F'"' '{print $2}')
            product=$(udevadm info -a "$port" 2>/dev/null | grep 'ATTRS{idProduct}' | head -1 | awk -F'"' '{print $2}')

            model="unknown"
            case "$vendor:$product" in
                "1e0e:9001") model="SIM7600E-L1C" ;;
                "2c7c:0125") model="Quectel EC25"  ;;
                "2c7c:0306") model="Quectel EP06"  ;;
            esac

            echo "  ✓ $port -> $model (AT-порт)"
        fi
    fi
done
echo ""

echo "=== Права доступа ==="
if groups | grep -q dialout; then
    echo "  ✓ Пользователь в группе dialout"
else
    echo "  ✗ Добавьте себя: sudo usermod -aG dialout \$USER"
    echo "    Затем перелогиньтесь"
fi
echo ""

echo "=== Готово ==="
echo "Отредактируйте config.yaml, впишите порты и номера SIM-карт."