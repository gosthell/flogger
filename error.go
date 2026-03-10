package flogger

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
)

// ContextProvider is an interface for errors that carry additional context
// that should be logged but not included in the error message.
type ContextProvider interface {
	Context() map[string]any
}

// StackProvider is an interface for errors that carry a stack trace.
type StackProvider interface {
	Stack() string
}

// contextError wraps an error with additional context data and stack trace.
type contextError struct {
	err     error
	context map[string]any
	stack   string
}

// Error returns only the wrapped error message, without the context.
func (e *contextError) Error() string {
	return e.err.Error()
}

// Unwrap returns the wrapped error for use with errors.Is/As.
func (e *contextError) Unwrap() error {
	return e.err
}

// Context returns the additional context data.
func (e *contextError) Context() map[string]any {
	return e.context
}

// Stack returns the stack trace captured at error creation.
func (e *contextError) Stack() string {
	return e.stack
}

// captureStack captures the current stack trace, skipping the specified
// number of frames (to exclude the error creation functions themselves).
func captureStack(skip int) string {
	const maxFrames = 32
	pcs := make([]uintptr, maxFrames)
	n := runtime.Callers(skip, pcs)
	if n == 0 {
		return ""
	}

	frames := runtime.CallersFrames(pcs[:n])
	var sb strings.Builder

	for {
		frame, more := frames.Next()
		// Skip runtime and stdlib frames
		if strings.Contains(frame.File, "runtime/") {
			if !more {
				break
			}
			continue
		}
		fmt.Fprintf(&sb, "%s\n\t%s:%d\n", frame.Function, frame.File, frame.Line)
		if !more {
			break
		}
	}

	return sb.String()
}

// ErrorWithContext wraps an error with additional context that will be
// available for logging but not included in the error message.
func ErrorWithContext(err error, ctx map[string]any) error {
	if err == nil {
		return nil
	}
	return &contextError{
		err:     err,
		context: ctx,
		stack:   captureStack(3),
	}
}

// Errorf creates a new error with context. The context is available via
// the ContextProvider interface but not included in Error().
func Errorf(ctx map[string]any, format string, args ...any) error {
	return &contextError{
		err:     fmt.Errorf(format, args...),
		context: ctx,
		stack:   captureStack(3),
	}
}

// GetErrorContext extracts context from an error chain.
// Returns nil if no context is found.
func GetErrorContext(err error) map[string]any {
	var ce *contextError
	if errors.As(err, &ce) {
		return ce.context
	}
	return nil
}

// GetErrorStack extracts the stack trace from an error.
// Returns empty string if no stack is found.
func GetErrorStack(err error) string {
	var sp StackProvider
	if errors.As(err, &sp) {
		return sp.Stack()
	}
	return ""
}

// GetAllErrorContext collects context from all errors in the chain
// that implement ContextProvider. Later contexts override earlier ones
// for duplicate keys.
func GetAllErrorContext(err error) map[string]any {
	if err == nil {
		return nil
	}

	result := make(map[string]any)
	for err != nil {
		if cp, ok := err.(ContextProvider); ok {
			for k, v := range cp.Context() {
				if _, exists := result[k]; !exists {
					result[k] = v
				}
			}
		}
		err = errors.Unwrap(err)
	}

	if len(result) == 0 {
		return nil
	}
	return result
}
