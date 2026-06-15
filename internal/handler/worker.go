package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/viogus/oci-helper-go/internal/db"
	ociclient "github.com/viogus/oci-helper-go/internal/oci"
)

const pollInterval = 5 * time.Second

type Worker struct {
	store   *db.Store
	keysDir string
	restarts int
}

func NewWorker(store *db.Store, keysDir string) *Worker {
	return &Worker{store: store, keysDir: keysDir}
}

func (w *Worker) newClient(t *db.Tenant) (*ociclient.Client, error) {
	if t.KeyFile != "" && !filepath.IsAbs(t.KeyFile) {
		resolved := *t
		resolved.KeyFile = filepath.Join(w.keysDir, t.KeyFile)
		return ociclient.NewClient(&resolved)
	}
	return ociclient.NewClient(t)
}

func (w *Worker) Run() {
	log.Println("[worker] started")
	defer func() {
		if r := recover(); r != nil {
			w.restarts++
			if w.restarts > 5 {
				log.Printf("[worker] panicked %d times, giving up", w.restarts)
				return
			}
			backoff := time.Duration(w.restarts) * 10 * time.Second
			log.Printf("[worker] panic: %v — restart in %v (attempt %d)", r, backoff, w.restarts)
			time.Sleep(backoff)
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
		case "batch_create":
			w.runBatchCreate(t)
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

	client, err := w.newClient(tenant)
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

func (w *Worker) runBatchCreate(task *db.Task) {
	w.store.UpdateTaskStatus(task.ID, "running", 0, "creating instances...")

	var payload struct {
		TenantID           int64  `json:"tenant_id"`
		InstancesPerTenant int    `json:"instances_per_tenant"`
		Region             string `json:"region"`
		Shape              string `json:"shape"`
		ImageID            string `json:"image_id"`
		SubnetID           string `json:"subnet_id"`
		AvailabilityDomain string `json:"availability_domain"`
		BootVolumeSizeGB   int64  `json:"boot_volume_size_gb"`
		DisplayNamePrefix  string `json:"display_name_prefix"`
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

	client, err := w.newClient(tenant)
	if err != nil {
		w.store.UpdateTaskStatus(task.ID, "failed", 0, "oci client: "+err.Error())
		return
	}

	total := payload.InstancesPerTenant
	for i := 0; i < total; i++ {
		progress := (i * 100) / total
		displayName := fmt.Sprintf("%s-%s-%d", payload.DisplayNamePrefix, tenant.Name, i+1)
		w.store.UpdateTaskStatus(task.ID, "running", progress, "creating "+displayName)

		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		err := client.LaunchInstance(ctx, payload.Region, payload.AvailabilityDomain, payload.Shape, payload.ImageID, payload.SubnetID, displayName, payload.BootVolumeSizeGB)
		cancel()
		if err != nil {
			w.store.UpdateTaskStatus(task.ID, "running", progress, "failed "+displayName+": "+err.Error())
			continue
		}

		w.store.UpdateTaskStatus(task.ID, "running", progress, "created "+displayName)
		time.Sleep(2 * time.Second)
	}

	w.store.UpdateTaskStatus(task.ID, "completed", 100, fmt.Sprintf("created %d instances", total))
}
