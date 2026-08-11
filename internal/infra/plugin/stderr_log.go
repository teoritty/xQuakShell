package plugin

import (
	"bufio"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/loghub"
	"xquakshell/internal/infra/plugin/sandbox"
	"xquakshell/internal/pkg/safego"
)

const stderrMaxLineBytes = 64 << 10

// NewRedactingStderrWriter returns a stderr sink that redacts secrets before logging.
func NewRedactingStderrWriter(pluginID string) io.WriteCloser {
	return newRedactingStderrWriter(pluginID)
}

// stderrTailLines is how much of a plugin's stderr is kept for a failure message. Enough to carry a
// Go panic's first frames or a runtime's complaint, and far short of anything that would hold a
// stream of log output in memory.
const stderrTailLines = 12

// redactingStderrWriter forwards plugin stderr through secret redaction before slog.
type redactingStderrWriter struct {
	pluginID string
	writer   *io.PipeWriter
	done     chan struct{}

	// mu guards tail, which is read from the start path while the consume goroutine is still
	// filling it.
	mu   sync.Mutex
	tail []string
}

// Tail returns the last lines the plugin wrote to stderr, already redacted.
//
// It exists for one caller: the error a failed start reports. A plugin that dies before its
// handshake says why on stderr, and stderr goes to the log hub — which is the right home for a
// running plugin's output and the wrong one for a diagnosis nobody will think to look for. Handing
// the last few lines back puts the cause in the error itself.
func (rs *redactingStderrWriter) Tail() []string {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return append([]string(nil), rs.tail...)
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

	rs.mu.Lock()
	rs.tail = append(rs.tail, message)
	if len(rs.tail) > stderrTailLines {
		rs.tail = rs.tail[len(rs.tail)-stderrTailLines:]
	}
	rs.mu.Unlock()

	// The sandbox shim's refusal is the one line here that is not plugin output: it is this
	// application explaining why it would not start the plugin, and it arrives on the same pipe
	// only because the shim becomes the plugin. Publishing it to the hub alone hides it from every
	// log a developer or a CI run actually reads, which turns "the plugin will not start" into a
	// bare "initialize: EOF" with no cause attached. It goes to slog as well.
	if strings.HasPrefix(message, sandbox.ShimFailurePrefix) {
		slog.Warn("plugin sandbox refused to start a plugin",
			"component", "plugin.stderr", "pluginId", rs.pluginID, "detail", message)
	}
}
