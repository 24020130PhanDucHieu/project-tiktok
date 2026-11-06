package domain

// FaultKind says what went wrong in terms of the business, not of HTTP: the transport layer decides which status code
// each kind becomes. A use case returns a Fault when the caller can fix the request or has to be told no.
type FaultKind int

// The kinds of fault.
const (
	FaultInvalid       FaultKind = iota + 1 // the request is wrong (bad value, rule broken)
	FaultUnauthorized                       // who are you? (bad credentials)
	FaultForbidden                          // known, but not allowed
	FaultNotFound                           // the thing asked for does not exist
	FaultConflict                           // clashes with the current state (duplicate, last admin...)
	FaultUnprocessable                      // well formed but cannot be done (creator not in the marketplace)
	FaultLocked                             // temporarily locked (too many failed sign-ins)
	FaultRateLimited                        // slow down
	FaultUnavailable                        // an outside service we depend on said no or did not answer
)

// Fault is an error a client is meant to see, with the message to show.
type Fault struct {
	Kind    FaultKind
	Message string
}

func (f *Fault) Error() string { return f.Message }

// Invalid is a FaultInvalid.
func Invalid(message string) *Fault { return &Fault{Kind: FaultInvalid, Message: message} }

// Unauthorized is a FaultUnauthorized.
func Unauthorized(message string) *Fault { return &Fault{Kind: FaultUnauthorized, Message: message} }

// Forbidden is a FaultForbidden.
func Forbidden(message string) *Fault { return &Fault{Kind: FaultForbidden, Message: message} }

// NotFound is a FaultNotFound.
func NotFound(message string) *Fault { return &Fault{Kind: FaultNotFound, Message: message} }

// Conflict is a FaultConflict.
func Conflict(message string) *Fault { return &Fault{Kind: FaultConflict, Message: message} }

// Unprocessable is a FaultUnprocessable.
func Unprocessable(message string) *Fault { return &Fault{Kind: FaultUnprocessable, Message: message} }

// Locked is a FaultLocked.
func Locked(message string) *Fault { return &Fault{Kind: FaultLocked, Message: message} }

// RateLimited is a FaultRateLimited.
func RateLimited(message string) *Fault { return &Fault{Kind: FaultRateLimited, Message: message} }

// Unavailable is a FaultUnavailable.
func Unavailable(message string) *Fault { return &Fault{Kind: FaultUnavailable, Message: message} }
