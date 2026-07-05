package domain

import "context"

// Pinger checks network reachability (TCP connect probe).
// Implementations perform I/O; usecase only orchestrates timing and result mapping.
// Success means the connection was established; closing the connection is the implementation's responsibility.
type Pinger interface {
	Ping(ctx context.Context, network, address string) error
}
