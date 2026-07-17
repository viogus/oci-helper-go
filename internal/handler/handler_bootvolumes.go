package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

)

func (s *Server) handleBootVolumes(w http.ResponseWriter, r *http.Request) {
	client, t, ok := s.ociClientFromQuery(w, r)
	if !ok {
		return
	}
	vols, err := client.ListBootVolumes(r.Context(), t.TenancyOCID)
	if err != nil {
		jsonErr(w, "list boot volumes: "+err.Error())
		return
	}
	jsonOK(w, vols)
}

func (s *Server) handleBootVolumeByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/boot-volumes/")
	path = strings.TrimSuffix(path, "/")
	parts := strings.SplitN(path, "/", 2)
	bootVolumeID := parts[0]
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}

	if r.Method != http.MethodPost {
		// GET boot volume details
		client, _, ok := s.ociClientFromQuery(w, r)
		if !ok {
			return
		}
		vol, err := client.GetBootVolume(r.Context(), bootVolumeID)
		if err != nil {
			jsonErr(w, "get boot volume: "+err.Error())
			return
		}
		jsonOK(w, vol)
		return
	}

	var req struct {
		TenantID   int64  `json:"tenantId"`
		SizeInGBs  int64  `json:"sizeInGBs"`
		InstanceID string `json:"instanceId"`
		VpusPerGB  int64  `json:"vpusPerGB"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}

	client, t, ok := s.getTenantClient(req.TenantID, w)
	if !ok {
		return
	}

	switch action {
	case "resize":
		if req.SizeInGBs <= 0 {
			jsonErr(w, "sizeInGBs required")
			return
		}
		vol, err := client.UpdateBootVolume(r.Context(), bootVolumeID, req.SizeInGBs, "")
		if err != nil {
			jsonErr(w, "resize: "+err.Error())
			return
		}
		s.audit(req.TenantID, "bootvolume:resize", fmt.Sprintf("%s → %dGB", bootVolumeID, req.SizeInGBs), r)
		jsonOK(w, vol)
	case "attach":
		if req.InstanceID == "" {
			jsonErr(w, "instanceId required")
			return
		}
		instanceID := req.InstanceID
		if parts := strings.SplitN(instanceID, ":", 2); len(parts) == 2 {
			instanceID = parts[1]
		}
		att, err := client.AttachBootVolume(r.Context(), bootVolumeID, instanceID)
		if err != nil {
			jsonErr(w, "attach: "+err.Error())
			return
		}
		s.audit(req.TenantID, "bootvolume:attach", fmt.Sprintf("%s → %s", bootVolumeID, req.InstanceID), r)
		jsonOK(w, att)
	case "detach":
		// find attachment ID
		attachments, err := client.ListBootVolumeAttachments(r.Context(), t.TenancyOCID, "")
		if err != nil {
			jsonErr(w, "list attachments: "+err.Error())
			return
		}
		var attachmentID string
		for _, a := range attachments {
			if strOr(a.BootVolumeId, "") == bootVolumeID {
				attachmentID = strOr(a.Id, "")
				break
			}
		}
		if attachmentID == "" {
			jsonErr(w, "no attachment found for boot volume")
			return
		}
		if err := client.DetachBootVolume(r.Context(), attachmentID); err != nil {
			jsonErr(w, "detach: "+err.Error())
			return
		}
		s.audit(req.TenantID, "bootvolume:detach", bootVolumeID, r)
		jsonOK(w, map[string]string{"status": "ok"})
	case "terminate":
		if err := client.DeleteBootVolume(r.Context(), bootVolumeID); err != nil {
			jsonErr(w, "terminate: "+err.Error())
			return
		}
		s.audit(req.TenantID, "bootvolume:terminate", bootVolumeID, r)
		jsonOK(w, map[string]string{"status": "ok"})
	case "config":
		if req.SizeInGBs <= 0 && req.VpusPerGB <= 0 {
			jsonErr(w, "sizeInGBs or vpusPerGB required")
			return
		}
		vol, err := client.UpdateBootVolumeWithVPU(r.Context(), bootVolumeID, req.SizeInGBs, "", req.VpusPerGB)
		if err != nil {
			jsonErr(w, "config: "+err.Error())
			return
		}
		s.audit(req.TenantID, "bootvolume:config", fmt.Sprintf("%s vpu=%d size=%d", bootVolumeID, req.VpusPerGB, req.SizeInGBs), r)
		jsonOK(w, vol)
	default:
		jsonErr(w, "unknown action: "+action+". use resize|attach|detach|terminate|config")
	}
}
