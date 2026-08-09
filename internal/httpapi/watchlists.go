package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/shanecjones1999/trademind/internal/market"
	"github.com/shanecjones1999/trademind/internal/paper"
)

const maxWatchlistRequestBytes = 4 << 10

type createWatchlistRequest struct {
	Name string `json:"name"`
}

type watchlistSymbolRequest struct {
	Symbol string `json:"symbol"`
}

func (s *Server) watchlists(writer http.ResponseWriter, request *http.Request) {
	if !s.watchlistsConfigured(writer) {
		return
	}
	session, ok := s.sessionFromRequest(writer, request)
	if !ok {
		return
	}

	switch request.Method {
	case http.MethodGet:
		watchlists, err := s.googleAuth.Watchlists.ListWatchlists(request.Context(), session.Subject)
		if err != nil {
			s.logger.Error("list watchlists", "error", err)
			writeError(writer, http.StatusServiceUnavailable, "unable to load watchlists")
			return
		}
		writeJSON(writer, http.StatusOK, watchlists)
	case http.MethodPost:
		var body createWatchlistRequest
		if !decodeRequestJSON(writer, request, &body) {
			return
		}
		watchlist, err := s.googleAuth.Watchlists.CreateWatchlist(request.Context(), session.Subject, body.Name)
		if err != nil {
			s.writeWatchlistError(writer, err)
			return
		}
		writeJSON(writer, http.StatusCreated, watchlist)
	default:
		methodNotAllowed(writer, http.MethodGet+", "+http.MethodPost)
	}
}

func (s *Server) watchlistSymbols(writer http.ResponseWriter, request *http.Request) {
	if !s.watchlistsConfigured(writer) {
		return
	}
	session, ok := s.sessionFromRequest(writer, request)
	if !ok {
		return
	}

	segments := strings.Split(strings.TrimPrefix(request.URL.Path, watchlistsPath+"/"), "/")
	if len(segments) < 2 || segments[0] == "" || segments[1] != "symbols" {
		writeError(writer, http.StatusNotFound, "watchlist not found")
		return
	}
	watchlistID := segments[0]

	switch request.Method {
	case http.MethodPost:
		if len(segments) != 2 {
			writeError(writer, http.StatusNotFound, "watchlist not found")
			return
		}
		var body watchlistSymbolRequest
		if !decodeRequestJSON(writer, request, &body) {
			return
		}
		symbol, err := market.NormalizeSymbol(body.Symbol)
		if err != nil {
			writeError(writer, http.StatusBadRequest, "symbol is invalid")
			return
		}
		watchlistSymbol, err := s.googleAuth.Watchlists.AddWatchlistSymbol(request.Context(), session.Subject, watchlistID, symbol)
		if err != nil {
			s.writeWatchlistError(writer, err)
			return
		}
		writeJSON(writer, http.StatusCreated, watchlistSymbol)
	case http.MethodDelete:
		if len(segments) != 3 {
			writeError(writer, http.StatusNotFound, "watchlist symbol not found")
			return
		}
		symbol, err := market.NormalizeSymbol(segments[2])
		if err != nil {
			writeError(writer, http.StatusNotFound, "watchlist symbol not found")
			return
		}
		if err := s.googleAuth.Watchlists.RemoveWatchlistSymbol(request.Context(), session.Subject, watchlistID, symbol); err != nil {
			s.writeWatchlistError(writer, err)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(writer, http.MethodPost+", "+http.MethodDelete)
	}
}

func (s *Server) watchlistsConfigured(writer http.ResponseWriter) bool {
	if !s.authenticationConfigured(writer) {
		return false
	}
	if s.googleAuth.Watchlists == nil {
		writeError(writer, http.StatusServiceUnavailable, "watchlists are not configured")
		return false
	}
	return true
}

func (s *Server) writeWatchlistError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, paper.ErrWatchlistNotFound):
		writeError(writer, http.StatusNotFound, "watchlist not found")
	case errors.Is(err, paper.ErrDuplicateWatchlistSymbol):
		writeError(writer, http.StatusConflict, "watchlist already contains symbol")
	case errors.Is(err, paper.ErrInvalidTransaction):
		writeError(writer, http.StatusBadRequest, "watchlist details are invalid")
	default:
		s.logger.Error("update watchlist", "error", err)
		writeError(writer, http.StatusServiceUnavailable, "unable to update watchlist")
	}
}

func decodeRequestJSON(writer http.ResponseWriter, request *http.Request, destination any) bool {
	request.Body = http.MaxBytesReader(writer, request.Body, maxWatchlistRequestBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(writer, http.StatusBadRequest, "request body is invalid")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(writer, http.StatusBadRequest, "request body must contain one JSON object")
		return false
	}
	return true
}
