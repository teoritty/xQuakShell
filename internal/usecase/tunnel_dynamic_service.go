package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"strconv"
	"sync"
	"time"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/pkg/safego"
)

const maxPreBindTunnelEntries = 256

type localEntry struct {
	conn net.Conn
	stop chan struct{}
	done chan struct{}
}

// TunnelDynamicService manages pre-bind local/tunnel channel registries for dynamic forwards.
type TunnelDynamicService struct {
	mu       sync.Mutex
	local    map[string]*localEntry
	channels map[string]net.Conn
	dialer   domain.TunnelChannelDialer
	notify   PluginTunnelNotifier
}

// NewTunnelDynamicService creates a dynamic tunnel coordinator.
func NewTunnelDynamicService(dialer domain.TunnelChannelDialer, notify PluginTunnelNotifier) *TunnelDynamicService {
	return &TunnelDynamicService{
		local:    make(map[string]*localEntry),
		channels: make(map[string]net.Conn),
		dialer:   dialer,
		notify:   notify,
	}
}

// Dial opens an SSH direct-tcpip channel and stores it under tunnelID.
func (s *TunnelDynamicService) Dial(ctx context.Context, tunnelID, targetHost string, targetPort int) error {
	conn, err := s.dialer.OpenDirectTCP(ctx, net.JoinHostPort(targetHost, strconv.Itoa(targetPort)))
	if err != nil {
		return err
	}
	s.mu.Lock()
	if _, exists := s.channels[tunnelID]; exists {
		s.mu.Unlock()
		conn.Close()
		return domainplugin.ErrTunnelNotFound
	}
	s.channels[tunnelID] = conn
	s.mu.Unlock()
	return nil
}

// Bind hands off local and tunnel connections to the native splice path.
func (s *TunnelDynamicService) Bind(localConnID, tunnelID string) error {
	s.mu.Lock()
	entry, okL := s.local[localConnID]
	remote, okR := s.channels[tunnelID]
	if okL {
		delete(s.local, localConnID)
	}
	if okR {
		delete(s.channels, tunnelID)
	}
	s.mu.Unlock()
	if !okL || !okR {
		return domainplugin.ErrTunnelNotFound
	}
	s.stopReading(entry)
	safego.Go(func() { splice(entry.conn, remote) })
	return nil
}

func (s *TunnelDynamicService) stopReading(entry *localEntry) {
	close(entry.stop)
	_ = entry.conn.SetReadDeadline(time.Now())
	<-entry.done
	_ = entry.conn.SetReadDeadline(time.Time{})
}

// CloseChannel closes an unbound SSH tunnel channel.
func (s *TunnelDynamicService) CloseChannel(tunnelID string) error {
	s.mu.Lock()
	conn, ok := s.channels[tunnelID]
	delete(s.channels, tunnelID)
	s.mu.Unlock()
	if !ok {
		return domainplugin.ErrTunnelNotFound
	}
	return conn.Close()
}

// HasLocal reports whether a pre-bind local connection is registered.
func (s *TunnelDynamicService) HasLocal(localConnID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.local[localConnID]
	return ok
}

// HasChannel reports whether an unbound tunnel channel exists.
func (s *TunnelDynamicService) HasChannel(tunnelID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.channels[tunnelID]
	return ok
}

// RegisterLocal stores a pre-bind local connection and starts frame forwarding to the plugin.
func (s *TunnelDynamicService) RegisterLocal(ctx context.Context, pluginID, ruleID, providerID, localConnID string, conn net.Conn) error {
	entry := &localEntry{
		conn: conn,
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	s.mu.Lock()
	if len(s.local)+len(s.channels) >= maxPreBindTunnelEntries {
		s.mu.Unlock()
		conn.Close()
		return domainplugin.ErrRateLimited
	}
	s.local[localConnID] = entry
	s.mu.Unlock()

	if s.notify != nil {
		params, _ := json.Marshal(map[string]string{
			"ruleId": ruleID, "providerId": providerID, "localConnId": localConnID,
		})
		if err := s.notify(ctx, pluginID, "", "tunnel.localAccept", params); err != nil {
			s.closeLocal(localConnID)
			return err
		}
	}

	safego.Go(func() {
		defer close(entry.done)
		buf := make([]byte, 32*1024)
		for {
			select {
			case <-entry.stop:
				return
			default:
			}
			n, err := conn.Read(buf)
			if n > 0 && s.notify != nil {
				params, _ := json.Marshal(map[string]any{
					"localConnId": localConnID,
					"dataBase64":  base64.StdEncoding.EncodeToString(buf[:n]),
					"eof":         false,
				})
				_ = s.notify(ctx, pluginID, "", "tunnel.localFrame", params)
			}
			if err != nil {
				select {
				case <-entry.stop:
					return
				default:
				}
				if s.notify != nil {
					params, _ := json.Marshal(map[string]string{"localConnId": localConnID})
					_ = s.notify(ctx, pluginID, "", "tunnel.localClose", params)
				}
				return
			}
		}
	})
	return nil
}

func (s *TunnelDynamicService) closeLocal(localConnID string) {
	s.mu.Lock()
	entry, ok := s.local[localConnID]
	if ok {
		delete(s.local, localConnID)
	}
	s.mu.Unlock()
	if !ok {
		return
	}
	s.stopReading(entry)
}

// WriteLocal writes plugin protocol bytes to a pre-bind local connection.
func (s *TunnelDynamicService) WriteLocal(localConnID string, data []byte) error {
	s.mu.Lock()
	entry, ok := s.local[localConnID]
	s.mu.Unlock()
	if !ok {
		return domainplugin.ErrTunnelNotFound
	}
	_, err := entry.conn.Write(data)
	return err
}

// CloseLocal closes a pre-bind local connection.
func (s *TunnelDynamicService) CloseLocal(localConnID string) error {
	s.closeLocal(localConnID)
	return nil
}
