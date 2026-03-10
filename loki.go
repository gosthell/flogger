package flogger

import (
	"fmt"
	"time"

	"github.com/gosthell/promtail"
	"github.com/sirupsen/logrus"
)

// LokiConfig defines configuration for hook for Loki
type LokiConfig struct {
	URL                string
	Labels             string
	BatchWait          time.Duration
	BatchEntriesNumber int
	QueueSize          int
	Level              logrus.Level
}

func (c *LokiConfig) setDefault() {
	if c.URL == "" {
		c.URL = "http://localhost:3100/api"
	}
	if c.Labels == "" {
		c.Labels = "{source=\"" + "test" + "\",job=\"" + "job" + "\"}"
	}
	if c.BatchWait == time.Second {
		c.BatchWait = 5 * time.Second
	}
	if c.BatchEntriesNumber == 0 {
		c.BatchEntriesNumber = 1000
	}
	if c.Level == 0 {
		c.Level = logrus.InfoLevel
	}
	if c.QueueSize == 0 {
		c.QueueSize = 10000
	}
}

type LokiHook struct {
	c      *LokiConfig
	client promtail.Client
}

// NewLokiHook creates a new hook for Loki
func NewLokiHook(c *LokiConfig) (*LokiHook, error) {
	if c == nil {
		c = &LokiConfig{}
	}
	c.setDefault()
	promtailClient, err := promtail.NewJSONv1Client(c.URL, nil,
		promtail.WithSendBatchSize(uint(c.BatchEntriesNumber)),
		promtail.WithSendBatchTimeout(c.BatchWait),
		promtail.WithQueueSize(c.QueueSize))
	if err != nil {
		return nil, err
	}
	return &LokiHook{
		c:      c,
		client: promtailClient,
	}, nil
}

// Fire implements interface for logrus
func (hook *LokiHook) Fire(entry *logrus.Entry) error {
	labels := make(map[string]string, len(entry.Data)+1)
	for k, v := range entry.Data {
		switch x := v.(type) {
		case string:
			labels[k] = x
		case error:
			labels[k] = x.Error()
			// Extract context from errors that implement ContextProvider
			if ctx := GetAllErrorContext(x); ctx != nil {
				for ck, cv := range ctx {
					labels["ctx_"+ck] = fmt.Sprintf("%+v", cv)
				}
			}
			// Extract stack trace from errors that implement StackProvider
			if stack := GetErrorStack(x); stack != "" {
				labels["stack"] = stack
			}
		default:
			labels[k] = fmt.Sprintf("%v", v)
		}
	}
	labels["level"] = entry.Level.String()
	var level promtail.Level
	switch entry.Level {
	case logrus.PanicLevel:
		level = promtail.Panic
	case logrus.FatalLevel:
		level = promtail.Fatal
	case logrus.ErrorLevel:
		level = promtail.Error
	case logrus.WarnLevel:
		level = promtail.Warn
	case logrus.InfoLevel:
		level = promtail.Info
	case logrus.DebugLevel:
		level = promtail.Debug
	default:
		level = promtail.Debug
	}
	hook.client.LogfWithLabels(level, labels, entry.Message)
	return nil
}

// Levels retruns supported levels
func (hook *LokiHook) Levels() []logrus.Level {
	return logrus.AllLevels[:hook.c.Level+1]
}
