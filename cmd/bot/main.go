package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/wwsean08/action-items-bot/internal/actionitems"
	"github.com/wwsean08/action-items-bot/internal/config"
	"github.com/wwsean08/action-items-bot/internal/discord"
	"github.com/wwsean08/action-items-bot/internal/health"
	"github.com/wwsean08/action-items-bot/internal/store/postgres"
)

func main() {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	if err := postgres.Migrate(cfg.DatabaseURL); err != nil {
		log.Fatalf("running migrations: %v", err)
	}

	ctx := context.Background()
	repo, err := postgres.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connecting to database: %v", err)
	}
	defer repo.Close()

	service := actionitems.NewService(repo)

	bot, err := discord.New(cfg.DiscordToken, service)
	if err != nil {
		log.Fatalf("creating bot: %v", err)
	}

	if err := bot.Open(); err != nil {
		log.Fatalf("opening discord session: %v", err)
	}
	defer bot.Close()

	if err := bot.RegisterCommands(); err != nil {
		log.Fatalf("registering commands: %v", err)
	}

	healthServer := &http.Server{
		Addr:    ":" + cfg.HealthPort,
		Handler: health.NewHandler(bot, repo),
	}
	go func() {
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("health server: %v", err)
		}
	}()
	defer healthServer.Close()

	log.Println("bot is running, press ctrl+c to exit")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("shutting down")
}
