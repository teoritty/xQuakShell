package connectors

import (
	"context"
	"fmt"

	"go.bug.st/serial"
	"ssh-client/internal/domain"
	"ssh-client/internal/infra/stream"
)

// SerialConnector establishes a Serial session via COM port and streams I/O to the terminal.
type SerialConnector struct{}

// NewSerialConnector creates a Serial connector.
func NewSerialConnector() *SerialConnector {
	return &SerialConnector{}
}

// Protocol implements domain.SessionConnector.
func (c *SerialConnector) Protocol() string {
	return domain.ProtocolSerial
}

// Connect implements domain.SessionConnector.
func (c *SerialConnector) Connect(ctx context.Context, conn *domain.Connection, _ domain.ConnectorDeps, hooks domain.ConnectorHooks) error {
	if conn.SerialConfig == nil || conn.SerialConfig.Port == "" {
		hooks.UpdateState(domain.SessionError, "serial port not configured")
		return nil
	}

	cfg := conn.SerialConfig
	mode := &serial.Mode{
		BaudRate: baudRateOrDefault(cfg.BaudRate),
		Parity:   parityFromString(cfg.Parity),
		DataBits: dataBitsOrDefault(cfg.DataBits),
		StopBits: stopBitsFromInt(cfg.StopBits),
	}

	port, err := serial.Open(cfg.Port, mode)
	if err != nil {
		hooks.UpdateState(domain.SessionError, fmt.Sprintf("serial open: %s", err.Error()))
		return nil
	}

	bridge := stream.NewBridge()
	outputCh, err := bridge.StartFromConn(ctx, port)
	if err != nil {
		port.Close()
		hooks.UpdateState(domain.SessionError, fmt.Sprintf("serial stream: %s", err.Error()))
		return nil
	}

	hooks.SetPTYBridge(bridge)
	if hooks.OnStreamReady != nil {
		hooks.OnStreamReady(outputCh)
	}
	hooks.UpdateState(domain.SessionReady, "")
	return nil
}

func parityFromString(p string) serial.Parity {
	switch p {
	case "N", "None", "none":
		return serial.NoParity
	case "O", "Odd", "odd":
		return serial.OddParity
	case "E", "Even", "even":
		return serial.EvenParity
	case "M", "Mark", "mark":
		return serial.MarkParity
	case "S", "Space", "space":
		return serial.SpaceParity
	default:
		return serial.NoParity
	}
}

func baudRateOrDefault(n int) int {
	if n > 0 {
		return n
	}
	return 9600
}

func dataBitsOrDefault(n int) int {
	if n >= 5 && n <= 8 {
		return n
	}
	return 8
}

func stopBitsFromInt(n int) serial.StopBits {
	switch n {
	case 2:
		return serial.TwoStopBits
	case 15:
		return serial.OnePointFiveStopBits
	default:
		return serial.OneStopBit
	}
}
