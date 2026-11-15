package service

import (
	"context"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// claimKocs is a KOC store with the atomic contact mark: marks is the lastContactedAt of every KOC "in the database".
type claimKocs struct {
	fakeOutreachKocs
	marks    map[domain.ID]*time.Time
	claims   int
	releases int
	claimErr error
}

func (c *claimKocs) ClaimContact(_ context.Context, id domain.ID, notAfter, at time.Time) (bool, *time.Time, error) {
	if c.claimErr != nil {
		return false, nil, c.claimErr
	}
	prev := c.marks[id]
	if prev != nil && prev.After(notAfter) {
		return false, prev, nil
	}
	c.claims++
	mark := at
	c.marks[id] = &mark
	return true, prev, nil
}

func (c *claimKocs) ReleaseContact(_ context.Context, id domain.ID, at time.Time, previous *time.Time) error {
	c.releases++
	if m := c.marks[id]; m != nil && m.Equal(at) {
		c.marks[id] = previous
	}
	return nil
}

func kocWith(name, phone string) *domain.Koc {
	koc := domain.NewKoc()
	koc.ID = domain.NewID()
	koc.DisplayName, koc.Phone = name, phone
	return koc
}
