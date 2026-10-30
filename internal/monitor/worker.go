package monitor

import (
	"clickhouse-platform/internal/model"
	"clickhouse-platform/internal/store"
	"context"
	"sync"
	"time"
)

// Worker выполняет периодический параллельный опрос состояния узлов кластера.
type Worker struct {
	store    *store.ClusterStore
	interval time.Duration
}

// NewWorker создает экземпляр воркера с заданным интервалом опроса.
func NewWorker(s *store.ClusterStore, interval time.Duration) *Worker {
	return &Worker{
		store:    s,
		interval: interval,
	}
}

// CheckAll опрашивает все зарегистрированные узлы параллельно на пуле горутин с sync.WaitGroup.
func (w *Worker) CheckAll() {
	nodes := w.store.GetAllNodes()
	var wg sync.WaitGroup

	for _, node := range nodes {
		wg.Add(1)
		go func(n *model.Node) {
			defer wg.Done()
			CheckNode(n)
		}(node)
	}
	wg.Wait()
}

// Start запускает цикл фонового мониторинга с поддержкой отмены через context.Context.
func (w *Worker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	w.CheckAll()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.CheckAll()
		}
	}
}
