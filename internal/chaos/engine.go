package chaos

import (
	"clickhouse-platform/internal/store"
	"os/exec"
	"runtime"
	"sync"
)

// Engine управляет инъекцией инфраструктурных сбоев (Chaos Engineering) в кластер ClickHouse.
type Engine struct {
	store *store.ClusterStore
	mu    sync.Mutex
}

// NewEngine создает экземпляр движка хаоса со ссылкой на общее хранилище топологии.
func NewEngine(s *store.ClusterStore) *Engine {
	return &Engine{store: s}
}

// runLinuxCommand умно определяет окружение выполнения:
// - в Docker или чистом Linux: выполняет команду ОС напрямую;
// - при разработке под Windows: прозрачно проксирует вызов через WSL.
func runLinuxCommand(cmdName string, args ...string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		wslArgs := append([]string{"-u", "root", cmdName}, args...)
		return exec.Command("wsl", wslArgs...)
	}
	return exec.Command(cmdName, args...)
}

// KillReplica2 инициирует жесткое падение процесса второй реплики (SIGKILL).
func (e *Engine) KillReplica2() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	cmd := runLinuxCommand("pkill", "-9", "-f", "config-file=/etc/clickhouse-server2")
	return cmd.Run()
}

// RestoreReplica2 повторно поднимает демон второй реплики после аварии.
func (e *Engine) RestoreReplica2() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if runtime.GOOS == "windows" {
		cmd := exec.Command("wsl", "-u", "root", "sudo", "-u", "clickhouse", "/usr/local/bin/clickhouse", "server",
			"--config-file=/etc/clickhouse-server2/config.xml", "--daemon", "--pid-file=/var/lib/clickhouse2/clickhouse-server.pid")
		return cmd.Run()
	}

	cmd := exec.Command("sudo", "-u", "clickhouse", "/usr/local/bin/clickhouse", "server",
		"--config-file=/etc/clickhouse-server2/config.xml", "--daemon", "--pid-file=/var/lib/clickhouse2/clickhouse-server.pid")
	return cmd.Run()
}

// IsolateKeeper замораживает порт координатора (9181), эмулируя потерю кворума Raft и сетевой сплит.
func (e *Engine) IsolateKeeper() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	cmd := runLinuxCommand("fuser", "-k", "-STOP", "9181/tcp")
	return cmd.Run()
}

// RestoreKeeperNetwork снимает сигнал остановки и восстанавливает связность Keeper.
func (e *Engine) RestoreKeeperNetwork() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	cmd := runLinuxCommand("fuser", "-k", "-CONT", "9181/tcp")
	return cmd.Run()
}

// FreezeNode1 отправляет SIGSTOP узлу node1, моделируя зависание ввода-вывода (Gray Failure).
func (e *Engine) FreezeNode1() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	cmd := runLinuxCommand("fuser", "-k", "-STOP", "9000/tcp")
	return cmd.Run()
}

// UnfreezeNode1 размораживает сокет и потоки узла node1 (SIGCONT).
func (e *Engine) UnfreezeNode1() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	cmd := runLinuxCommand("fuser", "-k", "-CONT", "9000/tcp")
	return cmd.Run()
}
