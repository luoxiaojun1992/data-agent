package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
	"github.com/redis/go-redis/v9"
)

const (
	streamKey     = "agent:task:queue"
	dlqKey        = "agent:task:dlq"
	consumerGroup = "worker-pool"
	maxRetries    = 3
)

// Stream wraps Redis Stream operations with consumer group support.
type Stream struct {
	client *redis.Client
}

// NewStream creates a Redis Stream queue wrapper.
func NewStream(client *redis.Client) (*Stream, error) {
	// Create consumer group if not exists
	err := client.XGroupCreateMkStream(context.Background(), streamKey, consumerGroup, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return nil, fmt.Errorf("create consumer group: %w", err)
	}
	return &Stream{client: client}, nil
}

// Enqueue adds a task to the Redis Stream.
func (s *Stream) Enqueue(ctx context.Context, t *task.Task) error {
	msg := task.QueueMessage{
		TaskID:     t.ID,
		SessionID:  t.SessionID,
		UserID:     t.UserID,
		Type:       t.Type,
		ModelID:    t.ModelID,
		SkillChain: t.SkillChain,
		Params:     t.Params,
		CreatedAt:  t.CreatedAt.Format(time.RFC3339),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal queue message: %w", err)
	}

	_, err = s.client.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		Values: map[string]interface{}{"data": string(data)},
	}).Result()
	if err != nil {
		return fmt.Errorf("xadd to stream: %w", err)
	}
	return nil
}

// Dequeue reads pending messages for the consumer group.
func (s *Stream) Dequeue(ctx context.Context, consumerID string, block time.Duration) ([]redis.XMessage, error) {
	msgs, err := s.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    consumerGroup,
		Consumer: consumerID,
		Streams:  []string{streamKey, ">"},
		Block:    block,
		Count:    1,
	}).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("xreadgroup: %w", err)
	}

	var result []redis.XMessage
	for _, stream := range msgs {
		result = append(result, stream.Messages...)
	}
	return result, nil
}

// Ack acknowledges a message as processed.
func (s *Stream) Ack(ctx context.Context, messageID string) error {
	return s.client.XAck(ctx, streamKey, consumerGroup, messageID).Err()
}

// MoveToDLQ moves a failed message to the dead letter queue.
func (s *Stream) MoveToDLQ(ctx context.Context, msgID string, data []byte) error {
	_, err := s.client.XAdd(ctx, &redis.XAddArgs{
		Stream: dlqKey,
		Values: map[string]interface{}{
			"data":        string(data),
			"failed_at":   time.Now().Format(time.RFC3339),
			"original_id": msgID,
		},
	}).Result()
	if err != nil {
		return fmt.Errorf("move to dlq: %w", err)
	}
	return nil
}

// ClaimPending claims pending messages (for recovery after worker crash).
func (s *Stream) ClaimPending(ctx context.Context, consumerID string, minIdle time.Duration) ([]redis.XMessage, error) {
	pending, err := s.client.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: streamKey,
		Group:  consumerGroup,
		Start:  "-",
		End:    "+",
		Count:  10,
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("xpending: %w", err)
	}

	var msgs []redis.XMessage
	for _, p := range pending {
		if p.Idle >= minIdle {
			claimed, err := s.client.XClaim(ctx, &redis.XClaimArgs{
				Stream:   streamKey,
				Group:    consumerGroup,
				Consumer: consumerID,
				MinIdle:  minIdle,
				Messages: []string{p.ID},
			}).Result()
			if err == nil {
				msgs = append(msgs, claimed...)
			}
		}
	}
	return msgs, nil
}
