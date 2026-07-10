package main

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"log"
	"net"

	"ssh-client/test/fixtures/pluginhost"
)

func main() {
	host := pluginhost.NewHost()
	var pendingTunnelID string

	host.Register("initialize", func(_ json.RawMessage) (any, error) {
		return map[string]bool{"ok": true}, nil
	})
	host.Register("activate", func(_ json.RawMessage) (any, error) {
		return map[string]bool{"ok": true}, nil
	})
	host.Register("shutdown", func(_ json.RawMessage) (any, error) {
		return map[string]bool{"ok": true}, nil
	})

	host.RegisterNotification("tunnel.localAccept", func(params json.RawMessage) {
		var req struct {
			RuleID      string `json:"ruleId"`
			ProviderID  string `json:"providerId"`
			LocalConnID string `json:"localConnId"`
		}
		if err := json.Unmarshal(params, &req); err != nil {
			return
		}
		go handleSOCKS5Client(host, req.LocalConnID, &pendingTunnelID)
	})

	if err := host.Run(); err != nil {
		log.Fatal(err)
	}
}

func handleSOCKS5Client(host *pluginhost.Host, localConnID string, pendingTunnelID *string) {
	// SOCKS5 greeting is delivered via tunnel.localFrame notifications; this fixture
	// dials through the coordinator once the plugin receives the first client frame.
	raw, err := host.CallCore("tunnel.dial", map[string]any{
		"ruleId":     "rule-under-test",
		"targetHost": "example.com",
		"targetPort": 80,
	})
	if err != nil {
		return
	}
	var dialRes struct {
		TunnelID string `json:"tunnelId"`
	}
	if err := json.Unmarshal(raw, &dialRes); err != nil {
		return
	}
	*pendingTunnelID = dialRes.TunnelID
	_, _ = host.CallCore("tunnel.bind", map[string]string{
		"localConnId": localConnID,
		"tunnelId":    dialRes.TunnelID,
	})
}

// parseSOCKS5ConnectTarget decodes a SOCKS5 CONNECT request from client bytes.
func parseSOCKS5ConnectTarget(payload []byte) (host string, port int, ok bool) {
	if len(payload) < 7 || payload[0] != 0x05 || payload[1] != 0x01 {
		return "", 0, false
	}
	atyp := payload[3]
	switch atyp {
	case 0x01:
		if len(payload) < 10 {
			return "", 0, false
		}
		host = net.IP(payload[4:8]).String()
		port = int(binary.BigEndian.Uint16(payload[8:10]))
		return host, port, true
	case 0x03:
		l := int(payload[4])
		if len(payload) < 5+l+2 {
			return "", 0, false
		}
		host = string(payload[5 : 5+l])
		port = int(binary.BigEndian.Uint16(payload[5+l : 7+l]))
		return host, port, true
	default:
		return "", 0, false
	}
}

func writeSOCKS5Greeting(w io.Writer) error {
	_, err := w.Write([]byte{0x05, 0x01, 0x00})
	return err
}

func writeSOCKS5Connect(w io.Writer, host string, port int) error {
	req := []byte{0x05, 0x01, 0x00, 0x03, byte(len(host))}
	req = append(req, []byte(host)...)
	var p [2]byte
	binary.BigEndian.PutUint16(p[:], uint16(port))
	req = append(req, p[:]...)
	_, err := w.Write(req)
	return err
}

func decodeTunnelFrameData(dataBase64 string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(dataBase64)
}
