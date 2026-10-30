package store

import (
	"clickhouse-platform/internal/model"
	"fmt"
	"sync"
)

// ClusterStore предоставляет потокобезопасное in-memory хранилище топологии кластера.
type ClusterStore struct {
	mu    sync.RWMutex
	nodes []*model.Node
}

// NewClusterStore инициализирует хранилище начальным списком узлов.
func NewClusterStore(initialNodes []*model.Node) *ClusterStore {
	return &ClusterStore{
		nodes: initialNodes,
	}
}

// GetAllNodes возвращает копию среза узлов под блокировкой на чтение (RLock).
func (s *ClusterStore) GetAllNodes() []*model.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return append([]*model.Node(nil), s.nodes...)
}

// AddNode безопасно добавляет новый узел с проверкой уникальности идентификатора.
func (s *ClusterStore) AddNode(node *model.Node) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, existingNode := range s.nodes {
		if existingNode.ID == node.ID {
			return fmt.Errorf("узел с ID '%s' уже зарегистрирован в кластере", node.ID)
		}
	}

	s.nodes = append(s.nodes, node)
	return nil
}

// SetNodeAlert выставляет аварийное сообщение на карточке узла и меняет статус на WARNING.
func (s *ClusterStore) SetNodeAlert(nodeID string, alertText string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, n := range s.nodes {
		if n.ID == nodeID {
			n.ActiveAlert = alertText
			if alertText != "" {
				n.Status = model.StatusWarning
			} else {
				n.Status = model.StatusHealthy
			}
			break
		}
	}
}

// RemoveNode удаляет узел из топологии по ID (с защитой координатора Keeper).
func (s *ClusterStore) RemoveNode(nodeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if nodeID == "keeper" {
		return fmt.Errorf("нельзя удалить координатор кластера (Keeper)")
	}

	for i, n := range s.nodes {
		if n.ID == nodeID {
			s.nodes = append(s.nodes[:i], s.nodes[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("узел '%s' не найден в топологии", nodeID)
}
