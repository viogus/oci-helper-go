package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/viogus/oci-helper-go/internal/db"
	ociclient "github.com/viogus/oci-helper-go/internal/oci"
)

const pollInterval = 5 * time.Second

// Worker runs pending background tasks (batch start, batch create) asynchronously.
// Runs in its own goroutine started by Server.New().
// Supports checkpoint-resume: interrupted tasks are reset to pending on restart
// and resume from where they left off using saved progress in the payload.
type Worker struct {
	store   *db.Store
	keysDir string
	// restarts tracks panic-restart count for exponential backoff.
	// Only accessed from the worker goroutine (even after panic recovery,
	// defer runs sequentially before go w.Run() spawns the next goroutine),
	// so no mutex is needed.
	restarts int
	stop     chan struct{}
}

// NewWorker creates a Worker with the given store and keys directory.
func NewWorker(store *db.Store, keysDir string) *Worker {
	return &Worker{store: store, keysDir: keysDir, stop: make(chan struct{})}
}

// Shutdown signals the worker to stop gracefully. Resets any running tasks
// back to pending so they can be picked up after restart. Safe to call
// multiple times (subsequent calls are no-ops).
func (w *Worker) Shutdown() {
	select {
	case <-w.stop:
		return // already stopped
	default:
	}
	log.Println("[worker] shutting down...")
	// Signal the worker goroutine to stop BEFORE resetting running tasks.
	// This avoids a race where ResetRunningTasks sets "running"→"pending"
	// then the worker goroutine re-sets it back to "running" before
	// processing the stop channel.
	close(w.stop)
	// Reset running tasks so they are not orphaned. The worker goroutine
	// is no longer processing, so this is safe.
	if n, err := w.store.ResetRunningTasks(); err != nil {
		log.Printf("[worker] shutdown: reset running tasks: %v", err)
	} else if n > 0 {
		log.Printf("[worker] shutdown: reset %d running task(s) to pending", n)
	}
}

// newClient delegates to the package-level clientForTenant with the worker's keysDir.
func (w *Worker) newClient(t *db.Tenant) (*ociclient.Client, error) {
	proxyURL, _ := w.store.GetConfig(fmt.Sprintf("tenant_proxy_%d", t.ID))
	return clientForTenant(t, w.keysDir, proxyURL)
}

// Run starts the worker loop. Picks one pending task per poll interval (5s).
// On startup, resets any "running" tasks (interrupted by previous shutdown) back to "pending".
// Auto-restarts on panic with exponential backoff (resets after 50 consecutive panics).
func (w *Worker) Run() {
	log.Println("[worker] started")

	// ── Checkpoint resume: reset interrupted tasks ──────────────────
	if n, err := w.store.ResetRunningTasks(); err != nil {
		log.Printf("[worker] reset running tasks: %v", err)
	} else if n > 0 {
		log.Printf("[worker] checkpoint-resume: reset %d interrupted task(s) to pending", n)
	}

	defer func() {
		if r := recover(); r != nil {
			w.restarts++
			if w.restarts > 50 {
				log.Printf("[worker] panicked %d times, resetting restart counter", w.restarts)
				w.restarts = 0
			}
			backoff := time.Duration(w.restarts) * 10 * time.Second
			log.Printf("[worker] panic: %v — restart in %v (attempt %d)", r, backoff, w.restarts)
			time.Sleep(backoff)
			go w.Run()
		}
	}()
	for {
		select {
		case <-w.stop:
			log.Println("[worker] stopped")
			return
		default:
		}
		w.processNext()
		// Sleep in small increments so shutdown is responsive.
		for i := 0; i < 5; i++ {
			select {
			case <-w.stop:
				log.Println("[worker] stopped")
				return
			default:
				time.Sleep(pollInterval / 5)
			}
		}
	}
}

