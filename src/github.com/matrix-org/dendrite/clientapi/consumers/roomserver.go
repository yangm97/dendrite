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

	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
)

// OutputRoomEventConsumer consumes events that originated in the room server.
type OutputRoomEventConsumer struct {
	db         *accounts.Database
	query      api.RoomserverQueryAPI
	serverName string
}

// NewOutputRoomEventConsumer creates a new OutputRoomEventConsumer.
func NewOutputRoomEventConsumer(
	cfg *config.Dendrite,
	store *accounts.Database,
	queryAPI api.RoomserverQueryAPI,
) *OutputRoomEventConsumer {

	s := &OutputRoomEventConsumer{
		db:         store,
		query:      queryAPI,
		serverName: string(cfg.Matrix.ServerName),
	}

	return s
}

// ProcessNewRoomEvent implements output.ProcessOutputEventHandler
func (s *OutputRoomEventConsumer) ProcessNewRoomEvent(ctx context.Context, event *api.OutputNewRoomEvent) error {
	ev := event.Event
	events, err := s.lookupStateEvents(event.AddsStateEventIDs, ev)
	if err != nil {
		return err
	}

	return s.db.UpdateMemberships(ctx, events, event.RemovesStateEventIDs)
}

// ProcessNewInviteEvent implements output.ProcessOutputEventHandler
func (s *OutputRoomEventConsumer) ProcessNewInviteEvent(ctx context.Context, event *api.OutputNewInviteEvent) error {
	return nil
}

// ProcessNewInviteEvent implements output.ProcessOutputEventHandler
func (s *OutputRoomEventConsumer) ProcessRetireInviteEvent(ctx context.Context, event *api.OutputRetireInviteEvent) error {
	return nil
}

// lookupStateEvents looks up the state events that are added by a new event.
func (s *OutputRoomEventConsumer) lookupStateEvents(
	addsStateEventIDs []string, event gomatrixserverlib.Event,
) ([]gomatrixserverlib.Event, error) {
	// Fast path if there aren't any new state events.
	if len(addsStateEventIDs) == 0 {
		// If the event is a membership update (e.g. for a profile update), it won't
		// show up in AddsStateEventIDs, so we need to add it manually
		if event.Type() == "m.room.member" {
			return []gomatrixserverlib.Event{event}, nil
		}
		return nil, nil
	}

	// Fast path if the only state event added is the event itself.
	if len(addsStateEventIDs) == 1 && addsStateEventIDs[0] == event.EventID() {
		return []gomatrixserverlib.Event{event}, nil
	}

	result := []gomatrixserverlib.Event{}
	missing := []string{}
	for _, id := range addsStateEventIDs {
		// Append the current event in the results if its ID is in the events list
		if id == event.EventID() {
			result = append(result, event)
		} else {
			// If the event isn't the current one, add it to the list of events
			// to retrieve from the roomserver
			missing = append(missing, id)
		}
	}

	// Request the missing events from the roomserver
	eventReq := api.QueryEventsByIDRequest{EventIDs: missing}
	var eventResp api.QueryEventsByIDResponse
	if err := s.query.QueryEventsByID(context.TODO(), &eventReq, &eventResp); err != nil {
		return nil, err
	}

	result = append(result, eventResp.Events...)

	return result, nil
}
