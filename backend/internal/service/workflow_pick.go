package service

import (
	"context"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// KocPicker takes KOCs that are already in the directory for a pick block (*KocPickerService).
type KocPicker interface {
	Pick(ctx context.Context, stages []string, folder string, limit int) ([]domain.TokenKoc, error)
}

// KocSearcher is the directory as the picker reads it: a page of the active KOCs of a stage and a folder, newest first
// (*repository.KocRepository).
type KocSearcher interface {
	Search(ctx context.Context, q domain.KocQuery, skip, limit int64) ([]domain.Koc, int64, error)
}

// KocPickerService answers a pick block from the KOC directory.
type KocPickerService struct{ dir KocSearcher }

// NewKocPicker builds the picker.
func NewKocPicker(dir KocSearcher) *KocPickerService { return &KocPickerService{dir: dir} }

const pickPage = 100

// Pick returns at most limit KOCs of the given stages (all stages when none is given) and folder (all folders when empty), in
// the order of the directory, one stage after the other. A KOC that must not be contacted (do not contact, or blocked
// because it already collaborated) is left out, and so is a KOC found twice. A directory that cannot be read is an error:
// an empty list would look like "nobody matches".
func (p *KocPickerService) Pick(ctx context.Context, stages []string, folder string, limit int) ([]domain.TokenKoc, error) {
	queries := []domain.KocQuery{{Category: folder}}
	if len(stages) > 0 {
		queries = queries[:0]
		for _, s := range stages {
			queries = append(queries, domain.KocQuery{Stage: s, Category: folder})
		}
	}
	var out []domain.TokenKoc
	seen := map[domain.ID]bool{}
	for _, q := range queries {
		for skip := int64(0); len(out) < limit; skip += pickPage {
			page, total, err := p.dir.Search(ctx, q, skip, pickPage)
			if err != nil {
				return nil, err
			}
			for _, k := range page {
				if k.Dnc || k.DaHopTac || seen[k.ID] {
					continue
				}
				seen[k.ID] = true
				out = append(out, domain.TokenKoc{KocID: k.ID.Hex()})
				if len(out) == limit {
					break
				}
			}
			if skip+pickPage >= total || len(page) == 0 {
				break
			}
		}
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// advancePick passes on the KOCs the workflow took when it started: the list is fixed at that moment, so a restart or a
// directory that changed in the meantime does not change who the workflow is about.
func (r *WorkflowRunner) advancePick(ctx context.Context, wf *domain.Workflow, n domain.WorkflowNode, t domain.WorkflowToken) advanceResult {
	r.setState(ctx, wf, domain.NodeState{NodeID: n.ID, Status: domain.NodeDone, Count: len(t.Kocs)})
	return advanceResult{tokens: r.leave(ctx, wf, n.ID, domain.PortOut, t.Kocs)}
}
