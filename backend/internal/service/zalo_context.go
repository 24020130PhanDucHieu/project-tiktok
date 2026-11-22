package service

import (
	"context"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// WithZaloAccount says which personal Zalo account the calls made with the returned context are for: the Zalo
// gateway sends the id to the bridge, which keeps one session per account. Without it the calls are for the default
// account, which is also what everything did before there were several.
func WithZaloAccount(ctx context.Context, accountID string) context.Context {
	return domain.WithZaloAccount(ctx, accountID)
}

// ZaloAccountFrom returns the account id set by WithZaloAccount, or "" for the default account.
func ZaloAccountFrom(ctx context.Context) string { return domain.ZaloAccountFrom(ctx) }
