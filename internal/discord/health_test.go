package discord

import (
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

func newBotWithSession(session *discordgo.Session) *Bot {
	return &Bot{Session: session}
}

func TestGatewayHealthy_ReturnsErrorWhenNotDataReady(t *testing.T) {
	b := newBotWithSession(&discordgo.Session{
		DataReady:        false,
		LastHeartbeatAck: time.Now(),
	})

	if err := b.GatewayHealthy(); err == nil {
		t.Fatal("expected error when DataReady is false")
	}
}

func TestGatewayHealthy_ReturnsErrorWhenHeartbeatStale(t *testing.T) {
	b := newBotWithSession(&discordgo.Session{
		DataReady:        true,
		LastHeartbeatAck: time.Now().Add(-2 * time.Minute),
	})

	if err := b.GatewayHealthy(); err == nil {
		t.Fatal("expected error when last heartbeat ack is stale")
	}
}

func TestGatewayHealthy_ReturnsNilWhenReadyAndHeartbeatRecent(t *testing.T) {
	b := newBotWithSession(&discordgo.Session{
		DataReady:        true,
		LastHeartbeatAck: time.Now(),
	})

	if err := b.GatewayHealthy(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
