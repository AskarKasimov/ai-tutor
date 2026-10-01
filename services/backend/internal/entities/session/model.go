package session

type Expiry struct{ AccessExpiresAt, RefreshExpiresAt int64 }
type Tokens struct {
	Access, Refresh string
	Expiry          Expiry
}
type Session struct {
	ID, UserID string
	ExpiresAt  int64
	RevokedAt  *int64
}
type RefreshState struct {
	Session Session
	UsedAt  *int64
}
type AccessState struct {
	Session   Session
	ExpiresAt int64
}
