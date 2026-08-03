package plugin

import (
	"bufio"
	"errors"
	"io"
	"log/slog"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/loghub"
	"xquakshell/internal/pkg/safego"
)

const stderrMaxLineBytes = 64 << 10

// NewRedactingStderrWriter returns a stderr sink that redacts secrets before logging.
func NewRedactingStderrWriter(pluginID string) io.WriteCloser {
	return newRedactingStderrWriter(pluginID)
}

// redactingStderrWriter forwards plugin stderr through secret redaction before slog.
type redactingStderrWriter struct {
	pluginID string
	writer   *io.PipeWriter
	done     chan struct{}
}

func newRedactingStderrWriter(pluginID string) *redactingStderrWriter {
	reader, writer := io.Pipe()
	rs := &redactingStderrWriter{
		pluginID: pluginID,
		writer:   writer,
		done:     make(chan struct{}),
	}
	safego.GoNamed("plugin.stderrConsume", func() { rs.consume(reader) })
	return rs
}

func (rs *redactingStderrWriter) Write(p []byte) (int, error) {
	return rs.writer.Write(p)
}

func (rs *redactingStderrWriter) Close() error {
	err := rs.writer.Close()
	<-rs.done
	return err
}

func (rs *redactingStderrWriter) consume(reader *io.PipeReader) {
	defer close(rs.done)
	defer reader.Close()

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), stderrMaxLineBytes)
	for scanner.Scan() {
		rs.logLine(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		// bufio.ErrTooLong means a stderr line exceeded the buffer and was
		// discarded — surface it rather than dropping silently.
		if errors.Is(err, bufio.ErrTooLong) {
			slog.Warn("plugin stderr line dropped: exceeds max length", "component", "plugin.stderr", "pluginId", rs.pluginID, "maxBytes", stderrMaxLineBytes)
		} else {
			slog.Debug("plugin stderr scanner", "component", "plugin.stderr", "pluginId", rs.pluginID, "err", err)
		}
	}
}

func (rs *redactingStderrWriter) logLine(line string) {
	// Publish exactly once via loghub (tagged plugin-stderr:<id>); the previous
	// extra slog.Info produced a duplicate entry in the hub.
	message, redacted := domainplugin.RedactLogMessage(line)
	loghub.PublishPluginStderr(rs.pluginID, message, redacted)
}
