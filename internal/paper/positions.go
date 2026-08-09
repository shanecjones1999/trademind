package paper

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

var ErrInvalidExecution = errors.New("invalid paper execution")

type Position struct {
	Symbol           string `json:"symbol"`
	Quantity         int64  `json:"quantity"`
	CostBasisCents   Money  `json:"cost_basis_cents"`
	RealizedPnLCents Money  `json:"realized_pnl_cents"`
}

type positionLot struct {
	quantity     int64
	costPerShare Money
}

// PositionLedger maintains FIFO lots so selling shares realizes gains and
// losses without losing cent-level cost-basis information.
type PositionLedger struct {
	accountID   string
	lots        map[string][]positionLot
	realizedPnL map[string]Money
}

func NewPositionLedger(accountID string) (*PositionLedger, error) {
	if accountID == "" {
		return nil, fmt.Errorf("%w: account ID is required", ErrInvalidExecution)
	}
	return &PositionLedger{
		accountID:   accountID,
		lots:        make(map[string][]positionLot),
		realizedPnL: make(map[string]Money),
	}, nil
}

func (l *PositionLedger) ApplyFill(fill OrderFill) (Position, error) {
	if l == nil {
		return Position{}, fmt.Errorf("%w: position ledger is required", ErrInvalidExecution)
	}
	if err := validateOrderFill(fill); err != nil {
		return Position{}, err
	}
	if fill.Order.AccountID != l.accountID {
		return Position{}, fmt.Errorf("%w: fill belongs to a different account", ErrInvalidExecution)
	}

	symbol := fill.Order.Symbol
	if fill.Order.Side == OrderSideBuy {
		l.lots[symbol] = append(l.lots[symbol], positionLot{
			quantity:     fill.Execution.Quantity,
			costPerShare: fill.Execution.PriceCents,
		})
		return l.position(symbol)
	}

	available, err := l.quantity(symbol)
	if err != nil {
		return Position{}, err
	}
	if fill.Execution.Quantity > available {
		return Position{}, ErrInsufficientPosition
	}

	cost, remainingLots, err := consumeLots(l.lots[symbol], fill.Execution.Quantity)
	if err != nil {
		return Position{}, err
	}
	proceeds, err := multipliedMoney(fill.Execution.PriceCents, fill.Execution.Quantity)
	if err != nil {
		return Position{}, err
	}
	realized := proceeds - cost
	currentRealized := l.realizedPnL[symbol]
	if (realized > 0 && currentRealized > Money(math.MaxInt64)-realized) ||
		(realized < 0 && currentRealized < Money(math.MinInt64)-realized) {
		return Position{}, fmt.Errorf("%w: realized profit and loss exceeds supported range", ErrInvalidExecution)
	}

	if len(remainingLots) == 0 {
		delete(l.lots, symbol)
	} else {
		l.lots[symbol] = remainingLots
	}
	l.realizedPnL[symbol] = currentRealized + realized
	return l.position(symbol)
}

func (l *PositionLedger) Positions() ([]Position, error) {
	if l == nil {
		return nil, fmt.Errorf("%w: position ledger is required", ErrInvalidExecution)
	}
	symbols := make([]string, 0, len(l.lots))
	for symbol := range l.lots {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)

	positions := make([]Position, 0, len(symbols))
	for _, symbol := range symbols {
		position, err := l.position(symbol)
		if err != nil {
			return nil, err
		}
		positions = append(positions, position)
	}
	return positions, nil
}

func (l *PositionLedger) position(symbol string) (Position, error) {
	quantity, err := l.quantity(symbol)
	if err != nil {
		return Position{}, err
	}
	var costBasis Money
	for _, lot := range l.lots[symbol] {
		cost, err := multipliedMoney(lot.costPerShare, lot.quantity)
		if err != nil {
			return Position{}, err
		}
		if costBasis > Money(math.MaxInt64)-cost {
			return Position{}, fmt.Errorf("%w: cost basis exceeds supported range", ErrInvalidExecution)
		}
		costBasis += cost
	}
	return Position{
		Symbol:           symbol,
		Quantity:         quantity,
		CostBasisCents:   costBasis,
		RealizedPnLCents: l.realizedPnL[symbol],
	}, nil
}

func (l *PositionLedger) quantity(symbol string) (int64, error) {
	var quantity int64
	for _, lot := range l.lots[symbol] {
		if lot.quantity <= 0 || quantity > math.MaxInt64-lot.quantity {
			return 0, fmt.Errorf("%w: position quantity exceeds supported range", ErrInvalidExecution)
		}
		quantity += lot.quantity
	}
	return quantity, nil
}

func validateOrderFill(fill OrderFill) error {
	if fill.Order.Status != OrderStatusFilled ||
		fill.Order.ID == "" ||
		fill.Execution.OrderID != fill.Order.ID ||
		fill.Execution.Symbol != fill.Order.Symbol ||
		fill.Execution.Quantity != fill.Order.Quantity ||
		fill.Execution.Quantity <= 0 ||
		fill.Execution.PriceCents <= 0 ||
		fill.Execution.OccurredAt.IsZero() {
		return fmt.Errorf("%w: fill does not match its completed order", ErrInvalidExecution)
	}
	return nil
}

func consumeLots(lots []positionLot, quantity int64) (Money, []positionLot, error) {
	remainingQuantity := quantity
	var cost Money
	remainingLots := make([]positionLot, 0, len(lots))
	for _, lot := range lots {
		consumed := min(lot.quantity, remainingQuantity)
		if consumed > 0 {
			consumedCost, err := multipliedMoney(lot.costPerShare, consumed)
			if err != nil {
				return 0, nil, err
			}
			if cost > Money(math.MaxInt64)-consumedCost {
				return 0, nil, fmt.Errorf("%w: cost basis exceeds supported range", ErrInvalidExecution)
			}
			cost += consumedCost
			lot.quantity -= consumed
			remainingQuantity -= consumed
		}
		if lot.quantity > 0 {
			remainingLots = append(remainingLots, lot)
		}
	}
	if remainingQuantity != 0 {
		return 0, nil, ErrInsufficientPosition
	}
	return cost, remainingLots, nil
}
