package market

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	assetTypeCommonStock = "CS"
	assetTypeETF         = "ETF"
)

// CatalogScope is one independently resumable exchange and security-type import.
type CatalogScope struct {
	Key       string
	Exchange  string
	AssetType string
}

// DefaultCatalogScopes returns the initial TradeMind US instrument universe in
// bootstrap order. Each scope is imported page by page.
func DefaultCatalogScopes() []CatalogScope {
	exchanges := []string{"XNAS", "XNYS", "XASE", "ARCX", "BATS", "BATY", "EDGA", "EDGX"}
	assetTypes := []string{assetTypeCommonStock, assetTypeETF}
	scopes := make([]CatalogScope, 0, len(exchanges)*len(assetTypes))
	for _, assetType := range assetTypes {
		for _, exchange := range exchanges {
			scopes = append(scopes, newCatalogScope(exchange, assetType))
		}
	}
	return scopes
}

func newCatalogScope(exchange, assetType string) CatalogScope {
	exchange = strings.ToUpper(strings.TrimSpace(exchange))
	assetType = strings.ToUpper(strings.TrimSpace(assetType))
	return CatalogScope{
		Key:       exchange + ":" + assetType,
		Exchange:  exchange,
		AssetType: assetType,
	}
}

func (s CatalogScope) valid() bool {
	return s.Key != "" && s.Exchange != "" && (s.AssetType == assetTypeCommonStock || s.AssetType == assetTypeETF)
}

// CatalogInstrument is the reference data required to identify a listed
// security locally. Quotes deliberately do not belong in this model.
type CatalogInstrument struct {
	Symbol            string
	Name              string
	AssetType         string
	PrimaryExchange   string
	CompositeFIGI     string
	ShareClassFIGI    string
	Active            bool
	ProviderUpdatedAt *time.Time
	DelistedAt        *time.Time
}

func (i CatalogInstrument) providerKey(scope CatalogScope) string {
	if figi := strings.TrimSpace(i.ShareClassFIGI); figi != "" {
		return "share-class-figi:" + figi
	}
	return "listing:" + scope.Key + ":" + i.Symbol
}

// CatalogPage is one page from a provider cursor.
type CatalogPage struct {
	Instruments []CatalogInstrument
	NextURL     string
}

// CatalogSource retrieves reference-data pages from a market-data provider.
type CatalogSource interface {
	ListCatalogPage(ctx context.Context, scope CatalogScope, nextURL string) (CatalogPage, error)
}

// CatalogRun identifies a resumable scope import.
type CatalogRun struct {
	ID      string
	NextURL string
	Status  string
}

// CatalogScopeState is the persisted state used to choose the next scope.
type CatalogScopeState struct {
	Exists bool
	Run    CatalogRun
}

// CatalogStore persists reference data and sync progress.
type CatalogStore interface {
	ScopeState(ctx context.Context, scope CatalogScope) (CatalogScopeState, error)
	StartOrResumeCatalogRun(ctx context.Context, scope CatalogScope) (CatalogRun, error)
	ApplyCatalogPage(ctx context.Context, scope CatalogScope, run CatalogRun, instruments []CatalogInstrument, nextURL string) (CatalogRun, error)
}

// CatalogSynchronizer imports at most one provider page per SyncPage call.
// Persisting each page makes periodic jobs resumable and bounds provider usage.
type CatalogSynchronizer struct {
	source CatalogSource
	store  CatalogStore
}

func NewCatalogSynchronizer(source CatalogSource, store CatalogStore) (*CatalogSynchronizer, error) {
	if source == nil {
		return nil, fmt.Errorf("catalog source is required")
	}
	if store == nil {
		return nil, fmt.Errorf("catalog store is required")
	}
	return &CatalogSynchronizer{source: source, store: store}, nil
}

type CatalogSyncResult struct {
	Scope        CatalogScope
	RunID        string
	RecordCount  int
	CompletedRun bool
}

func (s *CatalogSynchronizer) SyncPage(ctx context.Context, scope CatalogScope) (CatalogSyncResult, error) {
	if !scope.valid() {
		return CatalogSyncResult{}, fmt.Errorf("invalid catalog scope %q", scope.Key)
	}

	run, err := s.store.StartOrResumeCatalogRun(ctx, scope)
	if err != nil {
		return CatalogSyncResult{}, fmt.Errorf("start catalog run: %w", err)
	}
	page, err := s.source.ListCatalogPage(ctx, scope, run.NextURL)
	if err != nil {
		return CatalogSyncResult{}, fmt.Errorf("fetch catalog page: %w", err)
	}
	instruments, err := normalizeCatalogInstruments(scope, page.Instruments)
	if err != nil {
		return CatalogSyncResult{}, err
	}
	run, err = s.store.ApplyCatalogPage(ctx, scope, run, instruments, page.NextURL)
	if err != nil {
		return CatalogSyncResult{}, fmt.Errorf("persist catalog page: %w", err)
	}
	return CatalogSyncResult{
		Scope:        scope,
		RunID:        run.ID,
		RecordCount:  len(instruments),
		CompletedRun: run.Status == "completed",
	}, nil
}

// NextScope favors unfinished imports, then starts the next never-imported
// scope. Once every scope has completed, it begins a new refresh at the first.
func (s *CatalogSynchronizer) NextScope(ctx context.Context, scopes []CatalogScope) (CatalogScope, error) {
	if len(scopes) == 0 {
		return CatalogScope{}, fmt.Errorf("at least one catalog scope is required")
	}

	for _, scope := range scopes {
		if !scope.valid() {
			return CatalogScope{}, fmt.Errorf("invalid catalog scope %q", scope.Key)
		}
		state, err := s.store.ScopeState(ctx, scope)
		if err != nil {
			return CatalogScope{}, fmt.Errorf("load catalog scope %q: %w", scope.Key, err)
		}
		if state.Exists && state.Run.Status == "running" {
			return scope, nil
		}
	}
	for _, scope := range scopes {
		state, err := s.store.ScopeState(ctx, scope)
		if err != nil {
			return CatalogScope{}, fmt.Errorf("load catalog scope %q: %w", scope.Key, err)
		}
		if !state.Exists {
			return scope, nil
		}
	}
	return scopes[0], nil
}

func normalizeCatalogInstruments(scope CatalogScope, instruments []CatalogInstrument) ([]CatalogInstrument, error) {
	normalized := make([]CatalogInstrument, 0, len(instruments))
	for _, instrument := range instruments {
		symbol, err := NormalizeSymbol(instrument.Symbol)
		if err != nil {
			return nil, fmt.Errorf("normalize provider symbol %q: %w", instrument.Symbol, err)
		}
		instrument.Symbol = symbol
		instrument.Name = strings.TrimSpace(instrument.Name)
		instrument.AssetType = strings.ToUpper(strings.TrimSpace(instrument.AssetType))
		instrument.PrimaryExchange = strings.ToUpper(strings.TrimSpace(instrument.PrimaryExchange))
		instrument.CompositeFIGI = strings.TrimSpace(instrument.CompositeFIGI)
		instrument.ShareClassFIGI = strings.TrimSpace(instrument.ShareClassFIGI)
		if instrument.Name == "" {
			return nil, fmt.Errorf("provider returned an empty name for %s", instrument.Symbol)
		}
		if instrument.PrimaryExchange != scope.Exchange || instrument.AssetType != scope.AssetType {
			return nil, fmt.Errorf(
				"provider returned %s/%s for scope %s",
				instrument.PrimaryExchange,
				instrument.AssetType,
				scope.Key,
			)
		}
		normalized = append(normalized, instrument)
	}
	return normalized, nil
}
