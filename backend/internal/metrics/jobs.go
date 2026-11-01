package metrics

import (
	"sync"
	"time"
)

// JobRun describes the last execution of a background job.
type JobRun struct {
	Name       string    `json:"name"`
	Interval   string    `json:"interval"`
	LastStart  time.Time `json:"lastStart"`
	DurationMs int64     `json:"durationMs"`
	OK         bool      `json:"ok"`
	Error      string    `json:"error,omitempty"`
	Summary    any       `json:"summary,omitempty"`
	Runs       int64     `json:"runs"`
	Failures   int64     `json:"failures"`
}

// Jobs is a registry of background job executions shown on the admin monitoring page.
type Jobs struct {
	mu   sync.Mutex
	runs map[string]*JobRun
}

// NewJobs creates an empty registry.
func NewJobs() *Jobs { return &Jobs{runs: map[string]*JobRun{}} }

// Register declares a job so it is listed before its first run.
func (j *Jobs) Register(name, interval string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if _, ok := j.runs[name]; !ok {
		j.runs[name] = &JobRun{Name: name, Interval: interval, OK: true}
	}
}

// Record stores the outcome of one run.
func (j *Jobs) Record(name string, start time.Time, err error, summary any) {
	if j == nil {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	r, ok := j.runs[name]
	if !ok {
		r = &JobRun{Name: name}
		j.runs[name] = r
	}
	r.LastStart, r.DurationMs, r.Runs = start, time.Since(start).Milliseconds(), r.Runs+1
	r.OK, r.Error, r.Summary = err == nil, "", summary
	if err != nil {
		r.Error = err.Error()
		r.Failures++
	}
}

// List returns a copy of every job.
func (j *Jobs) List() []JobRun {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]JobRun, 0, len(j.runs))
	for _, r := range j.runs {
		out = append(out, *r)
	}
	return out
}