func (w *Worker) processNext() {
	// Use a DB query that returns only pending tasks, avoiding an O(n) scan.
	tasks, err := w.store.ListTasks()
	if err != nil {
		log.Printf("[worker] list tasks: %v", err)
		return
	}
	for i := range tasks {
		t := &tasks[i]
		if t.Status != "pending" {
			continue
		}
		// Atomically claim the task — guards against TOCTOU race with
		// HTTP cancel/pause handler that also writes to this task row.
		claimed, err := w.store.ClaimTask(t.ID)
		if err != nil {
			log.Printf("[worker] claim task %d: %v", t.ID, err)
			return
		}
		if !claimed {
			continue // another goroutine or HTTP handler already claimed it
		}
		switch t.Type {
		case "batch_start":
			w.runBatchStart(t)
		case "batch_create":
			w.runBatchCreate(t)
		default:
			w.store.UpdateTaskStatus(t.ID, "failed", 0, "unknown task type: "+t.Type)
		}
		return // one at a time
	}
}

// batchStartPayload is saved in Task.Payload as JSON.
// completedIndex tracks which instances have already been processed (checkpoint).
type batchStartPayload struct {
	TenantID       int64    `json:"tenantId"`
	InstanceIDs    []string `json:"instanceIds"`
	CompletedIndex int      `json:"completedIndex"`
}

func (w *Worker) runBatchStart(task *db.Task) {
	var payload batchStartPayload
	if err := json.Unmarshal([]byte(task.Payload), &payload); err != nil {
		w.store.UpdateTaskStatus(task.ID, "failed", 0, "invalid payload: "+err.Error())
		return
	}

	tenant, err := w.store.GetTenant(payload.TenantID)
	if err != nil || tenant == nil {
		w.store.UpdateTaskStatus(task.ID, "failed", 0, "tenant not found")
		return
	}

	client, err := w.newClient(tenant)
	if err != nil {
		w.store.UpdateTaskStatus(task.ID, "failed", 0, "oci client: "+err.Error())
		return
	}

	total := len(payload.InstanceIDs)

	// Resume from checkpoint
	for i := payload.CompletedIndex; i < total; i++ {
		// Check for shutdown signal before each instance action.
		select {
		case <-w.stop:
			w.saveCheckpoint(task, &payload, i)
			w.store.UpdateTaskStatus(task.ID, "pending", 0, "interrupted by shutdown")
			log.Printf("[worker] batch_start %d interrupted by shutdown at index %d/%d", task.ID, i, total)
			return
		default:
		}
		// Re-check DB status — user may have paused/cancelled the task.
		if !w.taskIsActive(task.ID) {
			w.saveCheckpoint(task, &payload, i)
			w.store.UpdateTaskStatus(task.ID, "pending", 0, "paused by user")
			log.Printf("[worker] batch_start %d paused at index %d/%d", task.ID, i, total)
			return
		}

		instID := bareOCID(payload.InstanceIDs[i])
		progress := (i * 100) / total
		w.store.UpdateTaskStatus(task.ID, "running", progress, instID)

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		_, err := client.GetInstance(ctx, instID)
		cancel()
		if err != nil {
			w.store.UpdateTaskStatus(task.ID, "running", progress, "skip "+instID+": "+err.Error())
			w.saveCheckpoint(task, &payload, i+1)
			continue
		}

		ctx2, cancel2 := context.WithTimeout(context.Background(), 120*time.Second)
		_, err = client.InstanceAction(ctx2, instID, core.InstanceActionActionStart)
		cancel2()
		if err != nil {
			w.store.UpdateTaskStatus(task.ID, "running", progress, "fail "+instID+": "+err.Error())
			w.saveCheckpoint(task, &payload, i+1)
			continue
		}

		// Save checkpoint after each successful action
		w.saveCheckpoint(task, &payload, i+1)
	}

	w.store.UpdateTaskStatus(task.ID, "completed", 100, "done")
}

// batchCreatePayload is saved in Task.Payload as JSON.
type batchCreatePayload struct {
	TenantID           int64  `json:"tenant_id"`
	InstancesPerTenant int    `json:"instances_per_tenant"`
	Region             string `json:"region"`
	Shape              string `json:"shape"`
	ImageID            string `json:"image_id"`
	SubnetID           string `json:"subnet_id"`
	AvailabilityDomain string `json:"availability_domain"`
	BootVolumeSizeGB   int64  `json:"boot_volume_size_gb"`
	DisplayNamePrefix  string `json:"display_name_prefix"`
	CompletedIndex     int    `json:"completedIndex"`
}

