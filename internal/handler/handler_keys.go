package handler

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

)

func (s *Server) handleKeys(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		entries, err := os.ReadDir(s.cfg.KeysDir)
		if err != nil {
			jsonErr(w, "read keys dir: "+err.Error())
			return
		}
		var keys []map[string]interface{}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".pem") {
				continue
			}
			info, _ := e.Info()
			keys = append(keys, map[string]interface{}{
				"name": e.Name(),
				"size": info.Size(),
				"time": info.ModTime().Format("2006-01-02 15:04"),
			})
		}
		if keys == nil {
			keys = []map[string]interface{}{}
		}
		jsonOK(w, keys)

	case http.MethodPost:
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			jsonErr(w, "parse multipart: "+err.Error())
			return
		}
		files := r.MultipartForm.File["files"]
		if len(files) == 0 {
			jsonErr(w, "no files uploaded (field name: files)")
			return
		}
		var saved []string
		for _, fh := range files {
			if !strings.HasSuffix(strings.ToLower(fh.Filename), ".pem") {
				jsonErr(w, "only .pem files allowed: "+fh.Filename)
				return
			}
			// sanitize: base name only
			name := filepath.Base(fh.Filename)
			dst := filepath.Join(s.cfg.KeysDir, name)
			src, err := fh.Open()
			if err != nil {
				jsonErr(w, "open upload: "+err.Error())
				return
			}
			out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
			if err != nil {
				src.Close()
				if os.IsPermission(err) {
					jsonErr(w, fmt.Sprintf("permission denied writing key file — check that %s is writable", s.cfg.KeysDir))
				} else {
					jsonErr(w, "create file: "+err.Error())
				}
				return
			}
			if _, err := io.Copy(out, src); err != nil {
				out.Close()
				src.Close()
				jsonErr(w, "write file: "+err.Error())
				return
			}
			out.Close()
			src.Close()
			saved = append(saved, name)
		}
		s.audit(0, "keys:upload", strings.Join(saved, ","), r)
		jsonOK(w, map[string]interface{}{"saved": saved})

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleKeyByID(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/keys/")
	name = strings.TrimSuffix(name, "/")
	name = filepath.Base(name)
	if name == "" || name == "." {
		jsonErr(w, "invalid key name")
		return
	}
	switch r.Method {
	case http.MethodDelete:
		path := filepath.Join(s.cfg.KeysDir, name)
		if err := os.Remove(path); err != nil {
			jsonErr(w, "delete key: "+err.Error())
			return
		}
		s.audit(0, "keys:delete", name, r)
		jsonOK(w, map[string]string{"status": "ok"})
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}
