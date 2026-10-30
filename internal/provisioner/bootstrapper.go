package provisioner

import (
	"clickhouse-platform/internal/model"
	"fmt"
	"os/exec"
	"runtime"
)

// Bootstrapper отвечает за физическое развертывание (Day-0) и демонтаж узлов ClickHouse.
type Bootstrapper struct{}

// NewBootstrapper создает экземпляр загрузчика узлов.
func NewBootstrapper() *Bootstrapper {
	return &Bootstrapper{}
}

// runLinuxCommand выполняет команды ОС нативно или через WSL при разработке в Windows.
func runLinuxCommand(cmdName string, args ...string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		wslArgs := append([]string{"-u", "root", cmdName}, args...)
		return exec.Command("wsl", wslArgs...)
	}
	return exec.Command(cmdName, args...)
}

// Provision физически подготавливает файловую систему, генерирует конфиг и запускает процесс ClickHouse.
func (b *Bootstrapper) Provision(req model.ProvisionRequest) error {
	// Динамический расчет портов во избежание коллизий
	offset := req.Port - 9000
	if offset < 0 {
		offset = 3
	}
	httpPort := 8123 + offset
	interPort := 9009 + offset
	shard := req.ShardNumber
	if shard <= 0 {
		shard = 1
	}

	configDir := fmt.Sprintf("/etc/clickhouse-%s", req.ID)
	dataDir := fmt.Sprintf("/var/lib/clickhouse-%s", req.ID)
	logDir := fmt.Sprintf("/var/log/clickhouse-server-%s", req.ID)

	// 1. Создаем директории данных и логов с правами системного пользователя clickhouse
	setupScript := fmt.Sprintf(`mkdir -p %s %s %s && chown -R clickhouse:clickhouse %s %s`,
		configDir, dataDir, logDir, dataDir, logDir)

	if err := runLinuxCommand("sh", "-c", setupScript).Run(); err != nil {
		return fmt.Errorf("ошибка создания каталогов: %w", err)
	}

	// 2. Генерируем минимальную Day-0 XML конфигурацию с макросами репликации
	configXML := fmt.Sprintf(`<clickhouse>
    <logger>
        <level>information</level>
        <log>%s/clickhouse-server.log</log>
        <errorlog>%s/clickhouse-server.err.log</errorlog>
    </logger>
    <http_port>%d</http_port>
    <tcp_port>%d</tcp_port>
    <interserver_http_port>%d</interserver_http_port>
    <listen_host>0.0.0.0</listen_host>
    <path>%s/</path>
    <tmp_path>%s/tmp/</tmp_path>
    <user_files_path>%s/user_files/</user_files_path>
    <zookeeper>
        <node>
            <host>127.0.0.1</host>
            <port>9181</port>
        </node>
    </zookeeper>
    <macros>
        <cluster>company_cluster</cluster>
        <shard>%d</shard>
        <replica>%s</replica>
    </macros>
    <users_config>/etc/clickhouse-server/users.xml</users_config>
</clickhouse>`, logDir, logDir, httpPort, req.Port, interPort, dataDir, dataDir, dataDir, shard, req.ID)

	writeScript := fmt.Sprintf("cat << 'EOF' > %s/config.xml\n%s\nEOF", configDir, configXML)
	if err := runLinuxCommand("sh", "-c", writeScript).Run(); err != nil {
		return fmt.Errorf("ошибка записи конфига: %w", err)
	}

	// 3. Запуск демона ClickHouse под пользователем clickhouse
	startCmd := fmt.Sprintf("sudo -u clickhouse /usr/local/bin/clickhouse server --config-file=%s/config.xml --daemon --pid-file=%s/clickhouse-server.pid", configDir, dataDir)
	if err := runLinuxCommand("sh", "-c", startCmd).Run(); err != nil {
		return fmt.Errorf("ошибка старта процесса: %w", err)
	}

	return nil
}

// Teardown останавливает демон ClickHouse и удаляет временные каталоги узла.
func (b *Bootstrapper) Teardown(nodeID string) error {
	configDir := fmt.Sprintf("/etc/clickhouse-%s", nodeID)
	dataDir := fmt.Sprintf("/var/lib/clickhouse-%s", nodeID)
	logDir := fmt.Sprintf("/var/log/clickhouse-server-%s", nodeID)

	stopScript := fmt.Sprintf(`pkill -9 -f config-file=%s || true && rm -rf %s %s %s`,
		configDir, configDir, dataDir, logDir)

	_ = runLinuxCommand("sh", "-c", stopScript).Run()
	return nil
}
