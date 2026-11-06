// Package journal stores compactable queue states in Redpanda. Audio lives on
// the gateway's persistent volume; queue records never contain user content.
package journal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
)

type Record struct {
	Operation, ID string
	Value         []byte
}
type Store interface {
	Append(context.Context, Record) error
	Replay(context.Context) ([]Record, error)
	Ping(context.Context) error
	Close()
}
type Kafka struct {
	client  *kgo.Client
	brokers []string
	topics  map[string]string
	mu      sync.Mutex
}

func New(brokers []string, stt, tts string) (*Kafka, error) {
	client, err := kgo.NewClient(kgo.SeedBrokers(brokers...), kgo.RequiredAcks(kgo.AllISRAcks()), kgo.RecordRetries(2), kgo.RecordDeliveryTimeout(5*time.Second), kgo.MaxBufferedRecords(64))
	if err != nil {
		return nil, err
	}
	return &Kafka{client: client, brokers: brokers, topics: map[string]string{"stt": stt, "tts": tts}}, nil
}
func (k *Kafka) Ping(ctx context.Context) error { return k.client.Ping(ctx) }
func (k *Kafka) Close()                         { k.client.Close() }
func (k *Kafka) Append(ctx context.Context, r Record) error {
	topic, ok := k.topics[r.Operation]
	if !ok {
		return errors.New("unknown queue")
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.client.ProduceSync(ctx, &kgo.Record{Topic: topic, Partition: 0, Key: []byte(r.ID), Value: r.Value}).FirstErr()
}

// Replay appends a barrier to each single-partition topic and reads through it.
// Startup never proceeds with a partial replay.
func (k *Kafka) Replay(ctx context.Context) ([]Record, error) {
	metadata, err := k.client.Request(ctx, &kmsg.MetadataRequest{Topics: []kmsg.MetadataRequestTopic{{Topic: strptr(k.topics["stt"])}, {Topic: strptr(k.topics["tts"])}}})
	if err != nil {
		return nil, err
	}
	for _, t := range metadata.(*kmsg.MetadataResponse).Topics {
		if t.ErrorCode != 0 || len(t.Partitions) != 1 || t.Partitions[0].Partition != 0 {
			return nil, errors.New("voice topics must exist with exactly one partition")
		}
	}
	offsets := map[string]map[int32]kgo.Offset{}
	reverse := map[string]string{}
	for op, topic := range k.topics {
		offsets[topic] = map[int32]kgo.Offset{0: kgo.NewOffset().AtStart()}
		reverse[topic] = op
	}
	reader, err := kgo.NewClient(kgo.SeedBrokers(k.brokers...), kgo.ConsumePartitions(offsets), kgo.FetchMaxWait(100*time.Millisecond))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	bytes := make([]byte, 16)
	if _, err = rand.Read(bytes); err != nil {
		return nil, err
	}
	barrier := hex.EncodeToString(bytes)
	for _, topic := range k.topics {
		if err = k.client.ProduceSync(ctx, &kgo.Record{Topic: topic, Key: []byte("_restore"), Value: []byte(barrier)}).FirstErr(); err != nil {
			return nil, err
		}
	}
	seen := map[string]bool{}
	latest := map[string]Record{}
	for len(seen) < len(k.topics) {
		fetch := reader.PollFetches(ctx)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errs := fetch.Errors(); len(errs) > 0 {
			return nil, fmt.Errorf("replay voice queue: %w", errs[0].Err)
		}
		for it := fetch.RecordIter(); !it.Done(); {
			r := it.Next()
			if seen[r.Topic] {
				continue
			}
			if string(r.Key) == "_restore" {
				if string(r.Value) == barrier {
					seen[r.Topic] = true
				}
				continue
			}
			key := r.Topic + "/" + string(r.Key)
			if len(r.Value) == 0 {
				delete(latest, key)
			} else {
				latest[key] = Record{Operation: reverse[r.Topic], ID: string(r.Key), Value: append([]byte(nil), r.Value...)}
			}
		}
	}
	result := make([]Record, 0, len(latest))
	for _, r := range latest {
		result = append(result, r)
	}
	return result, nil
}
func strptr(s string) *string { return &s }
