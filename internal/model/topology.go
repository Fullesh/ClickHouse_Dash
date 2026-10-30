package model

// NodeStatus отражает текущее состояние доступности узла.
type NodeStatus string

const (
	StatusHealthy NodeStatus = "HEALTHY" // Узел отвечает на протокольные запросы
	StatusWarning NodeStatus = "WARNING" // Деградация: серый отказ, задержка или инцидент
	StatusDead    NodeStatus = "DEAD"    // Соединение разорвано или процесс завершен
)

// Node представляет сущность узла кластера ClickHouse (Keeper или Replica).
type Node struct {
	ID          string     `json:"id"`
	Role        string     `json:"role"`
	Host        string     `json:"host"`
	Port        int        `json:"port"`
	Status      NodeStatus `json:"status"`
	Latency     float64    `json:"latency"`
	QueueSize   int        `json:"queueSize"`
	LastError   string     `json:"lastError"`
	ActiveAlert string     `json:"activeAlert"`
}

// ProvisionRequest описывает входящие параметры для динамического развертывания новой ноды.
type ProvisionRequest struct {
	ID          string `json:"id"`
	Role        string `json:"role"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	ShardNumber int    `json:"shardNumber"`
}
