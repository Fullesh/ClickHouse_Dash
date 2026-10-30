package api

import (
	"clickhouse-platform/internal/chaos"
	"clickhouse-platform/internal/model"
	"clickhouse-platform/internal/provisioner"
	"clickhouse-platform/internal/store"
	"encoding/json"
	"net/http"
)

// Server предоставляет HTTP REST API для веб-интерфейса и управления кластером.
type Server struct {
	store        *store.ClusterStore
	chaos        *chaos.Engine
	bootstrapper *provisioner.Bootstrapper
	mux          *http.ServeMux
}

// NewServer создает HTTP-сервер со всеми зарегистрированными маршрутами.
func NewServer(s *store.ClusterStore, c *chaos.Engine, b *provisioner.Bootstrapper) *Server {
	srv := &Server{
		store:        s,
		chaos:        c,
		bootstrapper: b,
		mux:          http.NewServeMux(),
	}
	srv.routes()
	return srv
}

func (s *Server) routes() {
	s.mux.HandleFunc("/api/v1/topology", s.handleTopology)
	s.mux.HandleFunc("/api/v1/chaos/trigger", s.HandleChaosTrigger)
	s.mux.HandleFunc("/api/v1/nodes/provision", s.handleProvision)
	s.mux.HandleFunc("/api/v1/nodes/delete", s.handleDeleteNode)

	// Статика веб-интерфейса
	fileServer := http.FileServer(http.Dir("./web"))
	s.mux.Handle("/", fileServer)
}

// handleTopology возвращает текущий срез топологии и метрик всех узлов в JSON.
func (s *Server) handleTopology(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.store.GetAllNodes())
}

// HandleChaosTrigger запускает выбранный сценарий инфраструктурного сбоя.
func (s *Server) HandleChaosTrigger(w http.ResponseWriter, r *http.Request) {
	action := r.URL.Query().Get("action")
	var err error
	switch action {
	case "isolate-keeper":
		err = s.chaos.IsolateKeeper()
	case "restore-keeper":
		err = s.chaos.RestoreKeeperNetwork()
	case "kill-node2":
		err = s.chaos.KillReplica2()
	case "restore-node2":
		err = s.chaos.RestoreReplica2()
	case "freeze-node1":
		err = s.chaos.FreezeNode1()
	case "unfreeze-node1":
		err = s.chaos.UnfreezeNode1()
	}
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{"status": "error", "message": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}

// handleProvision валидирует запрос, физически поднимает ClickHouse и регистрирует узел.
func (s *Server) handleProvision(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req model.ProvisionRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"status": "error", "message": "Некорректный JSON"})
		return
	}

	if req.ID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"status": "error", "message": "ID ноды должен быть заполнен"})
		return
	}
	if req.Host == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"status": "error", "message": "Хост ноды должен быть заполнен"})
		return
	}

	if req.Port <= 0 || req.Port > 65535 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"status": "error", "message": "Порт меньше нуля или больше 65535"})
		return
	}

	if req.Role == "" {
		req.Role = "REPLICA"
	}

	newNode := &model.Node{
		ID:     req.ID,
		Role:   req.Role,
		Host:   req.Host,
		Port:   req.Port,
		Status: model.StatusDead,
	}

	// 1. Физический подъем демона ClickHouse в ОС
	if err := s.bootstrapper.Provision(req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{"status": "error", "message": "Сбой запуска ClickHouse: " + err.Error()})
		return
	}

	// 2. Регистрация в топологии кластера
	if err := s.store.AddNode(newNode); err != nil {
		_ = s.bootstrapper.Teardown(req.ID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"status": "error", "message": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{"status": "ok", "node": newNode})
}

// handleDeleteNode останавливает демон ClickHouse и удаляет узел из топологии.
func (s *Server) handleDeleteNode(w http.ResponseWriter, r *http.Request) {
	nodeID := r.URL.Query().Get("id")
	w.Header().Set("Content-Type", "application/json")

	if nodeID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"status": "error", "message": "Параметр id обязателен"})
		return
	}

	// 1. Остановка демона и удаление каталогов
	_ = s.bootstrapper.Teardown(nodeID)

	// 2. Удаление из топологии
	if err := s.store.RemoveNode(nodeID); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"status": "error", "message": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}
