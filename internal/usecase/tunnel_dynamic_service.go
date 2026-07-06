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
	conn         net.Conn
	stop         chan struct{}
	done         chan struct{}
	preBindTimer *time.Timer
}

func (e *localEntry) disarmPreBindTimer() {
	if e.preBindTimer != nil {
		e.preBindTimer.Stop()
		e.preBindTimer = nil
	}
}

// TunnelDynamicService manages pre-bind local/tunnel channel registries for dynamic forwards.
type TunnelDynamicService struct {
	mu             sync.Mutex
	local          map[string]*localEntry
	channels       map[string]net.Conn
	dialer         domain.TunnelChannelDialer
	notify         PluginTunnelNotifier
	preBindTimeout time.Duration // zero → domainplugin.PreBindTunnelTimeout
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

func (s *TunnelDynamicService) preBindTimeoutOrDefault() time.Duration {
	if s.preBindTimeout > 0 {
		return s.preBindTimeout
	}
	return domainplugin.PreBindTunnelTimeout
}

func (s *TunnelDynamicService) armPreBindTimer(localConnID string) {
	s.mu.Lock()
	entry, ok := s.local[localConnID]
	if !ok {
		s.mu.Unlock()
		return
	}
	entry.preBindTimer = time.AfterFunc(s.preBindTimeoutOrDefault(), func() {
		s.evictPreBindLocal(localConnID)
	})
	s.mu.Unlock()
}

func (s *TunnelDynamicService) evictPreBindLocal(localConnID string) {
	s.mu.Lock()
	entry, ok := s.local[localConnID]
	if !ok {
		s.mu.Unlock()
		return
	}
	entry.disarmPreBindTimer()
	delete(s.local, localConnID)
	s.mu.Unlock()
	s.stopReading(entry)
	_ = entry.conn.Close()
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
// onSpliceDone is invoked exactly once after both ends close; nil is ignored.
func (s *TunnelDynamicService) Bind(localConnID, tunnelID string, onSpliceDone func()) error {
	s.mu.Lock()
	entry, okL := s.local[localConnID]
	remote, okR := s.channels[tunnelID]
	if okL {
		entry.disarmPreBindTimer()
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
	safego.Go(func() {
		splice(entry.conn, remote)
		if onSpliceDone != nil {
			onSpliceDone()
		}
	})
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

// CloseAllPreBind closes every pre-bind local connection and unbound tunnel channel.
func (s *TunnelDynamicService) CloseAllPreBind() {
	s.mu.Lock()
	locals := s.local
	channels := s.channels
	s.local = make(map[string]*localEntry)
	s.channels = make(map[string]net.Conn)
	s.mu.Unlock()

	for _, entry := range locals {
		entry.disarmPreBindTimer()
		s.stopReading(entry)
		_ = entry.conn.Close()
	}
	for _, conn := range channels {
		_ = conn.Close()
	}
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

	s.armPreBindTimer(localConnID)

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
		entry.disarmPreBindTimer()
		delete(s.local, localConnID)
	}
	s.mu.Unlock()
	if !ok {
		return
	}
	s.stopReading(entry)
	_ = entry.conn.Close()
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
