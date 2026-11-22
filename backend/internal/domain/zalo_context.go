package domain

import "context"

type zaloAccountKey struct{}

// WithZaloAccount says which personal Zalo account the work done with the returned context is for. The Zalo gateway
// sends it to the bridge (one session per account) and the conversation store files and finds Zalo threads under it.
// An empty id leaves the context as it is: the work is then for the default account.
func WithZaloAccount(ctx context.Context, accountID string) context.Context {
	if accountID == "" {
		return ctx
	}
	return context.WithValue(ctx, zaloAccountKey{}, accountID)
}

// ZaloAccountFrom returns the account id set by WithZaloAccount, or "" when there is none.
func ZaloAccountFrom(ctx context.Context) string {
	id, _ := ctx.Value(zaloAccountKey{}).(string)
	return id
}
