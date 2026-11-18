package service

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// memImports keeps jobs in memory like the repository does: it stores copies, so what the worker changes is lost unless
// it is written through the store.
type memImports struct {
	mu          sync.Mutex
	jobs        []*domain.ImportJob
	waits       []int    // the attempt of every SetProgress that carried a waitUntil
	kinds       []string // and its kind
	finishFails int
	setRowErr   error
}

func cloneJob(j *domain.ImportJob) *domain.ImportJob {
	cp := *j
	cp.Rows = append([]domain.ImportRow(nil), j.Rows...)
	return &cp
}

func (m *memImports) Create(_ context.Context, job *domain.ImportJob) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.Status == domain.ImportRunning {
			return ErrImportInProgress
		}
	}
	job.ID, job.CreatedAt, job.Status = domain.NewID(), time.Now(), domain.ImportRunning
	m.jobs = append(m.jobs, cloneJob(job))
	return nil
}
func (m *memImports) find(id domain.ID) *domain.ImportJob {
	for _, j := range m.jobs {
		if j.ID == id {
			return j
		}
	}
	return nil
}
func (m *memImports) Running(context.Context) (*domain.ImportJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.Status == domain.ImportRunning {
			return cloneJob(j), nil
		}
	}
	return nil, nil
}
func (m *memImports) SetRow(_ context.Context, id domain.ID, i int, status, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.setRowErr != nil {
		return m.setRowErr
	}
	m.find(id).Rows[i].Status, m.find(id).Rows[i].Reason = status, reason
	return nil
}
func (m *memImports) SetProgress(_ context.Context, id domain.ID, current string, waitUntil *time.Time, attempt int, kind string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := m.find(id)
	j.Current, j.WaitUntil, j.WaitAttempt, j.WaitKind = current, waitUntil, attempt, kind
	if waitUntil != nil {
		m.waits = append(m.waits, attempt)
		m.kinds = append(m.kinds, kind)
	}
	return nil
}
func (m *memImports) StopRequested(_ context.Context, id domain.ID) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.find(id).StopRequested, nil
}
func (m *memImports) Finish(_ context.Context, id domain.ID, status, reason string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.finishFails > 0 {
		m.finishFails--
		return errors.New("db down")
	}
	j := m.find(id)
	j.Status, j.StopReason, j.FinishedAt = status, reason, &at
	j.Current, j.WaitUntil, j.WaitAttempt, j.WaitKind = "", nil, 0, ""
	return nil
}
func (m *memImports) only() *domain.ImportJob {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneJob(m.jobs[0])
}
func (m *memImports) askToStop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobs[0].StopRequested = true
}

// scriptedImporter answers every row from a script: for a TikTok id, the list of results of its successive tries (the
// last one repeats).
type scriptedImporter struct {
	mu     sync.Mutex
	script map[string][]string // id -> successive statuses; an entry "error" is a failure of our store
	calls  []string
	onCall func(id string)
}

func (s *scriptedImporter) ImportOne(_ context.Context, raw BulkItem, _ string) (*ImportResult, error) {
	s.mu.Lock()
	s.calls = append(s.calls, raw.TiktokID)
	tries := 0
	for _, c := range s.calls {
		if c == raw.TiktokID {
			tries++
		}
	}
	seq := s.script[raw.TiktokID]
	status := ImportAdded
	if len(seq) > 0 {
		status = seq[min(tries, len(seq))-1]
	}
	cb := s.onCall
	s.mu.Unlock()
	if cb != nil {
		cb(raw.TiktokID)
	}
	if status == "error" {
		return nil, errors.New("db down")
	}
	res := &ImportResult{TiktokID: raw.TiktokID, Status: status}
	switch status {
	case ImportDuplicate:
		res.Reason = "Trùng ID kênh"
	case ImportNotListed:
		res.Reason = "Không có trong Nhà sáng tạo"
	}
	return res, nil
}

type importEnv struct {
	r      *ImportRunner
	imp    *scriptedImporter
	store  *memImports
	sleeps []time.Duration
	onNap  func(n int)
	naps   int
}

func newImportEnv(script map[string][]string) *importEnv {
	e := &importEnv{imp: &scriptedImporter{script: script}, store: &memImports{}}
	e.r = NewImportRunner(e.imp, e.store)
	e.r.sleep = func(_ context.Context, d time.Duration) error {
		e.naps++
		e.sleeps = append(e.sleeps, d)
		if e.onNap != nil {
			e.onNap(e.naps)
		}
		return nil
	}
	return e
}

