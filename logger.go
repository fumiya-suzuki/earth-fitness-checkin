package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type contextKey string

const requestIDContextKey contextKey = "request_id"

const (
	logDir            = "logs"
	logRetentionDays  = 90
	requestIDHeader   = "X-Request-Id"
	logTimestampField = "timestamp"
)

var jst = time.FixedZone("Asia/Tokyo", 9*60*60)

type eventFields map[string]interface{}

type appLogger struct {
	mu         sync.Mutex
	currentDay string
	file       *os.File
	base       *log.Logger
}

func newAppLogger() *appLogger {
	return &appLogger{
		base: log.New(os.Stdout, "", 0),
	}
}

func (l *appLogger) ensureWriter(now time.Time) *os.File {
	day := now.In(jst).Format("2006-01-02")

	if l.file != nil && l.currentDay == day {
		return l.file
	}

	if err := os.MkdirAll(logDir, 0o755); err != nil {
		l.base.Printf(`{"level":"ERROR","event":"log_dir_create_failed","error":%q}`, err.Error())
		return nil
	}

	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}

	path := filepath.Join(logDir, "app-"+day+".log")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		l.base.Printf(`{"level":"ERROR","event":"log_file_open_failed","path":%q,"error":%q}`, path, err.Error())
		return nil
	}

	l.currentDay = day
	l.file = file
	return l.file
}

func (l *appLogger) write(level, event string, fields eventFields) {
	now := time.Now().In(jst)
	record := map[string]interface{}{
		logTimestampField: now.Format(time.RFC3339),
		"level":           level,
		"event":           event,
	}

	for k, v := range fields {
		record[k] = v
	}

	line, err := json.Marshal(record)
	if err != nil {
		l.base.Printf(`{"timestamp":%q,"level":"ERROR","event":"log_json_marshal_failed","source_event":%q,"error":%q}`, now.Format(time.RFC3339), event, err.Error())
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	file := l.ensureWriter(now)
	if _, err := os.Stdout.Write(append(line, '\n')); err != nil {
		l.base.Printf(`{"timestamp":%q,"level":"ERROR","event":"stdout_log_write_failed","source_event":%q,"error":%q}`, now.Format(time.RFC3339), event, err.Error())
	}
	if file != nil {
		if _, err := file.Write(append(line, '\n')); err != nil {
			l.base.Printf(`{"timestamp":%q,"level":"ERROR","event":"file_log_write_failed","source_event":%q,"error":%q}`, now.Format(time.RFC3339), event, err.Error())
		}
	}
}

func (l *appLogger) info(event string, fields eventFields) {
	l.write("INFO", event, fields)
}

func (l *appLogger) error(event string, fields eventFields) {
	l.write("ERROR", event, fields)
}

func (l *appLogger) cleanupOldFiles(now time.Time) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		l.error("log_dir_create_failed", eventFields{"error": err.Error()})
		return
	}

	entries, err := os.ReadDir(logDir)
	if err != nil {
		l.error("log_dir_read_failed", eventFields{"error": err.Error()})
		return
	}

	cutoff := now.In(jst).AddDate(0, 0, -logRetentionDays)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			l.error("log_file_stat_failed", eventFields{"file": entry.Name(), "error": err.Error()})
			continue
		}

		if info.ModTime().Before(cutoff) {
			path := filepath.Join(logDir, entry.Name())
			if err := os.Remove(path); err != nil {
				l.error("log_file_cleanup_failed", eventFields{"file": path, "error": err.Error()})
				continue
			}
			l.info("log_file_deleted", eventFields{"file": path})
		}
	}
}

var appLog = newAppLogger()

func newRequestID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "req-" + time.Now().Format("20060102150405.000000000")
	}
	return "req-" + hex.EncodeToString(buf)
}

func requestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDContextKey).(string)
	return value
}

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := newRequestID()
		ctx := context.WithValue(r.Context(), requestIDContextKey, requestID)
		w.Header().Set(requestIDHeader, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func eventFieldsFromRequest(r *http.Request) eventFields {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	return eventFields{
		"request_id": requestIDFromContext(r.Context()),
		"path":       r.URL.Path,
		"method":     r.Method,
		"remote_ip":  host,
		"user_agent": r.UserAgent(),
	}
}
