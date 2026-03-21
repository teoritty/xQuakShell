package ssh

import (
	"context"
	"fmt"
	"net"

	gossh "golang.org/x/crypto/ssh"

	"ssh-client/internal/domain"
)

// DirectDialer establishes direct TCP connections.
type DirectDialer struct{}

// DialContext opens a TCP connection to the given address.
func (d *DirectDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	var dialer net.Dialer
	return dialer.DialContext(ctx, network, address)
}

// BastionDialer routes connections through an existing SSH client using direct-tcpip channels.
type BastionDialer struct {
	client *gossh.Client
}

// NewBastionDialer creates a transport dialer that routes through the given SSH client.
func NewBastionDialer(client *gossh.Client) *BastionDialer {
	return &BastionDialer{client: client}
}

// DialContext opens a direct-tcpip channel through the bastion SSH connection.
func (b *BastionDialer) DialContext(_ context.Context, _, address string) (net.Conn, error) {
	conn, err := b.client.Dial("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("bastion dial %s: %w", address, err)
	}
	return conn, nil
}

// BuildTransportChain establishes SSH connections through a chain of bastion hops.
// Returns the final net.Conn that reaches the target, and a cleanup function for
// all intermediate SSH clients.
//
// chain[0] is the first bastion, chain[len-1] is the last bastion before the target.
// The caller should use the returned net.Conn as SSHClientConfig.Transport for the final target.
// timeoutSeconds is used for each hop's TCP/SSH handshake (0 = default 15).
// proxyAuth, when non-nil, routes the first hop's TCP through SOCKS.
// Prefer JumpTransportBuilder.BuildChain from higher layers; this function is the implementation.
func BuildTransportChain(
	ctx context.Context,
	hops []domain.JumpHop,
	targetHost string,
	targetPort int,
	timeoutSeconds int,
	proxyAuth *domain.ProxyAuth,
	sshFactory domain.SSHClientFactory,
	hostKeyCallback gossh.HostKeyCallback,
	resolveHopAuth domain.JumpHopAuthResolver,
) (net.Conn, func(), error) {
	var clients []domain.SSHClient
	cleanup := func() {
		for i := len(clients) - 1; i >= 0; i-- {
			clients[i].Close()
		}
	}

	var transport net.Conn

	for i, hop := range hops {
		signers, password, err := resolveHopAuth(hop)
		if err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("hop %d auth: %w", i, err)
		}

		hopTimeout := timeoutSeconds
		if hopTimeout <= 0 {
			hopTimeout = 15
		}
		cfg := domain.SSHClientConfig{
			Host:            hop.Host,
			Port:            hop.Port,
			User:            hop.Username,
			Signers:         signers,
			Password:        password,
			HostKeyCallback: hostKeyCallback,
			TimeoutSeconds:  hopTimeout,
			Transport:       transport,
		}
		if i == 0 && proxyAuth != nil && proxyAuth.Host != "" && proxyAuth.Port > 0 {
			cfg.Proxy = proxyAuth
		}

		client, err := sshFactory.Create(ctx, cfg)
		if err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("hop %d (%s:%d) connect: %w", i, hop.Host, hop.Port, err)
		}
		clients = append(clients, client)

		nextAddr := fmt.Sprintf("%s:%d", targetHost, targetPort)
		if i < len(hops)-1 {
			nextAddr = fmt.Sprintf("%s:%d", hops[i+1].Host, hops[i+1].Port)
		}

		transport, err = client.Client().Dial("tcp", nextAddr)
		if err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("hop %d forward to %s: %w", i, nextAddr, err)
		}
	}

	return transport, cleanup, nil
}

type jumpTransportBuilder struct{}

// NewJumpTransportBuilder returns a domain.JumpTransportBuilder backed by BuildTransportChain.
func NewJumpTransportBuilder() domain.JumpTransportBuilder {
	return jumpTransportBuilder{}
}

func (jumpTransportBuilder) BuildChain(
	ctx context.Context,
	hops []domain.JumpHop,
	targetHost string,
	targetPort int,
	timeoutSeconds int,
	proxyAuth *domain.ProxyAuth,
	factory domain.SSHClientFactory,
	hostKeyCallback gossh.HostKeyCallback,
	resolveHopAuth domain.JumpHopAuthResolver,
) (net.Conn, func(), error) {
	return BuildTransportChain(ctx, hops, targetHost, targetPort, timeoutSeconds, proxyAuth, factory, hostKeyCallback, resolveHopAuth)
}
