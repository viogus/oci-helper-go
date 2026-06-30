package handler

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

var logWSUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type logWSMsg struct {
	Type  string   `json:"type"`
	Data  string   `json:"data,omitempty"`
	Time  string   `json:"time,omitempty"`
	Lines []string `json:"lines,omitempty"`
}

// handleLogWS streams log file updates over WebSocket.
// GET /api/logs/ws?tail=100
func (s *Server) handleLogWS(w http.ResponseWriter, r *http.Request) {
	tail := 100
	if v := r.URL.Query().Get("tail"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 2000 {
			tail = n
		}
	}

	conn, err := logWSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[wslog] upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// 60s read deadline, reset on pong
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	logFile := s.cfg.LogFile
	f, err := os.Open(logFile)
	if err != nil {
		sendLogWS(conn, logWSMsg{Type: "error", Data: "Cannot open log file: " + err.Error()})
		return
	}
	defer f.Close()

	// Send last N lines as initial batch
	initLines := readLastNLines(f, tail)
	if len(initLines) > 0 {
		sendLogWS(conn, logWSMsg{Type: "init", Lines: initLines})
	}

	// Seek to end for live tail
	f.Seek(0, io.SeekEnd)
	lastSize, _ := f.Seek(0, io.SeekCurrent)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	// Read goroutine to detect client disconnect
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			fi, err := os.Stat(logFile)
			if err != nil {
				continue
			}
			if fi.Size() < lastSize {
				// File was truncated/rotated — reopen to get the new inode.
				if err := sendLogWS(conn, logWSMsg{Type: "reset"}); err != nil {
					return
				}
				f.Close()
				var err error
				f, err = os.Open(logFile)
				if err != nil {
					sendLogWS(conn, logWSMsg{Type: "error", Data: "Cannot reopen log: " + err.Error()})
					return
				}
				lastSize = 0
			}
			if fi.Size() > lastSize {
				f.Seek(lastSize, io.SeekStart)
				scanner := bufio.NewScanner(f)
				// 1MB buffer for long lines
				scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
				for scanner.Scan() {
					line := scanner.Text()
					if err := sendLogWS(conn, logWSMsg{
						Type: "line",
						Data: line,
						Time: time.Now().Format(time.RFC3339),
					}); err != nil {
						return
					}
				}
				lastSize, _ = f.Seek(0, io.SeekCurrent)
			}
		}
	}
}

func sendLogWS(conn *websocket.Conn, msg logWSMsg) error {
	data, _ := json.Marshal(msg)
	conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	return conn.WriteMessage(websocket.TextMessage, data)
}

// readLastNLines reads the last n lines from a file.
func readLastNLines(f *os.File, n int) []string {
	f.Seek(0, io.SeekStart)
	var allLines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}
	if len(allLines) <= n {
		return allLines
	}
	return allLines[len(allLines)-n:]
}
