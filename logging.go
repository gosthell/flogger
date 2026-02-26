package assistant

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
)

type loggerContextKey struct{}
type spanContextKey struct{}

type spanContext struct {
	traceID string
	spanID  string
}

type LogSpan struct {
	ctx          context.Context
	logger       *log.Entry
	traceID      string
	spanID       string
	parentSpanID string
	spanName     string
	startedAt    time.Time
	ended        uint32
}

var traceSeq uint64
var spanSeq uint64
var eventSeq uint64

func nextTraceID(source string) string {
	seq := atomic.AddUint64(&traceSeq, 1)
	if source == "" {
		source = "request"
	}
	return fmt.Sprintf("t-%s-%d-%d", source, time.Now().UnixNano(), seq)
}

func nextSpanID(name string) string {
	seq := atomic.AddUint64(&spanSeq, 1)
	if name == "" {
		name = "span"
	}
	return fmt.Sprintf("s-%s-%d-%d", name, time.Now().UnixNano(), seq)
}

func copyLogFields(fields log.Fields) log.Fields {
	if len(fields) == 0 {
		return nil
	}
	copied := make(log.Fields, len(fields))
	for k, v := range fields {
		copied[k] = v
	}
	return copied
}

func stringField(entry *log.Entry, key string) string {
	if entry == nil {
		return ""
	}
	value, ok := entry.Data[key]
	if !ok || value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", value)
}

func ContextWithLogger(ctx context.Context, logger *log.Entry) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if logger == nil {
		return ctx
	}
	return context.WithValue(ctx, loggerContextKey{}, logger)
}

func LoggerFromContext(ctx context.Context, fallback *log.Entry) *log.Entry {
	if ctx != nil {
		if logger, ok := ctx.Value(loggerContextKey{}).(*log.Entry); ok && logger != nil {
			return logger
		}
	}
	if fallback != nil {
		return fallback
	}
	return log.NewEntry(log.StandardLogger())
}

func WithLogFields(ctx context.Context, fallback *log.Entry, fields log.Fields) (context.Context, *log.Entry) {
	logger := LoggerFromContext(ctx, fallback)
	if len(fields) > 0 {
		logger = logger.WithFields(copyLogFields(fields))
	}
	return ContextWithLogger(ctx, logger), logger
}

func WithLogField(ctx context.Context, fallback *log.Entry, key string, value any) (context.Context, *log.Entry) {
	return WithLogFields(ctx, fallback, log.Fields{key: value})
}

func WithRequestLogger(ctx context.Context, fallback *log.Entry, source string, fields log.Fields) (context.Context, *log.Entry) {
	merged := copyLogFields(fields)
	if merged == nil {
		merged = log.Fields{}
	}
	if source == "" {
		source = "request"
	}
	if _, exists := merged["trace_id"]; !exists {
		merged["trace_id"] = nextTraceID(source)
	}
	if _, exists := merged["span_id"]; !exists {
		merged["span_id"] = nextSpanID(source)
	}
	if _, exists := merged["parent_span_id"]; !exists {
		merged["parent_span_id"] = ""
	}
	if _, exists := merged["span_name"]; !exists {
		merged["span_name"] = source
	}
	if _, exists := merged["root_source"]; !exists {
		merged["root_source"] = source
	}
	if _, exists := merged["request_source"]; !exists {
		merged["request_source"] = source
	}
	ctx, logger := WithLogFields(ctx, fallback, merged)
	ctx = context.WithValue(ctx, spanContextKey{}, spanContext{
		traceID: stringField(logger, "trace_id"),
		spanID:  stringField(logger, "span_id"),
	})
	return ctx, logger
}

func startSpan(ctx context.Context, fallback *log.Entry, traceID, parentSpanID, spanName, requestSource string, fields log.Fields) (context.Context, *LogSpan) {
	merged := copyLogFields(fields)
	if merged == nil {
		merged = log.Fields{}
	}
	if spanName == "" {
		spanName = "span"
	}
	if requestSource == "" {
		requestSource = spanName
	}

	spanID := nextSpanID(spanName)
	merged["trace_id"] = traceID
	merged["span_id"] = spanID
	merged["parent_span_id"] = parentSpanID
	merged["span_name"] = spanName
	merged["request_source"] = requestSource
	ctx, logger := WithLogFields(ctx, fallback, merged)

	ctx = context.WithValue(ctx, spanContextKey{}, spanContext{
		traceID: traceID,
		spanID:  spanID,
	})

	span := &LogSpan{
		ctx:          ctx,
		logger:       logger,
		traceID:      traceID,
		spanID:       spanID,
		parentSpanID: parentSpanID,
		spanName:     spanName,
		startedAt:    time.Now(),
	}
	logger.WithField("event_type", "span_start").Info("span start")
	return ctx, span
}

func StartRequestSpan(ctx context.Context, fallback *log.Entry, source string, fields log.Fields) (context.Context, *LogSpan) {
	if source == "" {
		source = "request"
	}
	base := LoggerFromContext(ctx, fallback)
	traceID := stringField(base, "trace_id")
	if traceID == "" {
		traceID = nextTraceID(source)
	}
	return startSpan(ctx, base, traceID, "", source, source, fields)
}

func StartSpan(ctx context.Context, fallback *log.Entry, spanName string, fields log.Fields) (context.Context, *LogSpan) {
	base := LoggerFromContext(ctx, fallback)
	requestSource := stringField(base, "request_source")

	var parent spanContext
	if raw := ctx.Value(spanContextKey{}); raw != nil {
		if s, ok := raw.(spanContext); ok {
			parent = s
		}
	}

	traceID := parent.traceID
	parentSpanID := parent.spanID
	if traceID == "" {
		traceID = stringField(base, "trace_id")
	}
	if traceID == "" {
		traceID = nextTraceID(spanName)
	}
	return startSpan(ctx, base, traceID, parentSpanID, spanName, requestSource, fields)
}

func (s *LogSpan) Context() context.Context {
	if s == nil {
		return context.Background()
	}
	return s.ctx
}

func (s *LogSpan) Logger() *log.Entry {
	if s == nil {
		return log.NewEntry(log.StandardLogger())
	}
	return s.logger
}

func (s *LogSpan) End() {
	if s == nil {
		return
	}
	if !atomic.CompareAndSwapUint32(&s.ended, 0, 1) {
		return
	}
	duration := time.Since(s.startedAt).Milliseconds()
	s.logger.WithFields(log.Fields{
		"event_type":       "span_end",
		"span_duration_ms": duration,
	}).Info("span end")
}

type callTreeHook struct{}

func NewCallTreeHook() log.Hook {
	return &callTreeHook{}
}

func (h *callTreeHook) Levels() []log.Level {
	return log.AllLevels
}

func (h *callTreeHook) Fire(entry *log.Entry) error {
	if _, ok := entry.Data["event_type"]; !ok {
		entry.Data["event_type"] = "log"
	}
	if _, ok := entry.Data["event_seq"]; !ok {
		entry.Data["event_seq"] = atomic.AddUint64(&eventSeq, 1)
	}
	if _, ok := entry.Data["trace_id"]; !ok {
		entry.Data["trace_id"] = "trace_unknown"
	}
	if _, ok := entry.Data["span_id"]; !ok {
		entry.Data["span_id"] = "span_unknown"
	}
	if _, ok := entry.Data["parent_span_id"]; !ok {
		entry.Data["parent_span_id"] = ""
	}
	if _, ok := entry.Data["span_name"]; !ok {
		entry.Data["span_name"] = "unknown"
	}
	return nil
}
