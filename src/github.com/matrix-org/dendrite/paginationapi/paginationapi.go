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

package paginationapi

import (
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/common/basecomponent"
	"github.com/matrix-org/dendrite/paginationapi/consumers"
	"github.com/matrix-org/dendrite/paginationapi/storage"
	"github.com/sirupsen/logrus"
)

// SetupPaginationAPIComponent sets up and registers HTTP handlers for the PaginationAPI
// component.
func SetupPaginationAPIComponent(
	base *basecomponent.BaseDendrite,
	deviceDB *devices.Database,
) {
	tracer := base.CreateNewTracer("PaginationAPI")

	paginationDB, err := storage.NewPaginationAPIDatabase(string(base.Cfg.Database.PaginationAPI))
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to pagination api db")
	}

	handler := consumers.NewOutputRoomEventConsumer(paginationDB)
	base.StartRoomServerConsumer(tracer, paginationDB, handler)

	// routing.Setup(base.APIMux, deviceDB, publicRoomsDB)
}
