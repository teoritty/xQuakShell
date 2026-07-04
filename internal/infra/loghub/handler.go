package loghub

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"
)

// InstallDefault configures slog and the standard log package to publish into Default().
func InstallDefault() {
	h := NewHandler(Default(), os.Stderr)
	slog.SetDefault(slog.New(h))
}

// Handler publishes structured logs to a hub and optionally mirrors them to a writer.
type Handler struct {
	hub    *Hub
	fallback slog.Handler
}

// NewHandler creates a hub-backed slog handler with stderr/text fallback.
func NewHandler(hub *Hub, w io.Writer) *Handler {
	return &Handler{
		hub: hub,
		fallback: slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}),
	}
}

func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return h.fallback.Enabled(context.Background(), level)
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	fields := make(map[string]string)
	r.Attrs(func(a slog.Attr) bool {
		fields[a.Key] = attrString(a)
		return true
	})
	source := "core"
	if v, ok := fields["pluginId"]; ok {
		source = "plugin:" + v
		delete(fields, "pluginId")
	}
	h.hub.Publish(Entry{
		Time:    r.Time,
		Level:   strings.ToLower(r.Level.String()),
		Source:  source,
		Message: r.Message,
		Fields:  fields,
	})
	return h.fallback.Handle(ctx, r)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{hub: h.hub, fallback: h.fallback.WithAttrs(attrs)}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{hub: h.hub, fallback: h.fallback.WithGroup(name)}
}

func attrString(a slog.Attr) string {
	a.Value = a.Value.Resolve()
	switch a.Value.Kind() {
	case slog.KindString:
		return a.Value.String()
	case slog.KindInt64:
		return fmt.Sprintf("%d", a.Value.Int64())
	case slog.KindUint64:
		return fmt.Sprintf("%d", a.Value.Uint64())
	case slog.KindFloat64:
		return fmt.Sprintf("%g", a.Value.Float64())
	case slog.KindBool:
		if a.Value.Bool() {
			return "true"
		}
		return "false"
	case slog.KindTime:
		return a.Value.Time().Format(time.RFC3339Nano)
	default:
		return a.Value.String()
	}
}

// PublishPluginLog records a plugin log.write entry.
func PublishPluginLog(pluginID, level, message string, fields map[string]string) {
	if level == "" {
		level = "info"
	}
	src := "plugin:" + pluginID
	outFields := make(map[string]string, len(fields))
	for k, v := range fields {
		outFields[k] = v
	}
	Default().Publish(Entry{
		Time:    time.Now(),
		Level:   strings.ToLower(level),
		Source:  src,
		Message: message,
		Fields:  outFields,
	})
}

// PublishPluginStderr records a plugin stderr line.
func PublishPluginStderr(pluginID, message string, redacted bool) {
	fields := map[string]string{}
	if redacted {
		fields["redacted"] = "true"
	}
	Default().Publish(Entry{
		Time:    time.Now(),
		Level:   "info",
		Source:  "plugin-stderr:" + pluginID,
		Message: message,
		Fields:  fields,
	})
}

// PublishStdLog records a line from the standard log package.
func PublishStdLog(line string) {
	Default().Publish(Entry{
		Time:    time.Now(),
		Level:   "info",
		Source:  "core",
		Message: strings.TrimSpace(line),
	})
}
