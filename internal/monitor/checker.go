package monitor

import (
	"clickhouse-platform/internal/model"
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// CheckNode выполняет протокольную проверку доступности узла в зависимости от его роли.
func CheckNode(node *model.Node) {
	start := time.Now()
	address := fmt.Sprintf("%s:%d", node.Host, node.Port)

	// 1. Для ClickHouse Keeper: 4-буквенная команда "ruok" (Are You OK?)
	if node.Role == "KEEPER" {
		conn, err := net.DialTimeout("tcp", address, 1*time.Second)
		if err != nil {
			node.Status = model.StatusDead
			node.LastError = err.Error()
			node.Latency = 0
			return
		}
		defer conn.Close()

		conn.SetDeadline(time.Now().Add(500 * time.Millisecond))
		_, err = conn.Write([]byte("ruok"))
		if err != nil {
			node.Status = model.StatusDead
			node.LastError = "Keeper не принимает команды: " + err.Error()
			node.Latency = 0
			return
		}
		reply := make([]byte, 4)
		_, err = conn.Read(reply)
		if err != nil || string(reply) != "imok" {
			node.Status = model.StatusDead
			node.LastError = "Keeper не ответил 'imok'"
			node.Latency = 0
			return
		}
		node.Latency = float64(time.Since(start).Microseconds()) / 1000.0
		node.Status = model.StatusHealthy
		node.LastError = ""
		return
	}

	// 2. Для узлов данных ClickHouse: нативный бинарный TCP Ping
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()

	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr:        []string{address},
		DialTimeout: 400 * time.Millisecond,
	})
	if err != nil {
		node.Status = model.StatusDead
		node.LastError = err.Error()
		node.Latency = 0
		return
	}
	defer conn.Close()

	if pingErr := conn.Ping(ctx); pingErr != nil {
		errStr := pingErr.Error()
		// При зависании процесса (SIGSTOP) ядро держит сокет в backlog, но клиент ловит i/o timeout
		if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline") {
			node.Status = model.StatusWarning
			node.LastError = "Таймаут I/O: процесс завис (Gray Failure)"
			node.Latency = float64(time.Since(start).Microseconds()) / 1000.0
		} else {
			// При жестком завершении процесса (SIGKILL) ОС сразу возвращает connection refused
			node.Status = model.StatusDead
			node.LastError = "Узел недоступен: " + pingErr.Error()
			node.Latency = 0
		}
		return
	}

	node.Latency = float64(time.Since(start).Microseconds()) / 1000.0
	node.Status = model.StatusHealthy
	node.LastError = ""
}
