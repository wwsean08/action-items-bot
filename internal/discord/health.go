package discord

import (
	"fmt"
	"time"
)

// heartbeatStaleAfter is well over Discord's ~41s default heartbeat
// interval, giving room for a missed beat or two before declaring the
// gateway connection unhealthy.
const heartbeatStaleAfter = 90 * time.Second

func (b *Bot) GatewayHealthy() error {
	if !b.Session.DataReady {
		return fmt.Errorf("gateway connection not ready")
	}
	if age := time.Since(b.Session.LastHeartbeatAck); age > heartbeatStaleAfter {
		return fmt.Errorf("last heartbeat ack was %s ago", age.Round(time.Second))
	}
	return nil
}
