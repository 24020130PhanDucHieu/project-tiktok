package service

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// Limits and pace of a background import. The pace of the calls to TikTok itself is the pacer's (ttshop_pacer.go).
const (
	MaxImportRows      = 200
	ImportRateWait     = 50 * time.Second // the server pauses its TikTok calls for 45 s when TikTok limits them, plus a margin
	ImportRetryWait    = 15 * time.Second // when TikTok did not answer well (a timeout...): a short pause, then the same row again
	ImportRateMaxWaits = 3                // waits for the same row, of either kind, before the job gives up
	importStopPoll     = 5 * time.Second  // while waiting, how often the stop request is looked at
)

// ErrImportInProgress is what Start answers while another import job is running.
var ErrImportInProgress = domain.Conflict("Đang có một đợt nhập KOC chạy. Hãy chờ đợt đó xong hoặc dừng nó.")

// ImportStore is where import jobs are kept (*repository.ImportJobRepository).
type ImportStore interface {
	Create(ctx context.Context, job *domain.ImportJob) error
	Running(ctx context.Context) (*domain.ImportJob, error)
	SetRow(ctx context.Context, id domain.ID, index int, status, reason string) error
	SetRowKoc(ctx context.Context, id domain.ID, index int, kocID string) error
	SetProgress(ctx context.Context, id domain.ID, current string, waitUntil *time.Time, attempt int, kind string) error
	StopRequested(ctx context.Context, id domain.ID) (bool, error)
	Finish(ctx context.Context, id domain.ID, status, reason string, at time.Time) error
}

// RowImporter imports one row (*KocService).
type RowImporter interface {
	ImportOne(ctx context.Context, raw BulkItem, defaultSource string) (*ImportResult, error)
}

// ImportRunner imports the rows of a job in the background, one row after the other. It writes every answer into the job
// and the screen only reads the job, so closing or reloading the page changes nothing.
type ImportRunner struct {
	importer RowImporter
	store    ImportStore

	// tests replace these
	sleep func(ctx context.Context, d time.Duration) error
	now   func() time.Time

	wg sync.WaitGroup
}

// NewImportRunner builds the runner.
func NewImportRunner(importer RowImporter, store ImportStore) *ImportRunner {
	return &ImportRunner{importer: importer, store: store, sleep: realSleep, now: time.Now}
}

// Wait blocks until every background job that was started has ended (for tests).
func (r *ImportRunner) Wait() { r.wg.Wait() }

func cloneImportJob(job *domain.ImportJob) *domain.ImportJob {
	cp := *job
	cp.Rows = append([]domain.ImportRow(nil), job.Rows...)
	return &cp
}

// ImportOptions are the choices of one job: the workflow (and block) that starts it, so that the workflow finds the job
// again after a restart.
type ImportOptions struct{ WorkflowID, NodeID string }

// Start stores a job and begins it in the background. The rows are queued, except the ones the caller already marked
// invalid (their data was refused: they are kept in the job with their reason and never imported). It refuses a list with
// no row to import or too long, and answers ErrImportInProgress while another job is running. The caller gets a copy: the
// worker owns the job.
func (r *ImportRunner) Start(ctx context.Context, by, source string, rows []domain.ImportRow, opts ...ImportOptions) (*domain.ImportJob, error) {
	if len(rows) == 0 || len(rows) > MaxImportRows {
		return nil, domain.Invalid("Mỗi lần nhập từ 1 đến 200 dòng.")
	}
	queued := 0
	for i := range rows {
		if rows[i].Status == domain.RowInvalid {
			continue
		}
		rows[i].Status, rows[i].Reason = domain.RowQueued, ""
		queued++
	}
	if queued == 0 {
		return nil, domain.Invalid("Không có dòng nào hợp lệ để nhập.")
	}
	job := &domain.ImportJob{Source: source, Rows: rows, By: by}
	if len(opts) > 0 {
		job.WorkflowID, job.NodeID = opts[0].WorkflowID, opts[0].NodeID
	}
	if err := r.store.Create(ctx, job); err != nil {
		return nil, err
	}
	snapshot := cloneImportJob(job)
	r.spawn(job)
	return snapshot, nil
}

// Resume continues the job that was running when the server stopped. Nothing special is needed for the row that was being
// checked at that moment: it is still queued, and importing a row again is safe (a KOC already added is a duplicate).
func (r *ImportRunner) Resume(ctx context.Context) {
	job, err := r.store.Running(ctx)
	if err != nil {
		log.Printf("import: cannot look for a job to resume: %v", err)
		return
	}
	if job != nil {
		r.spawn(job)
	}
}

func (r *ImportRunner) spawn(job *domain.ImportJob) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.work(job)
	}()
}

func toBulkItem(row domain.ImportRow) BulkItem {
	return BulkItem{
		TiktokID: row.TiktokID, DisplayName: row.DisplayName, Phone: row.Phone, Category: row.Category, Email: row.Email,
		Recipient: row.Recipient, ShippingAddress: row.ShippingAddress, Followers: row.Followers, Gmv: row.Gmv, CastFee: row.CastFee,
	}
}

