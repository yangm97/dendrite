// Copyright 2017 New Ltd
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

package storage

import (
	"context"
	"database/sql"

	// Import the postgres database driver.
	_ "github.com/lib/pq"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/gomatrixserverlib"
)

// SyncServerDatabase represents a sync server database
type PaginationAPIDatabase struct {
	db *sql.DB
	common.PartitionOffsetStatements
	eventBuckets eventBucketsStatements
}

// NewSyncServerDatabase creates a new sync server database
func NewPaginationAPIDatabase(dataSourceName string) (*PaginationAPIDatabase, error) {
	var d PaginationAPIDatabase
	var err error
	if d.db, err = sql.Open("postgres", dataSourceName); err != nil {
		return nil, err
	}
	if err = d.PartitionOffsetStatements.Prepare(d.db, "paginationapi"); err != nil {
		return nil, err
	}
	if err = d.eventBuckets.prepare(d.db); err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *PaginationAPIDatabase) Paginate(ctx context.Context, eventID string, limit uint) ([]string, error) {
	return nil, nil
}

func (s *PaginationAPIDatabase) AddEvent(ctx context.Context, event *gomatrixserverlib.Event) error {
	// 1. figure out which buckets have events that point to that event (take
	//    smallest), otherwise look at buckets that prev_events are in
	// 2. find the largest bucket.
	// 3. what if one of the prev_events don't have buckets? Add to current
	//    bucket or not? should we have sentinel events? What if we can't get a
	//    sentinel? I guess we just ignore that edge.
	// 4. Check if the bucket is too big, if it is make a new one.
	// 5. Add event to bucket.

	// TODO: Handle having multiple latest events?

	return common.WithTransaction(s.db, func(txn *sql.Tx) error {
		eventID := event.EventID()

		minParentBucketID, err := s.eventBuckets.getEarliestBucketForEvent(ctx, txn, eventID)
		if err != nil {
			return err
		}

		maxChildBucketID, err := s.eventBuckets.getLatestBucketForEvents(ctx, txn, event.PrevEventIDs())
		if err != nil {
			return err
		}

		maxCurrentBucketID, err := s.eventBuckets.getLatestBucketID(ctx, txn, event.RoomID())
		if err != nil {
			return err
		}

		result := CalculateEventInsertion(minParentBucketID, maxChildBucketID, maxCurrentBucketID, true)

		switch result.Behaviour {
		case AppendAfter:
			return s.eventBuckets.insertAfterBucket(ctx, txn, event, result.BucketID)
		case AppendInto:
			return s.eventBuckets.insertIntoOrAfterBucket(ctx, txn, event, result.BucketID)
		case PrependInto:
			return s.eventBuckets.insertIntoOrBeforeBucket(ctx, txn, event, result.BucketID)
		case CreateNew:
			return s.eventBuckets.createBucketAndInsert(ctx, txn, event)
		case Quarantine:
			panic("Quarantine not implemented")
		default:
			panic("Unknown behaviour")
		}
	})
}

type EventInsertionBehaviour = uint

const (
	AppendAfter EventInsertionBehaviour = iota
	AppendInto
	PrependInto
	CreateNew
	Quarantine
)

type EventInsertionOutput struct {
	Behaviour EventInsertionBehaviour
	BucketID  uint
}

func CalculateEventInsertion(
	minParentBucketID *uint,
	maxChildBucketID *uint,
	maxCurrentBucketID *uint,
	isNewEvent bool,
) EventInsertionOutput {
	if maxCurrentBucketID == nil {
		return EventInsertionOutput{
			Behaviour: CreateNew,
		}
	}

	// TODO: Handle if both minParentBucketID and maxChildBucketID are non-nil

	if minParentBucketID != nil {
		return EventInsertionOutput{
			BucketID:  *minParentBucketID,
			Behaviour: PrependInto,
		}
	}

	if maxChildBucketID != nil && *maxCurrentBucketID == *maxChildBucketID {
		return EventInsertionOutput{
			BucketID:  *maxChildBucketID,
			Behaviour: AppendInto,
		}
	}

	if isNewEvent {
		return EventInsertionOutput{
			BucketID:  *maxCurrentBucketID,
			Behaviour: AppendAfter,
		}
	}

	return EventInsertionOutput{
		Behaviour: Quarantine,
	}
}
