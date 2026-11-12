package service

import (
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	ttsLogKeep    = 200      // entries kept in memory for GET /ttshop/logs
	ttsLogMaxFile = 10 << 20 // the log file is rotated to ".1" above this size (at start-up and while running): two files, about two days
	ttsLogBodyMax = 300
)

// ttsLog records every call made to the TikTok Shop Open Platform so they can be checked later: one line per call
// with method, path, HTTP status, TikTok code/message/request_id and duration. It never records tokens, the app
// secret, the signature or shop_cipher: only the path and a short allow-list of harmless query keys.
type ttsLog struct {
	mu   sync.Mutex
	ring []string
	file *os.File
	name string // path of the file ("" when only in memory)
	size int64  // bytes in the file now
}

// path is the log file, or "" when calls are only kept in memory.
func (l *ttsLog) path() string {
	if l == nil {
		return ""
	}
	return l.name
}

// newTTSLog opens (append) the log file. An empty path, or a file that cannot be opened, keeps logging in memory
// and on stdout only.
func newTTSLog(path string) *ttsLog {
	l := &ttsLog{}
	if path == "" {
		return l
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("[ttshop] không tạo được thư mục log %s: %v", filepath.Dir(path), err)
		return l
	}
	if st, err := os.Stat(path); err == nil && st.Size() > ttsLogMaxFile {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		log.Printf("[ttshop] không mở được file log %s: %v", path, err)
		return l
	}
	l.file, l.name = f, path
	if st, err := f.Stat(); err == nil {
		l.size = st.Size()
	}
	return l
}

// rotate moves a file that grew past ttsLogMaxFile to ".1" and starts a new one, so a log that now lives in a volume
// across deploys cannot fill the disk. Called with l.mu held.
func (l *ttsLog) rotate() {
	if l.file == nil || l.size <= ttsLogMaxFile {
		return
	}
	_ = l.file.Close()
	l.file = nil
	_ = os.Rename(l.name, l.name+".1")
	f, err := os.OpenFile(l.name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		log.Printf("[ttshop] không mở lại được file log %s: %v", l.name, err)
		return
	}
	l.file, l.size = f, 0
}

func (l *ttsLog) add(line string) {
	line = time.Now().UTC().Format(time.RFC3339) + " " + line
	log.Print("[ttshop] " + line)
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		n, _ := l.file.WriteString(line + "\n")
		l.size += int64(n)
		l.rotate()
	}
	l.ring = append(l.ring, line)
	if len(l.ring) > ttsLogKeep {
		l.ring = l.ring[len(l.ring)-ttsLogKeep:]
	}
}

// close releases the log file (used by tests; the server keeps it open until it exits).
func (l *ttsLog) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
}

// recent returns the newest n lines, oldest first.
func (l *ttsLog) recent(n int) []string {
	if l == nil {
		return []string{}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if n <= 0 || n > len(l.ring) {
		n = len(l.ring)
	}
	return append([]string{}, l.ring[len(l.ring)-n:]...)
}

// RecentLogs returns the newest n logged TikTok Shop calls (oldest first).
func (s *TTShopService) RecentLogs(n int) []string { return s.logs.recent(n) }

// safeQuery keeps only the query keys that are harmless to log (never app_key, sign, access_token, shop_cipher,
// app_secret, auth_code or refresh_token).
func safeQuery(q url.Values) string {
	var parts []string
	for _, k := range []string{"page_size", "page_token"} {
		if v := q.Get(k); v != "" {
			if k == "page_token" {
				v = "<set>"
			}
			parts = append(parts, k+"="+v)
		}
	}
	return strings.Join(parts, "&")
}

func requestBodyForLog(req *http.Request) string {
	if req.GetBody == nil {
		return ""
	}
	rc, err := req.GetBody()
	if err != nil {
		return ""
	}
	defer rc.Close()
	buf := make([]byte, ttsLogBodyMax+1)
	n, _ := rc.Read(buf)
	if n > ttsLogBodyMax {
		return string(buf[:ttsLogBodyMax]) + "…"
	}
	return string(buf[:n])
}
