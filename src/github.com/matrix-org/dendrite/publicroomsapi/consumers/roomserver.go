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

	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/publicroomsapi/storage"
	"github.com/matrix-org/dendrite/roomserver/api"
	log "github.com/sirupsen/logrus"
)

// OutputRoomEventConsumer consumes events that originated in the room server.
type OutputRoomEventConsumer struct {
	db    *storage.PublicRoomsServerDatabase
	query api.RoomserverQueryAPI
}

// NewOutputRoomEventConsumer creates a new OutputRoomEventConsumer.
func NewOutputRoomEventConsumer(
	cfg *config.Dendrite,
	store *storage.PublicRoomsServerDatabase,
	queryAPI api.RoomserverQueryAPI,
) *OutputRoomEventConsumer {
	s := &OutputRoomEventConsumer{
		db:    store,
		query: queryAPI,
	}

	return s
}

// ProcessNewRoomEvent implements output.ProcessOutputEventHandler
func (s *OutputRoomEventConsumer) ProcessNewRoomEvent(ctx context.Context, event *api.OutputNewRoomEvent) error {
	addQueryReq := api.QueryEventsByIDRequest{EventIDs: event.AddsStateEventIDs}
	var addQueryRes api.QueryEventsByIDResponse
	if err := s.query.QueryEventsByID(context.TODO(), &addQueryReq, &addQueryRes); err != nil {
		log.Warn(err)
		return err
	}

	remQueryReq := api.QueryEventsByIDRequest{EventIDs: event.RemovesStateEventIDs}
	var remQueryRes api.QueryEventsByIDResponse
	if err := s.query.QueryEventsByID(context.TODO(), &remQueryReq, &remQueryRes); err != nil {
		log.Warn(err)
		return err
	}

	return s.db.UpdateRoomFromEvents(context.TODO(), addQueryRes.Events, remQueryRes.Events)
}

// ProcessNewInviteEvent implements output.ProcessOutputEventHandler
func (s *OutputRoomEventConsumer) ProcessNewInviteEvent(ctx context.Context, event *api.OutputNewInviteEvent) error {
	return nil
}

// ProcessRetireInviteEvent implements output.ProcessOutputEventHandler
func (s *OutputRoomEventConsumer) ProcessRetireInviteEvent(ctx context.Context, event *api.OutputRetireInviteEvent) error {
	return nil
}
