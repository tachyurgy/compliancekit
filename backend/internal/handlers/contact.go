package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type ContactSubmission struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Company string `json:"company"`
	State   string `json:"state"`
	Topic   string `json:"topic"`
	Message string `json:"message"`
}

type ContactHandler struct {
	Pool *sql.DB
	Log  *slog.Logger
}

// Post records a public contact-form submission. Public, no auth.
func (h *ContactHandler) Post(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		http.Error(w, "missing body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)

	var s ContactSubmission
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	s.Name = strings.TrimSpace(s.Name)
	s.Email = strings.TrimSpace(strings.ToLower(s.Email))
	s.Message = strings.TrimSpace(s.Message)

	if s.Name == "" || s.Email == "" || s.Message == "" {
		http.Error(w, "name, email, and message required", http.StatusBadRequest)
		return
	}
	if !strings.Contains(s.Email, "@") || len(s.Email) > 254 {
		http.Error(w, "invalid email", http.StatusBadRequest)
		return
	}
	if len(s.Message) > 4000 || len(s.Name) > 200 {
		http.Error(w, "field too long", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	_, err := h.Pool.ExecContext(ctx, `
		INSERT INTO contact_submissions (
			id, name, email, company, state, topic, message, source_ip, user_agent, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		genID(),
		s.Name, s.Email, s.Company, s.State, s.Topic, s.Message,
		clientIP(r),
		truncate(r.UserAgent(), 500),
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		h.Log.Error("contact insert failed", "err", err)
		http.Error(w, "could not save", http.StatusInternalServerError)
		return
	}
	h.Log.Info("contact submission",
		"email", s.Email, "topic", s.Topic, "state", s.State, "company", s.Company)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func clientIP(r *http.Request) string {
	if v := r.Header.Get("CF-Connecting-IP"); v != "" {
		return v
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		return strings.TrimSpace(strings.Split(v, ",")[0])
	}
	return r.RemoteAddr
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func genID() string {
	return time.Now().UTC().Format("20060102T150405.000000000")
}