// waitFor waits for d, in short slices so that a stop request ends the wait. It reports false when the job was asked to stop.
func (r *ImportRunner) waitFor(ctx context.Context, id domain.ID, d time.Duration) bool {
	for d > 0 {
		slice := d
		if slice > importStopPoll {
			slice = importStopPoll
		}
		if err := r.sleep(ctx, slice); err != nil {
			return false
		}
		d -= slice
		if stop, err := r.store.StopRequested(ctx, id); err == nil && stop {
			return false
		}
	}
	return true
}

// work goes through the queued rows of the job.
func (r *ImportRunner) work(job *domain.ImportJob) {
	// nobody waits for an answer here: the calls to TikTok give way to the ones a person is waiting for
	ctx := WithBackgroundPriority(context.Background())
	finished := false
	finish := func(status, reason string) {
		finished = true
		// a job that is never marked as ended would block every later import, so the end is tried again before giving up
		var err error
		for try := 1; try <= 5; try++ {
			if err = r.store.Finish(ctx, job.ID, status, reason, r.now()); err == nil {
				return
			}
			_ = r.sleep(ctx, time.Duration(try)*time.Second)
		}
		log.Printf("import %s: cannot record the end: %v", job.ID, err)
	}
	defer func() {
		if p := recover(); p != nil {
			log.Printf("import %s: panic: %v", job.ID, p)
			if !finished {
				finish(domain.ImportStopped, "internal_error")
			}
		}
	}()

	failures := 0 // rows in a row that our own store failed for
	for i := range job.Rows {
		row := job.Rows[i]
		if row.Status != domain.RowQueued {
			continue
		}
		if stop, err := r.store.StopRequested(ctx, job.ID); err == nil && stop {
			finish(domain.ImportStopped, "stopped_by_user")
			return
		}
		_ = r.store.SetProgress(ctx, job.ID, row.TiktokID, nil, 0, "")

		status, reason, kocID := "", "", ""
		waits := 0
		for status == "" {
			res, err := r.importer.ImportOne(ctx, toBulkItem(row), job.Source)
			if err != nil {
				log.Printf("import %s: row %d: %v", job.ID, row.Line, err)
				status, reason = domain.RowFailed, "Lỗi hệ thống, hãy nhập lại dòng này"
				failures++
				break
			}
			failures = 0
			wait, kind := time.Duration(0), ""
			switch res.Status {
			case ImportAdded:
				status = domain.RowAdded
				if res.Koc != nil {
					kocID = res.Koc.ID.Hex()
				}
			case ImportDuplicate:
				status, reason = domain.RowDuplicate, res.Reason
			case ImportCollaborated:
				status, reason = domain.RowCollaborated, "KOC đã từng hợp tác với shop nên bị chặn"
			case ImportNotListed:
				status, reason = domain.RowNotListed, res.Reason
			case ImportEmpty:
				status, reason = domain.RowFailed, "Thiếu TikTok ID"
			case ImportNotConnected:
				finish(domain.ImportStopped, "tiktok_not_connected") // nothing can be checked until the shop is connected: the row stays queued
				return
			case ImportRateLimited:
				wait, kind = ImportRateWait, "rate_limited"
			case ImportUnavailable:
				wait, kind = ImportRetryWait, "unavailable" // a timeout or a bad answer is often gone a few seconds later
			}
			if kind == "" {
				if status == "" { // an answer this code does not know: never loop on it
					status, reason = domain.RowFailed, "Kết quả không xác định"
				}
				continue
			}
			if waits >= ImportRateMaxWaits {
				reasonCode := "rate_limited"
				if kind == "unavailable" {
					reasonCode = "tiktok_unavailable"
				}
				finish(domain.ImportStopped, reasonCode) // the row stays queued
				return
			}
			waits++
			until := r.now().Add(wait)
			_ = r.store.SetProgress(ctx, job.ID, row.TiktokID, &until, waits, kind)
			if !r.waitFor(ctx, job.ID, wait) {
				finish(domain.ImportStopped, "stopped_by_user")
				return
			}
			_ = r.store.SetProgress(ctx, job.ID, row.TiktokID, nil, 0, "")
		}
		if kocID != "" {
			// before the status: a row that is still queued after a crash is imported again (and then a duplicate), but the
			// KOC it created must stay known, for a workflow that goes on with the KOCs of this job
			if err := r.store.SetRowKoc(ctx, job.ID, i, kocID); err != nil {
				log.Printf("import %s: cannot record the KOC of row %d: %v", job.ID, row.Line, err)
			}
		}
		if err := r.store.SetRow(ctx, job.ID, i, status, reason); err != nil {
			log.Printf("import %s: cannot record row %d: %v", job.ID, row.Line, err)
			finish(domain.ImportStopped, "internal_error") // an answer that cannot be written is lost: stop here
			return
		}
		job.Rows[i].Status, job.Rows[i].Reason = status, reason
		if failures >= 3 {
			finish(domain.ImportStopped, "internal_error")
			return
		}
	}
	_ = r.store.SetProgress(ctx, job.ID, "", nil, 0, "")
	finish(domain.ImportDone, "")
}
