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

package consumers

import (
	"context"
	"encoding/json"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/sync"
	log "github.com/sirupsen/logrus"
	sarama "gopkg.in/Shopify/sarama.v1"
)

// OutputClientDataConsumer consumes events that originated in the client API server.
type OutputClientDataConsumer struct {
	db       *storage.SyncServerDatabase
	notifier *sync.Notifier
}

// NewOutputClientDataConsumer creates a new OutputClientData consumer. Call Start() to begin consuming from room servers.
func NewOutputClientDataConsumer(
	cfg *config.Dendrite,
	n *sync.Notifier,
	store *storage.SyncServerDatabase,
) *OutputClientDataConsumer {
	s := &OutputClientDataConsumer{
		db:       store,
		notifier: n,
	}

	return s
}

// ProcessMessage is called when the sync server receives a new event from the client API server output log.
// ProcessMessage implements common.ProcessKafkaMessage
func (s *OutputClientDataConsumer) ProcessMessage(ctx context.Context, msg *sarama.ConsumerMessage) error {
	// Parse out the event JSON
	var output common.AccountData
	if err := json.Unmarshal(msg.Value, &output); err != nil {
		// If the message was invalid, log it and move on to the next message in the stream
		log.WithError(err).Errorf("client API server output log: message parse failure")
		return nil
	}

	log.WithFields(log.Fields{
		"type":    output.Type,
		"room_id": output.RoomID,
	}).Info("received data from client API server")

	syncStreamPos, err := s.db.UpsertAccountData(
		context.TODO(), string(msg.Key), output.RoomID, output.Type,
	)
	if err != nil {
		log.WithFields(log.Fields{
			"type":       output.Type,
			"room_id":    output.RoomID,
			log.ErrorKey: err,
		}).Panicf("could not save account data")
	}

	s.notifier.OnNewEvent(nil, string(msg.Key), syncStreamPos)

	return nil
}
