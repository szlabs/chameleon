package lib

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const (
	managementAPIStats = "/api/v1"
)

//APIHandler provides API for the management requests
type APIHandler struct {
	scheduler *Scheduler
}

//ServeHTTP serve http requests
func (h *APIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	apiPath := strings.TrimPrefix(r.RequestURI, managementAPIStats)
	switch apiPath {
	case "/stats":
		if err := h.handlePoolStatsRequest(w, r); err != nil {
			h.internalError(w, err)
		}
	}
}

//HandlePoolStatsRequest handle pool stats request
func (h *APIHandler) handlePoolStatsRequest(w http.ResponseWriter, r *http.Request) error {
	runtimes := h.scheduler.GetRuntimes()
	data, err := json.Marshal(&runtimes)
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusOK)
	w.Write(data)

	return nil
}

//IsMatchedRequests check if the requests are management requests
func (h *APIHandler) IsMatchedRequests(r *http.Request) bool {
	return r != nil && strings.Contains(r.RequestURI, managementAPIStats)
}

func (h *APIHandler) internalError(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte(fmt.Sprintf("error: %s", err.Error())))
}
