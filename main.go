package main

import (
	"clickhouse-platform/internal/api"
	"clickhouse-platform/internal/chaos"
	"clickhouse-platform/internal/model"
	"clickhouse-platform/internal/monitor"
	"clickhouse-platform/internal/provisioner"
	"clickhouse-platform/internal/store"
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// 1. Инициализация активных узлов кластера
	initialNodes := []*model.Node{
		{ID: "keeper", Role: "KEEPER", Host: "127.0.0.1", Port: 9181},
		{ID: "node1", Role: "REPLICA", Host: "127.0.0.1", Port: 9000},
		{ID: "node2", Role: "REPLICA", Host: "127.0.0.1", Port: 9001},
	}

	clusterStore := store.NewClusterStore(initialNodes)
	chaosEngine := chaos.NewEngine(clusterStore)
	bootstrapper := provisioner.NewBootstrapper()

	// 2. Фоновый чекер топологии на горутинах
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := monitor.NewWorker(clusterStore, 1*time.Second)
	go worker.Start(ctx)

	// 3. Запуск HTTP Control Plane
	server := api.NewServer(clusterStore, chaosEngine, bootstrapper)
	httpServer := &http.Server{
		Addr:    ":8080",
		Handler: server,
	}

	go func() {
		log.Println("Control Plane запущен на http://localhost:8080")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Ошибка сервера: %v", err)
		}
	}()

	// 4. Graceful Shutdown (перехват сигналов завершения ОС)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("Получен сигнал завершения, остановка сервиса...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer shutdownCancel()
	httpServer.Shutdown(shutdownCtx)

	log.Println("Сервис успешно остановлен")
}
