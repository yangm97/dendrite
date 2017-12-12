// Copyright 2017 Vector Creations Ltd
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package common

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"

	opentracing "github.com/opentracing/opentracing-go"
	sarama "gopkg.in/Shopify/sarama.v1"
)

// A PartitionOffset is the offset into a partition of the input log.
type PartitionOffset struct {
	// The ID of the partition.
	Partition int32
	// The offset into the partition.
	Offset int64
}

// A PartitionStorer has the storage APIs needed by the consumer.
type PartitionStorer interface {
	// PartitionOffsets returns the offsets the consumer has reached for each partition.
	PartitionOffsets(ctx context.Context, topic string) ([]PartitionOffset, error)
	// SetPartitionOffset records where the consumer has reached for a partition.
	SetPartitionOffset(ctx context.Context, topic string, partition int32, offset int64) error
}

// A ContinualConsumer continually consumes logs even across restarts. It requires a PartitionStorer to
// remember the offset it reached.
type ContinualConsumer struct {
	// The kafkaesque topic to consume events from.
	// This is the name used in kafka to identify the stream to consume events from.
	topic string
	// A kafkaesque stream consumer providing the APIs for talking to the event source.
	// The interface is taken from a client library for Apache Kafka.
	// But any equivalent event streaming protocol could be made to implement the same interface.
	consumer sarama.Consumer
	// A thing which can load and save partition offsets for a topic.
	partitionStore PartitionStorer
	// ProcessMessage is a function which will be called for each message in the log. Return an error to
	// stop processing messages. See ErrShutdown for specific control signals.
	handler ProcessKafkaMessage
	tracer  opentracing.Tracer
}

// ErrShutdown can be returned from ContinualConsumer.ProcessMessage to stop the ContinualConsumer.
var ErrShutdown = fmt.Errorf("shutdown")

func NewContinualConsumer(
	topic string,
	consumer sarama.Consumer,
	partitionStore PartitionStorer,
	handler ProcessKafkaMessage,
	tracer opentracing.Tracer,
) ContinualConsumer {
	return ContinualConsumer{
		topic:          topic,
		consumer:       consumer,
		partitionStore: partitionStore,
		handler:        handler,
		tracer:         tracer,
	}
}

// Start starts the consumer consuming.
// Starts up a goroutine for each partition in the kafka stream.
// Returns nil once all the goroutines are started.
// Returns an error if it can't start consuming for any of the partitions.
func (c *ContinualConsumer) Start() error {
	offsets := map[int32]int64{}

	partitions, err := c.consumer.Partitions(c.topic)
	if err != nil {
		return err
	}
	for _, partition := range partitions {
		// Default all the offsets to the beginning of the stream.
		offsets[partition] = sarama.OffsetOldest
	}

	storedOffsets, err := c.partitionStore.PartitionOffsets(context.TODO(), c.topic)
	if err != nil {
		return err
	}
	for _, offset := range storedOffsets {
		// We've already processed events from this partition so advance the offset to where we got to.
		// ConsumePartition will start streaming from the message with the given offset (inclusive),
		// so increment 1 to avoid getting the same message a second time.
		offsets[offset.Partition] = 1 + offset.Offset
	}

	var partitionConsumers []sarama.PartitionConsumer
	for partition, offset := range offsets {
		pc, err := c.consumer.ConsumePartition(c.topic, partition, offset)
		if err != nil {
			for _, p := range partitionConsumers {
				p.Close() // nolint: errcheck
			}
			return err
		}
		partitionConsumers = append(partitionConsumers, pc)
	}
	for _, pc := range partitionConsumers {
		go c.consumePartition(pc)
	}

	return nil
}

// consumePartition consumes the room events for a single partition of the kafkaesque stream.
func (c *ContinualConsumer) consumePartition(pc sarama.PartitionConsumer) {
	defer pc.Close() // nolint: errcheck
	for message := range pc.Messages() {
		err := c.processMessage(message)
		// Shutdown if we were told to do so.
		if err == ErrShutdown {
			return
		}
	}
}

func (c *ContinualConsumer) processMessage(message *sarama.ConsumerMessage) error {
	span, ctx := c.DeserialiseOpentracingSpan(context.Background(), message)
	defer span.Finish()

	msgErr := c.handler.ProcessMessage(ctx, message)
	// Advance our position in the stream so that we will start at the right position after a restart.
	if err := c.partitionStore.SetPartitionOffset(ctx, c.topic, message.Partition, message.Offset); err != nil {
		panic(fmt.Errorf("the ContinualConsumer failed to SetPartitionOffset: %s", err))
	}

	if msgErr == ErrShutdown {
		return msgErr
	}

	if msgErr != nil {
		logrus.WithError(msgErr).WithField("topic", c.topic).Warn("Failed to handle message")
		ext.Error.Set(span, true)
		span.LogFields(log.Error(msgErr))
	}

	return nil
}

type ProcessKafkaMessage interface {
	ProcessMessage(ctx context.Context, msg *sarama.ConsumerMessage) error
}

const kafkaOpentracingHeaderKey string = "opentracingSpanContext"

func (c *ContinualConsumer) DeserialiseOpentracingSpan(
	ctx context.Context, msg *sarama.ConsumerMessage,
) (opentracing.Span, context.Context) {
	spanContext := c.spanContextFromMessage(msg)

	span := c.tracer.StartSpan(
		"process_message",
		opentracing.FollowsFrom(spanContext),
		ext.SpanKindConsumer,
		opentracing.Tag{Key: "message_bus.destination", Value: c.topic},
	)

	return span, opentracing.ContextWithSpan(ctx, span)
}

func (c *ContinualConsumer) spanContextFromMessage(msg *sarama.ConsumerMessage) opentracing.SpanContext {
	var opentracingHeader []byte
	for _, record := range msg.Headers {
		if bytes.Equal(record.Key, []byte(kafkaOpentracingHeaderKey)) {
			opentracingHeader = record.Value
			break
		}
	}

	if len(opentracingHeader) == 0 {
		logrus.Warn("Failed to find opentracing header")
		return nil
	}

	var tmc opentracing.TextMapCarrier
	if err := json.Unmarshal(opentracingHeader, &tmc); err != nil {
		logrus.WithError(err).Error("Failed to unmarshal opentracing header")
		return nil
	}

	spanContext, err := c.tracer.Extract(opentracing.TextMap, tmc)
	if err != nil {
		logrus.WithError(err).Error("Failed to extract spancontext from header")
		return nil
	}

	return spanContext
}

func SerialiseOpentracingSpan(
	tracer opentracing.Tracer, ctx context.Context, msg *sarama.ProducerMessage,
) {
	tmc := make(opentracing.TextMapCarrier)

	span := opentracing.SpanFromContext(ctx)
	if span == nil {
		logrus.Warn("Failed to find span in context")
		return
	}

	err := tracer.Inject(span.Context(), opentracing.TextMap, tmc)
	if err != nil {
		logrus.Warn("Failed to inject span")
		return
	}

	outputBytes, err := json.Marshal(tmc)
	if err != nil {
		logrus.Warn("Failed to marshal span")
		return
	}

	msg.Headers = append(msg.Headers, sarama.RecordHeader{
		Key:   []byte(kafkaOpentracingHeaderKey),
		Value: outputBytes,
	})
}
