// Copyright 2017 New Vector Ltd
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

	"github.com/matrix-org/dendrite/paginationapi/storage"
	"github.com/matrix-org/dendrite/roomserver/api"
)

// OutputRoomEventConsumer consumes events that originated in the room server.
type OutputRoomEventConsumer struct {
	db *storage.PaginationAPIDatabase
}

// NewOutputRoomEventConsumer creates a new OutputRoomEventConsumer.
func NewOutputRoomEventConsumer(db *storage.PaginationAPIDatabase) *OutputRoomEventConsumer {
	s := &OutputRoomEventConsumer{db: db}

	return s
}

// ProcessNewRoomEvent implements output.ProcessOutputEventHandler
func (s *OutputRoomEventConsumer) ProcessNewRoomEvent(
	ctx context.Context, msg *api.OutputNewRoomEvent,
) error {

	return s.db.AddEvent(ctx, &msg.Event)
}

// ProcessNewInviteEvent implements output.ProcessOutputEventHandler
func (s *OutputRoomEventConsumer) ProcessNewInviteEvent(
	ctx context.Context, msg *api.OutputNewInviteEvent,
) error {
	return nil
}

// ProcessRetireInviteEvent implements output.ProcessOutputEventHandler
func (s *OutputRoomEventConsumer) ProcessRetireInviteEvent(
	ctx context.Context, msg *api.OutputRetireInviteEvent,
) error {
	return nil
}
