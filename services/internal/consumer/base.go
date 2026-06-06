package consumer

import (
	"context"
	"fmt"
	"time"

	"github.com/cogent/services/internal/schema"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// Handler processes a batch of decoded events.
// Implementations must be safe to call from a single goroutine.
type Handler interface {
	// Handle processes one event. Return non-nil error to log and skip.
	Handle(ctx context.Context, event schema.Event) error
	// Flush writes any buffered batch to the backing store.
	Flush(ctx context.Context) error
}

// BaseConsumer manages the Kafka poll loop, batching, and graceful shutdown.
type BaseConsumer struct {
	bootstrapServers string
	topic            string
	groupID          string
	batchSize        int
	flushInterval    time.Duration
	handler          Handler
	logger           *zap.Logger
}

// NewBaseConsumer creates a BaseConsumer.
func NewBaseConsumer(
	bootstrapServers, topic, groupID string,
	batchSize int,
	flushInterval time.Duration,
	handler Handler,
	logger *zap.Logger,
) *BaseConsumer {
	return &BaseConsumer{
		bootstrapServers: bootstrapServers,
		topic:            topic,
		groupID:          groupID,
		batchSize:        batchSize,
		flushInterval:    flushInterval,
		handler:          handler,
		logger:           logger,
	}
}

// Run starts the poll loop. Blocks until ctx is cancelled or SIGTERM.
// On shutdown: flushes buffered events, commits offsets, closes the reader.
func (b *BaseConsumer) Run(ctx context.Context) {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{b.bootstrapServers},
		Topic:   b.topic,
		GroupID: b.groupID,
	})
	defer r.Close()

	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()

	buffered := 0

	flush := func() {
		if buffered == 0 {
			return
		}
		var backoff time.Duration = time.Second
		for attempt := 1; attempt <= 5; attempt++ {
			if err := b.handler.Flush(ctx); err != nil {
				b.logger.Error("flush error", zap.Int("attempt", attempt), zap.Error(err))
				if attempt < 5 {
					time.Sleep(backoff)
					backoff *= 2
					if backoff > 30*time.Second {
						backoff = 30 * time.Second
					}
				}
			} else {
				break
			}
		}
		buffered = 0
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case <-ticker.C:
			flush()
		default:
		}

		fetchCtx, cancel := context.WithTimeout(ctx, b.flushInterval)
		msg, err := r.FetchMessage(fetchCtx)
		cancel()

		if err != nil {
			if ctx.Err() != nil {
				flush()
				return
			}
			continue
		}

		event, err := schema.FromJSON(msg.Value)
		if err != nil {
			b.logger.Error("failed to parse event", zap.Error(err), zap.ByteString("raw", msg.Value[:min(len(msg.Value), 200)]))
			if cmErr := r.CommitMessages(ctx, msg); cmErr != nil {
				b.logger.Warn("commit failed", zap.Error(cmErr))
			}
			continue
		}

		if err := b.handler.Handle(ctx, event); err != nil {
			b.logger.Error("handle error", zap.Error(err), zap.String("span_id", event.SpanID))
		} else {
			buffered++
		}

		if err := r.CommitMessages(ctx, msg); err != nil {
			b.logger.Warn("commit failed", zap.Error(err))
		}

		if buffered >= b.batchSize {
			flush()
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Ptr returns a pointer to a value. Useful for optional schema fields.
func Ptr[T any](v T) *T { return &v }

// DSN builds a MySQL DSN string.
func DSN(user, password, host string, port int, database string) string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&interpolateParams=true", user, password, host, port, database)
}
