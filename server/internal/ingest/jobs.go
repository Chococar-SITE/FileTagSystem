package ingest

import (
	"context"
	"sync"
	"time"
)

// Job state values.
const (
	StateScanning  = "scanning"
	StateDone      = "done"
	StateError     = "error"
	StateCancelled = "cancelled"
)

// JobStatus is a snapshot of a scan job for the status endpoint (§8.2).
type JobStatus struct {
	State     string    `json:"state"`
	Processed int64     `json:"processed"`
	Phase     string    `json:"phase"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at"`
}

type job struct {
	status JobStatus
	cancel context.CancelFunc
}

// JobManager tracks at most one running scan per storage.
type JobManager struct {
	mu   sync.Mutex
	jobs map[int64]*job
}

// NewJobManager builds a JobManager.
func NewJobManager() *JobManager { return &JobManager{jobs: make(map[int64]*job)} }

// ErrBusy indicates a scan is already running for the storage.
type ErrBusy struct{}

func (ErrBusy) Error() string { return "ingest: a scan is already running for this storage" }

// Start launches run in the background for storageID. run receives a cancelable
// context and a progress callback. It is an error to start while one is running.
func (m *JobManager) Start(storageID int64, run func(ctx context.Context, progress func(int64)) (int64, error)) error {
	m.mu.Lock()
	if j, ok := m.jobs[storageID]; ok && j.status.State == StateScanning {
		m.mu.Unlock()
		return ErrBusy{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	j := &job{
		status: JobStatus{State: StateScanning, Phase: "scanning", StartedAt: time.Now().UTC()},
		cancel: cancel,
	}
	m.jobs[storageID] = j
	m.mu.Unlock()

	go func() {
		processed, err := run(ctx, func(n int64) { m.setProcessed(storageID, n) })
		m.finish(storageID, processed, err, ctx)
	}()
	return nil
}

func (m *JobManager) setProcessed(storageID, n int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if j, ok := m.jobs[storageID]; ok {
		j.status.Processed = n
	}
}

func (m *JobManager) finish(storageID, processed int64, err error, ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[storageID]
	if !ok {
		return
	}
	j.status.Processed = processed
	switch {
	case ctx.Err() != nil:
		j.status.State = StateCancelled
		j.status.Phase = "cancelled"
	case err != nil:
		j.status.State = StateError
		j.status.Phase = "error"
		j.status.Error = err.Error()
	default:
		j.status.State = StateDone
		j.status.Phase = "done"
	}
}

// Status returns the current job status for a storage, if any.
func (m *JobManager) Status(storageID int64) (JobStatus, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[storageID]
	if !ok {
		return JobStatus{}, false
	}
	return j.status, true
}

// Cancel requests cancellation of a running scan.
func (m *JobManager) Cancel(storageID int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[storageID]
	if !ok || j.status.State != StateScanning {
		return false
	}
	j.cancel()
	return true
}
