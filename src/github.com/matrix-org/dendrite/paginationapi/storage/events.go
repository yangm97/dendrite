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

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/gomatrixserverlib"
)

const eventBucketsSchema = `
CREATE TABLE IF NOT EXISTS paginationapi_events (
	id BIGSERIAL PRIMARY KEY,
	event_id TEXT NOT NULL,
	bucket_id BIGINT
);

CREATE UNIQUE INDEX IF NOT EXISTS paginationapi_events_event_id_idx ON paginationapi_events(event_id);

CREATE TABLE IF NOT EXISTS paginationapi_event_edges (
	parent_id BIGINT NOT NULL,
	child_id BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS paginationapi_event_edges_parent_idx ON paginationapi_event_edges(parent_id);
CREATE INDEX IF NOT EXISTS paginationapi_event_edges_child_idx ON paginationapi_event_edges(child_id);

CREATE TABLE IF NOT EXISTS paginationapi_rooms (
	id BIGSERIAL PRIMARY KEY,
	room_id TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS paginationapi_buckets (
	id BIGSERIAL PRIMARY KEY,
	room_id BIGINT NOT NULL,
	previous_bucket BIGINT,
	missing_event_ids TEXT[]
);

CREATE INDEX IF NOT EXISTS paginationapi_buckets_previous_idx ON paginationapi_buckets(previous_bucket);

CREATE TABLE IF NOT EXISTS paginationapi_latest_buckets (
	room_id BIGINT NOT NULL,
	bucket_id BIGINT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS paginationapi_latest_buckets_idx ON paginationapi_latest_buckets(room_id);
`

const getBucketForEventSQL = `SELECT bucket_id FROM paginationapi_events WHERE event_id = $1`

const getBucketsForParentEventsSQL = `
SELECT
	p.bucket_id
FROM paginationapi_event_edges
	INNER JOIN paginationapi_events AS p
		ON parent_id = p.id
	INNER JOIN paginationapi_events AS c
		ON child_id = c.id
WHERE
	c.event_id = $1
	AND p.bucket_id IS NOT NULL
`

const latestBucketSQL = `
SELECT bucket_id FROM paginationapi_latest_buckets AS l
INNER JOIN paginationapi_rooms AS r ON r.id = l.room_id
WHERE r.room_id = $1
`

type eventBucketsStatements struct {
	getBucketForEventStmt         *sql.Stmt
	getBucketsForParentEventsStmt *sql.Stmt
	latestBucketStmt              *sql.Stmt
}

func (s *eventBucketsStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(eventBucketsSchema)
	if err != nil {
		return
	}
	if s.getBucketForEventStmt, err = db.Prepare(getBucketForEventSQL); err != nil {
		return
	}
	if s.getBucketsForParentEventsStmt, err = db.Prepare(getBucketsForParentEventsSQL); err != nil {
		return
	}
	if s.latestBucketStmt, err = db.Prepare(latestBucketSQL); err != nil {
		return
	}
	return
}

func (s *eventBucketsStatements) isAfter(ctx context.Context, txn *sql.Tx, a uint, b uint) (bool, error) {
	return a < b, nil
}

