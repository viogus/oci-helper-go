package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/oracle/oci-go-sdk/v65/core"
)

// netbootRescueTag is the freeform tag key used to store the original boot
// volume ID on the instance while rescue mode is active. The tag is removed
// when rescue is stopped.
const netbootRescueTag = "oci-helper-netboot-rescue-orig-bv"

// handleNetbootRescue boots an instance from a rescue image by detaching the
// current boot volume, creating a rescue boot volume from the specified (or
// default Oracle Linux) image via a temporary instance, and attaching it.
//
//	POST /api/instances/netboot-rescue
func (s *Server) handleNetbootRescue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TenantID      int64  `json:"tenant_id"`
		InstanceID    string `json:"instance_id"`
		RescueImageID string `json:"rescue_image_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	if req.TenantID == 0 || req.InstanceID == "" {
		jsonErr(w, "tenant_id and instance_id required")
		return
	}

	client, tenant, ok := s.clientForInstance(req.TenantID, req.InstanceID, w)
	if !ok {
		return
	}

	ocid := bareOCID(req.InstanceID)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	// Step 1: Get current instance details.
	inst, err := client.GetInstance(ctx, ocid)
	if err != nil {
		jsonErr(w, "get instance: "+err.Error())
		return
	}

	// Step 2: Check if already in rescue mode.
	if inst.FreeformTags != nil {
		if _, rescueActive := inst.FreeformTags[netbootRescueTag]; rescueActive {
			jsonErr(w, "instance is already in netboot rescue mode; stop rescue first")
			return
		}
	}

	// Step 3: Stop instance if not already stopped.
	state := string(inst.LifecycleState)
	if state == "RUNNING" || state == "STARTING" {
		if _, err := client.InstanceAction(ctx, ocid, core.InstanceActionActionStop); err != nil {
			jsonErr(w, "stop instance: "+err.Error())
			return
		}
		if !client.WaitForState(ctx, ocid, "STOPPED", 120*time.Second) {
			jsonErr(w, "timeout waiting for instance to stop")
			return
		}
	} else if state != "STOPPED" {
		jsonErr(w, fmt.Sprintf("instance must be RUNNING or STOPPED (current: %s)", state))
		return
	}

	// Step 4: Get current boot volume attachment and save original boot volume ID.
	compartmentID := tenant.TenancyOCID
	attachment, err := client.GetBootVolumeAttachment(ctx, compartmentID, ocid)
	if err != nil {
		jsonErr(w, "get boot volume attachment: "+err.Error())
		return
	}
	if attachment.BootVolumeId == nil {
		jsonErr(w, "boot volume id not found in attachment")
		return
	}
	originalBV := *attachment.BootVolumeId

	// Step 5: Get subnet ID from instance VNICs (needed for temp instance).
	vnics, vnicErr := client.GetInstanceVNICs(ctx, compartmentID, ocid)
	if vnicErr != nil || len(vnics) == 0 || vnics[0].SubnetId == nil {
		jsonErr(w, "cannot determine subnet for instance; needed to create rescue boot volume")
		return
	}
	subnetID := *vnics[0].SubnetId

	// Step 6: Detach current boot volume.
	if err := client.DetachBootVolume(ctx, *attachment.Id); err != nil {
		jsonErr(w, "detach boot volume: "+err.Error())
		return
	}

	// Step 7: Determine rescue image.
	rescueImageID := req.RescueImageID
	if rescueImageID == "" {
		images, imgErr := client.ListImages(ctx, compartmentID, "Oracle Linux")
		if imgErr != nil || len(images) == 0 {
			// Rollback: re-attach original boot volume.
			if _, attachErr := client.AttachBootVolume(ctx, originalBV, ocid); attachErr != nil {
				log.Printf("[netboot-rescue] rollback attach original BV failed: %v", attachErr)
			}
			jsonErr(w, "no rescue image available in this region")
			return
		}
		rescueImageID = *images[0].Id
	}

	// Step 8: Determine AD for temp instance.
	ad := ""
	if inst.AvailabilityDomain != nil {
		ad = *inst.AvailabilityDomain
	}

	// Step 9: Launch temp instance from rescue image, detach its boot volume,
	// and terminate the temp instance. Returns the rescue boot volume ID.
	rescueBVName := fmt.Sprintf("netboot-rescue-%s", ocid)
	rescueBV, err := client.CreateBootVolumeFromImage(ctx, compartmentID, ad, rescueImageID, subnetID, rescueBVName, "VM.Standard.E2.1.Micro")
	if err != nil {
		// Rollback: re-attach original boot volume.
		if _, attachErr := client.AttachBootVolume(ctx, originalBV, ocid); attachErr != nil {
			log.Printf("[netboot-rescue] rollback attach original BV failed: %v", attachErr)
		}
		jsonErr(w, "create rescue boot volume: "+err.Error())
		return
	}

	// Step 10: Attach rescue boot volume.
	_, err = client.AttachBootVolume(ctx, rescueBV, ocid)
	if err != nil {
		// Rollback: delete rescue BV and re-attach original.
		if delErr := client.DeleteBootVolume(ctx, rescueBV); delErr != nil {
			log.Printf("[netboot-rescue] rollback delete rescue BV failed: %v", delErr)
		}
		if _, attachErr := client.AttachBootVolume(ctx, originalBV, ocid); attachErr != nil {
			log.Printf("[netboot-rescue] rollback attach original BV failed: %v", attachErr)
		}
		jsonErr(w, "attach rescue boot volume: "+err.Error())
		return
	}

	// Step 11: Tag instance with original boot volume ID so we can restore it later.
	if tagErr := client.UpdateInstanceFreeformTags(ctx, ocid, map[string]string{
		netbootRescueTag: originalBV,
	}); tagErr != nil {
		log.Printf("[netboot-rescue] WARNING: failed to tag instance with original BV: %v", tagErr)
	}

	// Step 12: Start instance with rescue boot volume.
	if _, err := client.InstanceAction(ctx, ocid, core.InstanceActionActionStart); err != nil {
		jsonErr(w, "start instance with rescue boot volume: "+err.Error())
		return
	}

	s.audit(req.TenantID, "instance:netboot-rescue", fmt.Sprintf("%s rescue_bv=%s", req.InstanceID, rescueBV), r)
	jsonOK(w, map[string]interface{}{
		"status":               "ok",
		"message":              "instance booting into rescue mode",
		"original_boot_volume": originalBV,
		"rescue_boot_volume":   rescueBV,
		"rescue_image_id":      rescueImageID,
	})
}

// handleNetbootRescueStop stops rescue mode: detaches the rescue boot volume,
// re-attaches the original boot volume, deletes the rescue boot volume, and
// starts the instance.
//
//	POST /api/instances/netboot-rescue/stop
func (s *Server) handleNetbootRescueStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TenantID   int64  `json:"tenant_id"`
		InstanceID string `json:"instance_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	if req.TenantID == 0 || req.InstanceID == "" {
		jsonErr(w, "tenant_id and instance_id required")
		return
	}

	client, tenant, ok := s.clientForInstance(req.TenantID, req.InstanceID, w)
	if !ok {
		return
	}

	ocid := bareOCID(req.InstanceID)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	// Step 1: Get current instance details.
	inst, err := client.GetInstance(ctx, ocid)
	if err != nil {
		jsonErr(w, "get instance: "+err.Error())
		return
	}

	// Step 2: Read original boot volume ID from freeform tags.
	originalBV := ""
	if inst.FreeformTags != nil {
		originalBV = inst.FreeformTags[netbootRescueTag]
	}
	if originalBV == "" {
		jsonErr(w, "instance is not in netboot rescue mode (no rescue tag found)")
		return
	}

	// Step 3: Stop instance if not already stopped.
	state := string(inst.LifecycleState)
	if state == "RUNNING" || state == "STARTING" {
		if _, err := client.InstanceAction(ctx, ocid, core.InstanceActionActionStop); err != nil {
			jsonErr(w, "stop instance: "+err.Error())
			return
		}
		if !client.WaitForState(ctx, ocid, "STOPPED", 120*time.Second) {
			jsonErr(w, "timeout waiting for instance to stop")
			return
		}
	} else if state != "STOPPED" {
		jsonErr(w, fmt.Sprintf("instance must be RUNNING or STOPPED (current: %s)", state))
		return
	}

	// Step 4: Get current (rescue) boot volume attachment.
	compartmentID := tenant.TenancyOCID
	attachment, err := client.GetBootVolumeAttachment(ctx, compartmentID, ocid)
	if err != nil {
		jsonErr(w, "get boot volume attachment: "+err.Error())
		return
	}

	// Step 5: Detach rescue boot volume and save its ID for deletion.
	rescueBV := ""
	if attachment.BootVolumeId != nil {
		rescueBV = *attachment.BootVolumeId
	}
	if err := client.DetachBootVolume(ctx, *attachment.Id); err != nil {
		jsonErr(w, "detach rescue boot volume: "+err.Error())
		return
	}

	// Step 6: Re-attach original boot volume.
	if _, err := client.AttachBootVolume(ctx, originalBV, ocid); err != nil {
		jsonErr(w, "attach original boot volume: "+err.Error())
		return
	}

	// Step 7: Clear the rescue tag from instance freeform tags.
	cleanedTags := make(map[string]string)
	for k, v := range inst.FreeformTags {
		if k != netbootRescueTag {
			cleanedTags[k] = v
		}
	}
	if tagErr := client.UpdateInstanceFreeformTags(ctx, ocid, cleanedTags); tagErr != nil {
		log.Printf("[netboot-rescue-stop] WARNING: failed to clear rescue tag: %v", tagErr)
	}

	// Step 8: Delete the rescue boot volume (best effort, non-fatal).
	if rescueBV != "" {
		if delErr := client.DeleteBootVolume(ctx, rescueBV); delErr != nil {
			log.Printf("[netboot-rescue-stop] WARNING: failed to delete rescue BV %s: %v", rescueBV, delErr)
		}
	}

	// Step 9: Start instance with original boot volume.
	if _, err := client.InstanceAction(ctx, ocid, core.InstanceActionActionStart); err != nil {
		jsonErr(w, "start instance with original boot volume: "+err.Error())
		return
	}

	s.audit(req.TenantID, "instance:netboot-rescue-stop", req.InstanceID, r)
	jsonOK(w, map[string]string{
		"status":               "ok",
		"message":              "instance booting from original boot volume",
		"original_boot_volume": originalBV,
	})
}