func sheetRows(n int) []domain.ImportRow {
	rows := make([]domain.ImportRow, n)
	for i := range rows {
		rows[i] = domain.ImportRow{Line: i + 2, TiktokID: "k" + strconv.Itoa(i)}
	}
	return rows
}

func (e *importEnv) start(n int) *domain.ImportJob {
	if _, err := e.r.Start(context.Background(), "a@x.vn", "TikToday", sheetRows(n)); err != nil {
		panic(err)
	}
	e.r.Wait()
	return e.store.only()
}

func rowStatuses(j *domain.ImportJob) []string {
	var out []string
	for _, r := range j.Rows {
		out = append(out, r.Status)
	}
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestAJobImportsEveryRowAndEndsDone(t *testing.T) {
	e := newImportEnv(map[string][]string{"k1": {ImportDuplicate}, "k2": {ImportNotListed}, "k3": {ImportCollaborated}})
	job := e.start(5)
	got := rowStatuses(job)
	want := []string{domain.RowAdded, domain.RowDuplicate, domain.RowNotListed, domain.RowCollaborated, domain.RowAdded}
	if job.Status != domain.ImportDone || !sameStrings(got, want) {
		t.Fatalf("%s %v", job.Status, got)
	}
	if job.Rows[1].Reason != "Trùng ID kênh" || job.Rows[3].Reason == "" || job.Current != "" || job.FinishedAt == nil {
		t.Fatalf("reasons are kept, the current row is cleared: %+v", job)
	}
}

func TestWhenTikTokLimitsTheRowIsTriedAgainAfterTheWaitAndTheJobCarriesOn(t *testing.T) {
	e := newImportEnv(map[string][]string{"k1": {ImportRateLimited, ImportAdded}})
	job := e.start(3)
	if job.Status != domain.ImportDone || !sameStrings(rowStatuses(job), []string{"added", "added", "added"}) {
		t.Fatalf("%s %v", job.Status, rowStatuses(job))
	}
	if len(e.store.waits) != 1 || e.store.waits[0] != 1 {
		t.Fatalf("the screen was told about the wait (attempt 1): %v", e.store.waits)
	}
	var total time.Duration
	for _, d := range e.sleeps {
		if d > importStopPoll {
			t.Fatalf("a wait is cut into slices of at most %v: %v", importStopPoll, d)
		}
		total += d
	}
	if total != ImportRateWait {
		t.Fatalf("it waited %v, want %v", total, ImportRateWait)
	}
	if n := countOf(e.imp.calls, "k1"); n != 2 {
		t.Fatalf("the limited row was tried twice, got %d", n)
	}
}

func countOf(list []string, v string) int {
	n := 0
	for _, x := range list {
		if x == v {
			n++
		}
	}
	return n
}

func TestTheJobGivesUpAfterTheMaximumWaitsAndLeavesTheRestQueued(t *testing.T) {
	e := newImportEnv(map[string][]string{"k1": {ImportRateLimited}})
	job := e.start(4)
	if job.Status != domain.ImportStopped || job.StopReason != "rate_limited" {
		t.Fatalf("%s %s", job.Status, job.StopReason)
	}
	if !sameStrings(rowStatuses(job), []string{"added", "queued", "queued", "queued"}) {
		t.Fatalf("the limited row and the ones after it wait to be run again: %v", rowStatuses(job))
	}
	if n := countOf(e.imp.calls, "k1"); n != 1+ImportRateMaxWaits {
		t.Fatalf("the first try and %d retries, got %d", ImportRateMaxWaits, n)
	}
}

func TestTheCountOfWaitsStartsOverWhenARowGoesThrough(t *testing.T) {
	// every row is limited once: 3 rows would need 3 waits in a row if the count never started over
	script := map[string][]string{}
	for i := 0; i < 6; i++ {
		script["k"+strconv.Itoa(i)] = []string{ImportRateLimited, ImportRateLimited, ImportAdded}
	}
	e := newImportEnv(script)
	job := e.start(6)
	if job.Status != domain.ImportDone {
		t.Fatalf("each row waited twice and went through: %s %s", job.Status, job.StopReason)
	}
}

func TestAShopThatIsNotConnectedStopsTheJobAtOnce(t *testing.T) {
	e := newImportEnv(map[string][]string{"k1": {ImportNotConnected}})
	job := e.start(3)
	if job.Status != domain.ImportStopped || job.StopReason != "tiktok_not_connected" || !sameStrings(rowStatuses(job), []string{"added", "queued", "queued"}) || len(e.sleeps) != 0 {
		t.Fatalf("nothing can be checked until the shop is connected, so waiting would be pointless: %s %s %v", job.Status, job.StopReason, rowStatuses(job))
	}
}

func TestATikTokThatDidNotAnswerWellIsWaitedForBrieflyAndTheSameRowIsTriedAgain(t *testing.T) {
	e := newImportEnv(map[string][]string{"k1": {ImportUnavailable, ImportAdded}})
	job := e.start(3)
	if job.Status != domain.ImportDone || !sameStrings(rowStatuses(job), []string{"added", "added", "added"}) {
		t.Fatalf("a timeout must not stop a long job: %s %v", job.Status, rowStatuses(job))
	}
	var total time.Duration
	for _, d := range e.sleeps {
		total += d
	}
	if total != ImportRetryWait || len(e.store.kinds) != 1 || e.store.kinds[0] != "unavailable" {
		t.Fatalf("a short wait, told to the screen as unavailable: waited %v kinds %v", total, e.store.kinds)
	}
	if countOf(e.imp.calls, "k1") != 2 {
		t.Fatalf("the row was tried twice, got %d", countOf(e.imp.calls, "k1"))
	}
}

func TestATikTokThatKeepsFailingStopsTheJobAfterTheMaximumWaits(t *testing.T) {
	e := newImportEnv(map[string][]string{"k1": {ImportUnavailable}})
	job := e.start(3)
	if job.Status != domain.ImportStopped || job.StopReason != "tiktok_unavailable" || !sameStrings(rowStatuses(job), []string{"added", "queued", "queued"}) {
		t.Fatalf("%s %s %v", job.Status, job.StopReason, rowStatuses(job))
	}
	if n := countOf(e.imp.calls, "k1"); n != 1+ImportRateMaxWaits {
		t.Fatalf("the first try and %d retries, got %d", ImportRateMaxWaits, n)
	}
}

func TestARateLimitIsToldToTheScreenAsSuch(t *testing.T) {
	e := newImportEnv(map[string][]string{"k0": {ImportRateLimited, ImportAdded}})
	e.start(1)
	if len(e.store.kinds) != 1 || e.store.kinds[0] != "rate_limited" {
		t.Fatalf("kinds %v", e.store.kinds)
	}
}

func TestARowWithAnAnswerTheWorkerDoesNotKnowFailsInsteadOfLooping(t *testing.T) {
	e := newImportEnv(map[string][]string{"k1": {"something_new"}})
	job := e.start(3)
	if job.Rows[1].Status != domain.RowFailed || job.Status != domain.ImportDone || countOf(e.imp.calls, "k1") != 1 {
		t.Fatalf("%v tries=%d", rowStatuses(job), countOf(e.imp.calls, "k1"))
	}
}

func TestRowsRefusedBeforeTheJobAreKeptAndNeverImported(t *testing.T) {
	e := newImportEnv(nil)
	rows := sheetRows(3)
	rows[1].Status, rows[1].Reason = domain.RowInvalid, "Email không đúng dạng"
	if _, err := e.r.Start(context.Background(), "a", "TikToday", rows); err != nil {
		t.Fatal(err)
	}
	e.r.Wait()
	job := e.store.only()
	if job.Status != domain.ImportDone || !sameStrings(rowStatuses(job), []string{"added", "invalid", "added"}) || job.Rows[1].Reason != "Email không đúng dạng" {
		t.Fatalf("%s %v %q", job.Status, rowStatuses(job), job.Rows[1].Reason)
	}
	if countOf(e.imp.calls, "k1") != 0 {
		t.Fatal("a refused row is not imported")
	}
}

func TestAJobWithNoRowToImportIsRefused(t *testing.T) {
	e := newImportEnv(nil)
	rows := sheetRows(2)
	rows[0].Status, rows[1].Status = domain.RowInvalid, domain.RowInvalid
	if _, err := e.r.Start(context.Background(), "a", "TikToday", rows); err == nil {
		t.Fatal("nothing valid to import: no job")
	}
	if len(e.store.jobs) != 0 {
		t.Fatal("no job was stored")
	}
}

func TestAStopRequestEndsTheJobBeforeTheNextRow(t *testing.T) {
	e := newImportEnv(nil)
	e.imp.onCall = func(id string) {
		if id == "k1" {
			e.store.askToStop() // asked while row 2 is being imported: that row is finished, the next one is not started
		}
	}
	job := e.start(5)
	if job.Status != domain.ImportStopped || job.StopReason != "stopped_by_user" || !sameStrings(rowStatuses(job), []string{"added", "added", "queued", "queued", "queued"}) {
		t.Fatalf("%s %s %v", job.Status, job.StopReason, rowStatuses(job))
	}
}

func TestAStopRequestDuringTheWaitEndsItWithoutAnotherTry(t *testing.T) {
	e := newImportEnv(map[string][]string{"k1": {ImportRateLimited}})
	e.onNap = func(int) { e.store.askToStop() }
	job := e.start(3)
	if job.StopReason != "stopped_by_user" || countOf(e.imp.calls, "k1") != 1 || e.naps != 1 {
		t.Fatalf("%s tries=%d naps=%d", job.StopReason, countOf(e.imp.calls, "k1"), e.naps)
	}
}

func TestOnlyOneJobRunsAndTheRowCountIsChecked(t *testing.T) {
	e := newImportEnv(nil)
	if _, err := e.r.Start(context.Background(), "a", "TikToday", nil); err == nil {
		t.Fatal("no rows")
	}
	if _, err := e.r.Start(context.Background(), "a", "TikToday", sheetRows(MaxImportRows+1)); err == nil {
		t.Fatal("too many rows")
	}
	e.store.jobs = append(e.store.jobs, &domain.ImportJob{ID: domain.NewID(), Status: domain.ImportRunning})
	if _, err := e.r.Start(context.Background(), "a", "TikToday", sheetRows(2)); !errors.Is(err, ErrImportInProgress) {
		t.Fatalf("one at a time: %v", err)
	}
	if len(e.imp.calls) != 0 {
		t.Fatal("nothing was imported")
	}
}

func TestTheCallerOfStartGetsACopy(t *testing.T) {
	e := newImportEnv(nil)
	job, err := e.r.Start(context.Background(), "a", "TikToday", sheetRows(2))
	if err != nil {
		t.Fatal(err)
	}
	e.r.Wait()
	if job.Rows[0].Status != domain.RowQueued {
		t.Fatalf("a snapshot taken before the worker ran, got %s", job.Rows[0].Status)
	}
}

func TestResumeContinuesWithTheQueuedRowsOnly(t *testing.T) {
	e := newImportEnv(nil)
	rows := sheetRows(3)
	rows[0].Status = domain.RowAdded // done before the server stopped
	rows[1].Status, rows[2].Status = domain.RowQueued, domain.RowQueued
	e.store.jobs = append(e.store.jobs, &domain.ImportJob{ID: domain.NewID(), Status: domain.ImportRunning, Rows: rows, Source: "TikToday"})
	e.r.Resume(context.Background())
	e.r.Wait()
	job := e.store.only()
	if job.Status != domain.ImportDone || !sameStrings(rowStatuses(job), []string{"added", "added", "added"}) {
		t.Fatalf("%s %v", job.Status, rowStatuses(job))
	}
	if countOf(e.imp.calls, "k0") != 0 || len(e.imp.calls) != 2 {
		t.Fatalf("the row that was already done is not imported again: %v", e.imp.calls)
	}
}

func TestResumeDoesNothingWhenNoJobIsRunning(t *testing.T) {
	e := newImportEnv(nil)
	e.r.Resume(context.Background())
	e.r.Wait()
	if len(e.imp.calls) != 0 {
		t.Fatal("nothing to resume")
	}
}

func TestAFailureOfOurStoreMarksTheRowAndTheJobCarriesOnUntilItKeepsFailing(t *testing.T) {
	e := newImportEnv(map[string][]string{"k1": {"error"}})
	job := e.start(3)
	if job.Status != domain.ImportDone || job.Rows[1].Status != domain.RowFailed || job.Rows[2].Status != domain.RowAdded {
		t.Fatalf("one failure is that row only: %s %v", job.Status, rowStatuses(job))
	}
	e2 := newImportEnv(map[string][]string{"k0": {"error"}, "k1": {"error"}, "k2": {"error"}})
	job2 := e2.start(5)
	if job2.Status != domain.ImportStopped || job2.StopReason != "internal_error" || job2.Rows[3].Status != domain.RowQueued {
		t.Fatalf("three in a row stop the job: %s %s %v", job2.Status, job2.StopReason, rowStatuses(job2))
	}
}

func TestARowAnswerThatCannotBeWrittenStopsTheJob(t *testing.T) {
	e := newImportEnv(nil)
	e.store.setRowErr = errors.New("db down")
	job := e.start(3)
	if job.Status != domain.ImportStopped || job.StopReason != "internal_error" {
		t.Fatalf("%s %s", job.Status, job.StopReason)
	}
}

func TestTheEndOfAJobIsTriedAgainWhenTheStoreFails(t *testing.T) {
	e := newImportEnv(nil)
	e.store.finishFails = 2
	job := e.start(2)
	if job.Status != domain.ImportDone {
		t.Fatalf("a Finish that fails once or twice must be retried or the job blocks every later import: %s", job.Status)
	}
}
