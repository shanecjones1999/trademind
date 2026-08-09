package market

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

type fakeCatalogSource struct {
	page      CatalogPage
	err       error
	scope     CatalogScope
	nextURL   string
	callCount int
}

func (s *fakeCatalogSource) ListCatalogPage(_ context.Context, scope CatalogScope, nextURL string) (CatalogPage, error) {
	s.scope = scope
	s.nextURL = nextURL
	s.callCount++
	return s.page, s.err
}

type fakeCatalogStore struct {
	states    map[string]CatalogScopeState
	run       CatalogRun
	applied   []CatalogInstrument
	appliedTo string
	nextURL   string
}

func (s *fakeCatalogStore) ScopeState(_ context.Context, scope CatalogScope) (CatalogScopeState, error) {
	return s.states[scope.Key], nil
}

func (s *fakeCatalogStore) StartOrResumeCatalogRun(_ context.Context, scope CatalogScope) (CatalogRun, error) {
	if s.run.ID == "" {
		s.run = CatalogRun{ID: "run-1", Status: "running"}
	}
	return s.run, nil
}

func (s *fakeCatalogStore) ApplyCatalogPage(_ context.Context, scope CatalogScope, run CatalogRun, instruments []CatalogInstrument, nextURL string) (CatalogRun, error) {
	s.appliedTo = scope.Key
	s.applied = instruments
	s.nextURL = nextURL
	run.NextURL = nextURL
	if nextURL == "" {
		run.Status = "completed"
	}
	s.run = run
	return run, nil
}

func TestDefaultCatalogScopesCoverMajorUSExchangesForBothTypes(t *testing.T) {
	scopes := DefaultCatalogScopes()
	if len(scopes) != 16 {
		t.Fatalf("scope count = %d, want 16", len(scopes))
	}
	wantFirst := newCatalogScope("XNAS", assetTypeCommonStock)
	wantLast := newCatalogScope("EDGX", assetTypeETF)
	if scopes[0] != wantFirst || scopes[len(scopes)-1] != wantLast {
		t.Fatalf("scope order = %#v ... %#v, want %#v ... %#v", scopes[0], scopes[len(scopes)-1], wantFirst, wantLast)
	}
}

func TestCatalogSynchronizerResumesAndPersistsOnePage(t *testing.T) {
	scope := newCatalogScope("XNAS", assetTypeCommonStock)
	source := &fakeCatalogSource{page: CatalogPage{
		Instruments: []CatalogInstrument{{
			Symbol:          "aapl",
			Name:            " Apple Inc. ",
			AssetType:       "cs",
			PrimaryExchange: "xnas",
			Active:          true,
		}},
		NextURL: "https://api.massive.com/v3/reference/tickers?cursor=next",
	}}
	store := &fakeCatalogStore{
		states: map[string]CatalogScopeState{},
		run:    CatalogRun{ID: "run-1", NextURL: "https://api.massive.com/v3/reference/tickers?cursor=current", Status: "running"},
	}
	synchronizer, err := NewCatalogSynchronizer(source, store)
	if err != nil {
		t.Fatalf("create synchronizer: %v", err)
	}

	result, err := synchronizer.SyncPage(context.Background(), scope)
	if err != nil {
		t.Fatalf("sync page: %v", err)
	}
	if result.CompletedRun {
		t.Fatal("result marked a paginated run complete")
	}
	if source.scope != scope || source.nextURL != "https://api.massive.com/v3/reference/tickers?cursor=current" {
		t.Fatalf("source request = (%#v, %q), want (%#v, current cursor)", source.scope, source.nextURL, scope)
	}
	if store.appliedTo != scope.Key || store.nextURL != source.page.NextURL {
		t.Fatalf("persisted page = (%q, %q), want scope and next cursor", store.appliedTo, store.nextURL)
	}
	want := CatalogInstrument{
		Symbol:          "AAPL",
		Name:            "Apple Inc.",
		AssetType:       "CS",
		PrimaryExchange: "XNAS",
		Active:          true,
	}
	if !reflect.DeepEqual(store.applied, []CatalogInstrument{want}) {
		t.Fatalf("persisted instruments = %#v, want %#v", store.applied, []CatalogInstrument{want})
	}
}

