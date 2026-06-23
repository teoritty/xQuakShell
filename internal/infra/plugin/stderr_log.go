package plugin

import (
	"bufio"
	"io"
	"log/slog"

	domainplugin "ssh-client/internal/domain/plugin"
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
	go rs.consume(reader)
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
		slog.Debug("plugin stderr scanner", "pluginId", rs.pluginID, "err", err)
	}
}

func (rs *redactingStderrWriter) logLine(line string) {
	message, redacted := domainplugin.RedactLogMessage(line)
	if redacted {
		slog.Info("plugin stderr", "pluginId", rs.pluginID, "message", message, "redacted", true)
		return
	}
	slog.Info("plugin stderr", "pluginId", rs.pluginID, "message", message)
}
