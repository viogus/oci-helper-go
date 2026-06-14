package handler

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/viogus/oci-helper-go/internal/db"
	ociclient "github.com/viogus/oci-helper-go/internal/oci"
)

const pollInterval = 5 * time.Second

type Worker struct {
	store *db.Store
}

func NewWorker(store *db.Store) *Worker {
	return &Worker{store: store}
}

func (w *Worker) Run() {
	log.Println("[worker] started")
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[worker] panic recovered: %v — restarting", r)
			go w.Run()
		}
	}()
	for {
		w.processNext()
		time.Sleep(pollInterval)
	}
}

func (w *Worker) processNext() {
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
		switch t.Type {
		case "batch_start":
			w.runBatchStart(t)
		default:
			w.store.UpdateTaskStatus(t.ID, "failed", 0, "unknown task type: "+t.Type)
		}
		return // one at a time
	}
}

func (w *Worker) runBatchStart(task *db.Task) {
	w.store.UpdateTaskStatus(task.ID, "running", 0, "starting...")

	var payload struct {
		TenantID    int64    `json:"tenantId"`
		InstanceIDs []string `json:"instanceIds"`
	}
	if err := json.Unmarshal([]byte(task.Payload), &payload); err != nil {
		w.store.UpdateTaskStatus(task.ID, "failed", 0, "invalid payload: "+err.Error())
		return
	}

	tenant, err := w.store.GetTenant(payload.TenantID)
	if err != nil || tenant == nil {
		w.store.UpdateTaskStatus(task.ID, "failed", 0, "tenant not found")
		return
	}

	client, err := ociclient.NewClient(tenant)
	if err != nil {
		w.store.UpdateTaskStatus(task.ID, "failed", 0, "oci client: "+err.Error())
		return
	}

	total := len(payload.InstanceIDs)
	for i, instID := range payload.InstanceIDs {
		progress := (i * 100) / total
		w.store.UpdateTaskStatus(task.ID, "running", progress, instID)

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		_, err := client.GetInstance(ctx, instID)
		cancel()
		if err != nil {
			w.store.UpdateTaskStatus(task.ID, "running", progress, "skip "+instID+": "+err.Error())
			continue
		}

		ctx2, cancel2 := context.WithTimeout(context.Background(), 120*time.Second)
		_, err = client.InstanceAction(ctx2, instID, core.InstanceActionActionStart)
		cancel2()
		if err != nil {
			w.store.UpdateTaskStatus(task.ID, "running", progress, "fail "+instID+": "+err.Error())
			continue
		}
	}

	w.store.UpdateTaskStatus(task.ID, "completed", 100, "done")
}