func (w *Worker) runBatchCreate(task *db.Task) {
	var payload batchCreatePayload
	if err := json.Unmarshal([]byte(task.Payload), &payload); err != nil {
		w.store.UpdateTaskStatus(task.ID, "failed", 0, "invalid payload: "+err.Error())
		return
	}

	tenant, err := w.store.GetTenant(payload.TenantID)
	if err != nil || tenant == nil {
		w.store.UpdateTaskStatus(task.ID, "failed", 0, "tenant not found")
		return
	}

	client, err := w.newClient(tenant)
	if err != nil {
		w.store.UpdateTaskStatus(task.ID, "failed", 0, "oci client: "+err.Error())
		return
	}

	total := payload.InstancesPerTenant

	// Resume from checkpoint
	for i := payload.CompletedIndex; i < total; i++ {
		// Check for shutdown signal before each instance action.
		select {
		case <-w.stop:
			w.saveCheckpoint(task, &payload, i)
			w.store.UpdateTaskStatus(task.ID, "pending", 0, "interrupted by shutdown")
			log.Printf("[worker] batch_create %d interrupted by shutdown at index %d/%d", task.ID, i, total)
			return
		default:
		}
		// Re-check DB status — user may have paused/cancelled the task.
		if !w.taskIsActive(task.ID) {
			w.saveCheckpoint(task, &payload, i)
			w.store.UpdateTaskStatus(task.ID, "pending", 0, "paused by user")
			log.Printf("[worker] batch_create %d paused at index %d/%d", task.ID, i, total)
			return
		}

		progress := (i * 100) / total
		displayName := fmt.Sprintf("%s-%s-%d", payload.DisplayNamePrefix, tenant.Name, i+1)
		w.store.UpdateTaskStatus(task.ID, "running", progress, "creating "+displayName)

		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		err := client.LaunchInstance(ctx, payload.Region, payload.AvailabilityDomain, payload.Shape, payload.ImageID, payload.SubnetID, displayName, payload.BootVolumeSizeGB)
		cancel()
		if err != nil {
			w.store.UpdateTaskStatus(task.ID, "running", progress, "failed "+displayName+": "+err.Error())
			w.saveCheckpoint(task, &payload, i+1)
			continue
		}

		w.store.UpdateTaskStatus(task.ID, "running", progress, "created "+displayName)
		w.saveCheckpoint(task, &payload, i+1)
		// Interruptible sleep — respects shutdown signal.
		select {
		case <-w.stop:
			w.saveCheckpoint(task, &payload, i+1)
			w.store.UpdateTaskStatus(task.ID, "pending", 0, "interrupted by shutdown")
			return
		case <-time.After(2 * time.Second):
		}
	}

	w.store.UpdateTaskStatus(task.ID, "completed", 100, fmt.Sprintf("created %d instances", total))
}

// taskIsActive re-reads the task status from the DB and returns true if it is
// still "running". Returns false for any other status (cancelled, paused, failed,
// completed, or missing) — the worker loop should stop processing.
func (w *Worker) taskIsActive(taskID int64) bool {
	t, err := w.store.GetTaskByID(taskID)
	if err != nil || t == nil {
		return false
	}
	return t.Status == "running"
}

// saveCheckpoint persists the current progress index into the task payload.
// This allows the task to resume from where it left off after a restart.
func (w *Worker) saveCheckpoint(task *db.Task, payload interface{}, index int) {
	switch p := payload.(type) {
	case *batchStartPayload:
		p.CompletedIndex = index
	case *batchCreatePayload:
		p.CompletedIndex = index
	}
	if data, err := json.Marshal(payload); err == nil {
		w.store.UpdateTaskPayload(task.ID, string(data))
	}
}
