package metrics

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// CollectionName is the MongoDB collection holding hourly counter documents.
const CollectionName = "stats_hourly"

// bucketKey identifies one (metric, hour) counter bucket.
type bucketKey struct {
	Metric Metric
	Hour   time.Time // truncated to hour, UTC
}

// HourlyStat is one hourly counter document (one per metric per hour).
type HourlyStat struct {
	ID        string    `bson:"_id"` // 纯 UUID，不承载业务语义
	Metric    string    `bson:"metric"`
	Hour      time.Time `bson:"hour"` // 小时桶起点（UTC）
	Value     int64     `bson:"value"`
	UpdatedAt time.Time `bson:"updated_at"`
}

// MongoCounter is the buffered MongoDB implementation of Counter. It
// accumulates deltas in an in-memory map (O(1) per Incr, no IO) and flushes
// them to MongoDB in batches on a fixed interval, using a swap pattern so the
// flush never blocks increments. It is safe for concurrent use.
type MongoCounter struct {
	mu            sync.Mutex
	buffer        map[bucketKey]int64
	coll          *mongo.Collection
	flushInterval time.Duration
	stop          chan struct{}
	done          chan struct{}
}

// NewCounter creates a buffered counter backed by the stats_hourly collection
// and starts its background flusher. flushInterval <= 0 defaults to 5s.
func NewCounter(db *mongo.Database, flushInterval time.Duration) *MongoCounter {
	if flushInterval <= 0 {
		flushInterval = 5 * time.Second
	}
	c := &MongoCounter{
		buffer:        make(map[bucketKey]int64),
		coll:          db.Collection(CollectionName),
		flushInterval: flushInterval,
		stop:          make(chan struct{}),
		done:          make(chan struct{}),
	}
	go c.loop()
	return c
}

func (c *MongoCounter) loop() {
	ticker := time.NewTicker(c.flushInterval)
	defer ticker.Stop()
	defer close(c.done)
	for {
		select {
		case <-ticker.C:
			c.flush(context.Background())
		case <-c.stop:
			c.flush(context.Background())
			return
		}
	}
}

// Incr accumulates delta for the metric into the in-memory buffer. It performs
// no IO: only a map write guarded by a mutex (O(1)). The actual database write
// happens on the next flush.
func (c *MongoCounter) Incr(_ context.Context, m Metric, at time.Time, delta int64) error {
	if delta == 0 {
		return nil
	}
	key := bucketKey{Metric: m, Hour: HourBucket(at)}
	c.mu.Lock()
	c.buffer[key] += delta
	c.mu.Unlock()
	return nil
}

// Stop flushes any buffered increments and stops the background flusher. It
// blocks until the final flush completes so callers lose nothing on shutdown.
func (c *MongoCounter) Stop() {
	close(c.stop)
	<-c.done
}

// flush swaps the buffer out (under lock, O(1)) and upserts each bucket into
// MongoDB outside the lock — never holding the mutex during IO.
func (c *MongoCounter) flush(ctx context.Context) {
	old := c.swapBuffer()
	for key, delta := range old {
		if err := upsertHourly(ctx, c.coll, key, delta); err != nil {
			// Statistical accounting: tolerate losing ≤flushInterval of a
			// single bucket's increment on a transient failure.
			log.Printf("[metrics] upsert %s @ %s: %v", key.Metric, key.Hour.Format(time.RFC3339), err)
		}
	}
}

// swapBuffer atomically swaps the in-memory buffer out for a fresh empty map
// and returns the drained increments. The swap is O(1); the caller performs
// any IO outside the lock.
func (c *MongoCounter) swapBuffer() map[bucketKey]int64 {
	c.mu.Lock()
	old := c.buffer
	c.buffer = make(map[bucketKey]int64)
	c.mu.Unlock()
	return old
}

// upsertHourly increments the hourly document for (metric, hour), creating it
// (with a UUID _id) on first write via the unique {metric,hour} index.
func upsertHourly(ctx context.Context, coll *mongo.Collection, key bucketKey, delta int64) error {
	filter := bson.M{"metric": string(key.Metric), "hour": key.Hour}
	update := bson.M{
		"$inc":         bson.M{"value": delta},
		"$set":         bson.M{"updated_at": time.Now()},
		"$setOnInsert": bson.M{"_id": uuid.New().String()},
	}
	_, err := coll.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	return err
}

// MongoReader is the MongoDB implementation of Reader. It aggregates hourly
// documents via the stats_hourly collection. Series buckets in Go (the hourly
// document count for a year is small, ≤8760/metric), avoiding MongoDB version
// dependencies.
type MongoReader struct {
	coll *mongo.Collection
}

// NewReader creates a reader backed by the stats_hourly collection.
func NewReader(db *mongo.Database) *MongoReader {
	return &MongoReader{coll: db.Collection(CollectionName)}
}

// Sum returns the total value of a metric over [since, until).
func (r *MongoReader) Sum(ctx context.Context, m Metric, since, until time.Time) (int64, error) {
	since, until = clampRange(since, until)
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{
			{Key: "metric", Value: string(m)},
			{Key: "hour", Value: bson.D{{Key: "$gte", Value: since}, {Key: "$lt", Value: until}}},
		}}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: nil},
			{Key: "total", Value: bson.D{{Key: "$sum", Value: "$value"}}},
		}}},
	}
	cur, err := r.coll.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cur.Close(ctx)
	var result struct {
		Total int64 `bson:"total"`
	}
	if cur.Next(ctx) {
		if err := cur.Decode(&result); err != nil {
			return 0, err
		}
	}
	return result.Total, cur.Err()
}

// Series returns the metric bucketed by granularity over [since, until). Hourly
// documents are fetched and bucketed in Go; empty buckets are preserved.
func (r *MongoReader) Series(ctx context.Context, m Metric, since, until time.Time, g Granularity) ([]Bucket, error) {
	since, until = clampRange(since, until)
	filter := bson.D{
		{Key: "metric", Value: string(m)},
		{Key: "hour", Value: bson.D{{Key: "$gte", Value: since}, {Key: "$lt", Value: until}}},
	}
	cur, err := r.coll.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	hourSums := make(map[time.Time]int64)
	for cur.Next(ctx) {
		var doc HourlyStat
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		hourSums[HourBucket(doc.Hour)] += doc.Value
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return bucketHours(hourSums, since, until, g), nil
}

// clampRange bounds the query window to [now-MaxRange, now] and fills zero
// values with the full-year default.
func clampRange(since, until time.Time) (time.Time, time.Time) {
	now := time.Now().UTC()
	if until.IsZero() {
		until = now
	}
	if since.IsZero() {
		since = now.Add(-MaxRange)
	}
	minSince := now.Add(-MaxRange)
	if since.Before(minSince) {
		since = minSince
	}
	if until.After(now) {
		until = now
	}
	return since, until
}
