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

package routing

import (
	"net/http"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/roomserver/api"

	"github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/clientapi/httputil"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/paginationapi/storage"
	"github.com/matrix-org/util"
)

const pathPrefixR0 = "/_matrix/client/r0"

// Setup registers HTTP handlers with the given ServeMux. It also supplies the given http.Client
// to clients which need to make outbound HTTP requests.
func Setup(
	apiMux *mux.Router,
	queryAPI api.RoomserverQueryAPI,
	paginationDB *storage.PaginationAPIDatabase,
) {
	r0mux := apiMux.PathPrefix(pathPrefixR0).Subrouter()

	r0mux.Handle("/rooms/{roomID}/messages",
		common.MakeExternalAPI("room_messages", func(req *http.Request) util.JSONResponse {
			eventID := req.URL.Query().Get("from")

			logrus.WithField("eventID", eventID).Warn("Got event")

			eventIDs, err := paginationDB.Paginate(req.Context(), eventID, 10)
			if err != nil {
				return httputil.LogThenError(req, err)
			}

			var response api.QueryEventsByIDResponse
			err = queryAPI.QueryEventsByID(req.Context(), &api.QueryEventsByIDRequest{
				EventIDs: eventIDs,
			}, &response)
			if err != nil {
				return httputil.LogThenError(req, err)
			}

			return util.JSONResponse{Code: 200, JSON: map[string]interface{}{
				"event_ids": eventIDs,
				"chunk":     gomatrixserverlib.ToClientEvents(response.Events, gomatrixserverlib.FormatAll),
				"start":     eventID,
				"end":       eventIDs[len(eventIDs)-1],
			}}
		}),
	).Methods("GET", "OPTIONS")
}
