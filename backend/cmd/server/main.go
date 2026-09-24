package main

import (
	"log"

	"github.com/example/cs-pulse/backend/internal/config"
	"github.com/example/cs-pulse/backend/internal/httpapi"
	"github.com/example/cs-pulse/backend/internal/repository"
	"github.com/example/cs-pulse/backend/internal/service"
	"github.com/joho/godotenv"
)

func main() {
	// 既支持在项目根目录执行，也支持在 backend 目录直接启动。
	_ = godotenv.Load("../.env", ".env")
	cfg := config.Load()
	db, err := repository.Open(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}

	repo := repository.New(db)
	if err := repo.MigrateAndSeed(); err != nil {
		log.Fatalf("database setup failed: %v", err)
	}

	syncer := service.NewSyncService(repo, cfg)
	// Steam 官方新闻会在启动时同步；HLTV 只有显式启用后才会触发。
	go syncer.SyncOnce()
	go syncer.Run()

	router := httpapi.NewRouter(repo, syncer, cfg)
	log.Printf("LIFE TV API listening on :%s", cfg.Port)
	if err := router.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
