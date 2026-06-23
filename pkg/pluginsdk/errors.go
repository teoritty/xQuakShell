package pluginsdk

import (
	"encoding/json"
	"errors"
	"fmt"
)

// JSON-RPC error codes mirrored from the xQuakShell core host.
const (
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternalError  = -32603
	ErrCodeCapabilityDenied = -32001
	ErrCodeResourceNotFound = -32002
	ErrCodeRateLimited    = -32003
	ErrCodeNotImplemented = -32004
)

// CoreError is a typed JSON-RPC error returned by the core host.
type CoreError struct {
	Code    int
	Message string
	Data    json.RawMessage
}

func (e *CoreError) Error() string {
	if e == nil {
		return "core rpc error"
	}
	return fmt.Sprintf("core rpc %d: %s", e.Code, e.Message)
}

// IsCapabilityDenied reports whether err is a core capability denial.
func IsCapabilityDenied(err error) bool {
	var core *CoreError
	return errors.As(err, &core) && core.Code == ErrCodeCapabilityDenied
}

// IsRateLimited reports whether err is a core rate-limit response.
func IsRateLimited(err error) bool {
	var core *CoreError
	return errors.As(err, &core) && core.Code == ErrCodeRateLimited
}

// IsResourceNotFound reports whether err is a core resource-not-found response.
func IsResourceNotFound(err error) bool {
	var core *CoreError
	return errors.As(err, &core) && core.Code == ErrCodeResourceNotFound
}

// CoreErrorCode returns the JSON-RPC code when err is a CoreError.
func CoreErrorCode(err error) (int, bool) {
	var core *CoreError
	if errors.As(err, &core) {
		return core.Code, true
	}
	return 0, false
}
