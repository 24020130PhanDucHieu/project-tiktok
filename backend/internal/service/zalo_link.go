package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// A KOC answers on Zalo from an account the app only knows by number: we sent the invitation to a phone number, but
// the reply arrives with a Zalo user id. When the thread was not bound to that id at send time (Zalo was offline, the
// look-up failed, the number was added later) the reply used to open a second, anonymous thread asking for the phone
// again. LinkZaloContact finds the KOC thread whose phone number belongs to that Zalo account and binds it.
const (
	zaloLinkCandidates = 5 // Zalo limits look-ups by phone: never try more than a handful per message
	zaloLinkRetryAfter = 5 * time.Minute
)

var zaloLinkTried = struct {
	sync.Mutex
	at map[string]time.Time
}{at: map[string]time.Time{}}

// matchZaloOwner returns the first thread whose phone resolves to uid. resolve returns "" when the phone has no account.
func matchZaloOwner(threads []domain.Conversation, uid string, resolve func(phone string) string) *domain.Conversation {
	for i := range threads {
		if got := resolve(threads[i].ContactPhone); got != "" && got == uid {
			return &threads[i]
		}
	}
	return nil
}

// LinkZaloContact binds the KOC thread that owns this Zalo user id (see above). It does nothing when the id is already
// bound to a KOC thread, and never fails the inbound message: the caller ignores the error.
func (s *OutreachService) LinkZaloContact(ctx context.Context, uid string) error {
	if uid == "" || s.zalo == nil {
		return nil
	}
	existing, err := s.convs.FindByExternal(ctx, uid)
	if err != nil {
		return err
	}
	if existing != nil && strings.HasPrefix(existing.ContactHandle, "@") {
		return nil // already a KOC's thread
	}
	zaloLinkTried.Lock()
	if at, ok := zaloLinkTried.at[uid]; ok && time.Since(at) < zaloLinkRetryAfter {
		zaloLinkTried.Unlock()
		return nil
	}
	zaloLinkTried.at[uid] = time.Now()
	if len(zaloLinkTried.at) > 500 {
		zaloLinkTried.at = map[string]time.Time{uid: time.Now()}
	}
	zaloLinkTried.Unlock()

	threads, err := s.convs.FindUnlinkedKocThreads(ctx, zaloLinkCandidates)
	if err != nil || len(threads) == 0 {
		return err
	}
	owner := matchZaloOwner(threads, uid, func(phone string) string {
		u, err := s.zalo.Personal().ResolveByPhone(ctx, phone)
		if err != nil || u == nil {
			return ""
		}
		return u.UserID
	})
	if owner == nil {
		return nil
	}
	if existing != nil { // the anonymous thread made by an earlier reply gives the id up
		if err := s.convs.ReleaseZalo(ctx, existing.ID); err != nil {
			return err
		}
	}
	owner.Channel = domain.ChannelZalo
	owner.ExternalContactID = uid
	return s.convs.SetZaloContact(ctx, owner.ID, domain.ZaloContact{Channel: domain.ChannelZalo, ExternalContactID: uid})
}