func TestCatalogSynchronizerRejectsProviderRecordsOutsideScope(t *testing.T) {
	scope := newCatalogScope("XNAS", assetTypeCommonStock)
	source := &fakeCatalogSource{page: CatalogPage{Instruments: []CatalogInstrument{{
		Symbol:          "MSFT",
		Name:            "Microsoft",
		AssetType:       assetTypeETF,
		PrimaryExchange: scope.Exchange,
		Active:          true,
	}}}}
	store := &fakeCatalogStore{states: map[string]CatalogScopeState{}}
	synchronizer, err := NewCatalogSynchronizer(source, store)
	if err != nil {
		t.Fatalf("create synchronizer: %v", err)
	}

	_, err = synchronizer.SyncPage(context.Background(), scope)
	if err == nil {
		t.Fatal("sync page succeeded for an out-of-scope instrument")
	}
	if len(store.applied) != 0 {
		t.Fatalf("out-of-scope instrument was persisted: %#v", store.applied)
	}
}

func TestCatalogSynchronizerNextScopePrioritizesRunningThenNewScopes(t *testing.T) {
	scopes := []CatalogScope{
		newCatalogScope("XNAS", assetTypeCommonStock),
		newCatalogScope("XNYS", assetTypeCommonStock),
		newCatalogScope("XASE", assetTypeCommonStock),
	}
	store := &fakeCatalogStore{states: map[string]CatalogScopeState{
		scopes[0].Key: {Exists: true, Run: CatalogRun{Status: "completed"}},
		scopes[1].Key: {Exists: true, Run: CatalogRun{Status: "running"}},
	}}
	synchronizer, err := NewCatalogSynchronizer(&fakeCatalogSource{}, store)
	if err != nil {
		t.Fatalf("create synchronizer: %v", err)
	}

	next, err := synchronizer.NextScope(context.Background(), scopes)
	if err != nil {
		t.Fatalf("select running scope: %v", err)
	}
	if next != scopes[1] {
		t.Fatalf("next scope = %#v, want running %#v", next, scopes[1])
	}

	store.states[scopes[1].Key] = CatalogScopeState{Exists: true, Run: CatalogRun{Status: "completed"}}
	next, err = synchronizer.NextScope(context.Background(), scopes)
	if err != nil {
		t.Fatalf("select new scope: %v", err)
	}
	if next != scopes[2] {
		t.Fatalf("next scope = %#v, want new %#v", next, scopes[2])
	}
}

func TestMassiveCatalogClientRequestsAndFollowsPagination(t *testing.T) {
	var requests []*http.Request
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests = append(requests, request)
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Query().Get("cursor") == "next" {
			_, _ = writer.Write([]byte(`{"results":[]}`))
			return
		}
		_, _ = writer.Write([]byte(`{
			"next_url": "` + serverURL(request) + `/v3/reference/tickers?cursor=next",
			"results": [{
				"ticker": "AAPL",
				"name": "Apple Inc.",
				"type": "CS",
				"primary_exchange": "XNAS",
				"composite_figi": "BBG000B9XRY4",
				"share_class_figi": "BBG001S5N8V8",
				"active": true,
				"last_updated_utc": "2026-08-09T12:00:00Z"
			}]
		}`))
	}))
	defer server.Close()

	client, err := newMassiveCatalogClient("test-key", server.URL, server.Client())
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	scope := newCatalogScope("XNAS", assetTypeCommonStock)
	page, err := client.ListCatalogPage(context.Background(), scope, "")
	if err != nil {
		t.Fatalf("request initial page: %v", err)
	}
	if len(page.Instruments) != 1 || page.Instruments[0].Symbol != "AAPL" {
		t.Fatalf("page instruments = %#v, want AAPL", page.Instruments)
	}
	_, err = client.ListCatalogPage(context.Background(), scope, page.NextURL)
	if err != nil {
		t.Fatalf("request paginated page: %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(requests))
	}
	initialQuery := requests[0].URL.Query()
	if requests[0].URL.Path != "/v3/reference/tickers" ||
		initialQuery.Get("exchange") != "XNAS" ||
		initialQuery.Get("type") != "CS" ||
		initialQuery.Get("limit") != "1000" ||
		initialQuery.Get("apiKey") != "test-key" {
		t.Fatalf("initial query = %q", requests[0].URL.RawQuery)
	}
	if requests[1].URL.Query().Get("cursor") != "next" || requests[1].URL.Query().Get("apiKey") != "test-key" {
		t.Fatalf("pagination query = %q", requests[1].URL.RawQuery)
	}
}

func TestMassiveCatalogClientRejectsForeignPaginationURL(t *testing.T) {
	client, err := newMassiveCatalogClient("test-key", "https://api.massive.com", http.DefaultClient)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	_, err = client.ListCatalogPage(
		context.Background(),
		newCatalogScope("XNAS", assetTypeCommonStock),
		"https://example.com/v3/reference/tickers?cursor=next",
	)
	if err == nil || errors.Is(err, ErrUnavailable) {
		t.Fatalf("foreign pagination URL error = %v, want validation error", err)
	}
}

func serverURL(request *http.Request) string {
	return "http://" + request.Host
}
