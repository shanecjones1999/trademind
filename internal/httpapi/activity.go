package httpapi

import (
	"net/http"
	"strconv"

	"github.com/shanecjones1999/trademind/internal/paper"
)

const (
	defaultListLimit = 50
	maximumListLimit = 200
)

func (s *Server) activity(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(writer, http.MethodGet)
		return
	}
	if !s.authenticationConfigured(writer) {
		return
	}
	if s.googleAuth.Accounts == nil {
		writeError(writer, http.StatusServiceUnavailable, "paper accounts are not configured")
		return
	}

	session, ok := s.sessionFromRequest(writer, request)
	if !ok {
		return
	}

	limit, err := parseListLimit(request.URL.Query().Get("limit"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "limit must be between 1 and 200")
		return
	}

	activity, err := s.googleAuth.Accounts.ListActivity(request.Context(), session.Subject, limit)
	if err != nil {
		s.logger.Error("list account activity", "error", err)
		writeError(writer, http.StatusServiceUnavailable, "unable to load account activity")
		return
	}
	writeJSON(writer, http.StatusOK, activity)
}

func parseListLimit(rawLimit string) (int, error) {
	if rawLimit == "" {
		return defaultListLimit, nil
	}
	limit, err := strconv.Atoi(rawLimit)
	if err != nil || limit < 1 || limit > maximumListLimit {
		return 0, paper.ErrInvalidTransaction
	}
	return limit, nil
}

func parseListOffset(rawOffset string) (int, error) {
	if rawOffset == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(rawOffset)
	if err != nil || offset < 0 {
		return 0, paper.ErrInvalidTransaction
	}
	return offset, nil
}
