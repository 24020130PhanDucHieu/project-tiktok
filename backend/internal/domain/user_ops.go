package domain

// UserQuery selects users for the admin list. Empty fields do not filter.
type UserQuery struct {
	Search, Role, Status string
	Skip, Limit          int64
}

// UserChange is a partial account update: nil fields stay as they are. RevokeTokens invalidates every session the
// account has (role and status changes do).
type UserChange struct {
	Name, Role, Status *string
	RevokeTokens       bool
}

// UserCounts are the account counters of the system page.
type UserCounts struct {
	Total, Admins, Suspended, ActiveLast7d int64
}
