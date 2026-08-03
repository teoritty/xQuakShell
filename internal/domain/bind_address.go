package domain

import (
	"net"
	"strings"
)

// IsLoopbackBind reports whether a bind address is loopback-only or empty (defaults to loopback).
func IsLoopbackBind(addr string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return true
	}
	host := addr
	if strings.Contains(addr, ":") {
		if h, _, err := net.SplitHostPort(addr); err == nil {
			host = h
		}
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return strings.EqualFold(host, "localhost")
}

// EffectiveBindAddress returns loopback default for empty bind addresses.
func EffectiveBindAddress(addr string) string {
	if strings.TrimSpace(addr) == "" {
		return "127.0.0.1"
	}
	return addr
}