func (s *eventBucketsStatements) getEarliestBucketForEvent(ctx context.Context, txn *sql.Tx, eventID string) (*uint, error) {
	rows, err := common.TxStmt(txn, s.getBucketsForParentEventsStmt).QueryContext(ctx, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close() // nolint: errcheck

	var minBucketID *uint
	for rows.Next() {
		var bucketID uint
		if err = rows.Scan(&bucketID); err != nil {
			return nil, err
		}

		if minBucketID == nil {
			minBucketID = &bucketID
			continue
		}

		isAfter, err := s.isAfter(ctx, txn, bucketID, *minBucketID)
		if err != nil {
			return nil, err
		}

		if isAfter {
			minBucketID = &bucketID
		}
	}

	return minBucketID, nil
}

func (s *eventBucketsStatements) getLatestBucketForEvents(ctx context.Context, txn *sql.Tx, eventIDs []string) (*uint, error) {
	var maxBucketID *uint

	// FIXME: This can be done via postgres arrays! \o/

	stmt := common.TxStmt(txn, s.getBucketForEventStmt)
	for _, eventID := range eventIDs {
		var bucketID *uint
		err := stmt.QueryRowContext(ctx, eventID).Scan(&bucketID)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}

		if bucketID == nil {
			continue
		}

		if maxBucketID == nil {
			maxBucketID = bucketID
		}

		isAfter, err := s.isAfter(ctx, txn, *maxBucketID, *bucketID)
		if err != nil {
			return nil, err
		}
		if isAfter {
			maxBucketID = bucketID
		}
	}

	return nil, nil
}

func (s *eventBucketsStatements) getLatestBucketID(ctx context.Context, txn *sql.Tx, roomID string) (*uint, error) {
	stmt := common.TxStmt(txn, s.latestBucketStmt)

	var bucketID sql.NullInt64
	err := stmt.QueryRowContext(ctx, roomID).Scan(&bucketID)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	if bucketID.Valid {
		result := uint(bucketID.Int64)
		return &result, nil
	}

	return nil, nil
}

func (s *eventBucketsStatements) insertIntoOrBeforeBucket(
	ctx context.Context, txn *sql.Tx, event *gomatrixserverlib.Event, bucketID uint,
) error {
	panic("not implemented")
}

func (s *eventBucketsStatements) insertIntoOrAfterBucket(
	ctx context.Context, txn *sql.Tx, event *gomatrixserverlib.Event, bucketID uint,
) error {
	// TODO: Create a new bucket if there are already too many events in that
	// bucket

	_, err := txn.ExecContext(
		ctx, "INSERT INTO paginationapi_events (event_id, bucket_id) VALUES ($1, $2)",
		event.EventID(), bucketID,
	)
	return err
}

func (s *eventBucketsStatements) insertAfterBucket(
	ctx context.Context, txn *sql.Tx, event *gomatrixserverlib.Event, bucketID uint,
) error {
	var roomNID uint
	err := txn.QueryRowContext(
		ctx, "SELECT room_id FROM paginationapi_rooms WHERE room_id = $1", event.RoomID(),
	).Scan(&roomNID)

	if err != nil {
		return err
	}

	// TODO: Add missing_event_ids

	var newBucketID uint
	err = txn.QueryRowContext(
		ctx, "INSERT INTO paginationapi_buckets (room_id, previous_bucket) VALUES ($1, $2) RETURNING id",
		roomNID, bucketID,
	).Scan(&newBucketID)

	if err != nil {
		return err
	}

	_, err = txn.ExecContext(
		ctx, "INSERT INTO paginationapi_latest_buckets (room_id, bucket_id) VALUES ($1, $2) ON CONFLICT (room_id) DO UPDATE SET bucket_id = $2",
		roomNID, newBucketID,
	)

	return err
}

func (s *eventBucketsStatements) createBucketAndInsert(
	ctx context.Context, txn *sql.Tx, event *gomatrixserverlib.Event,
) error {
	var roomNID uint
	err := txn.QueryRowContext(
		ctx, "SELECT room_id FROM paginationapi_rooms WHERE room_id = $1", event.RoomID(),
	).Scan(&roomNID)

	if err != nil {
		return err
	}

	var newBucketID uint
	err = txn.QueryRowContext(
		ctx, "INSERT INTO paginationapi_buckets (room_id) VALUES ($1) RETURNING id",
		roomNID,
	).Scan(&newBucketID)

	if err != nil {
		return err
	}

	_, err = txn.ExecContext(
		ctx, "INSERT INTO paginationapi_latest_buckets (room_id, bucket_id) VALUES ($1, $2)",
		roomNID, newBucketID,
	)

	return err
}
