package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

type APIError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
	// Wrapped, internal error; not serialized.
	Err error `json:"-"`
}

func (e *APIError) Error() string {
	if e.Err != nil {
		return e.Code + ": " + e.Message + ": " + e.Err.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *APIError) Unwrap() error { return e.Err }

func newErr(status int, code, message string) *APIError {
	return &APIError{Status: status, Code: code, Message: message}
}

// Sentinel error constructors. Wrap upstream errors via .Wrap().
var (
	ErrBadRequest   = newErr(http.StatusBadRequest, "bad_request", "invalid request")
	ErrUnauthorized = newErr(http.StatusUnauthorized, "unauthorized", "authentication required")
	ErrForbidden    = newErr(http.StatusForbidden, "forbidden", "access denied")
	ErrNotFound     = newErr(http.StatusNotFound, "not_found", "resource not found")
	ErrConflict     = newErr(http.StatusConflict, "conflict", "resource conflict")
	ErrInternal     = newErr(http.StatusInternalServerError, "internal", "internal server error")
)

// BadRequestf / NotFoundf / etc. produce a fresh APIError with a custom message but same code/status.
func BadRequestf(format string, args ...any) *APIError {
	return &APIError{Status: http.StatusBadRequest, Code: "bad_request", Message: sprintf(format, args...)}
}
func NotFoundf(format string, args ...any) *APIError {
	return &APIError{Status: http.StatusNotFound, Code: "not_found", Message: sprintf(format, args...)}
}
func ConflictF(format string, args ...any) *APIError {
	return &APIError{Status: http.StatusConflict, Code: "conflict", Message: sprintf(format, args...)}
}
func Wrap(base *APIError, err error) *APIError {
	copy := *base
	copy.Err = err
	return &copy
}

func sprintf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}

// RenderJSON writes v as JSON with the given status.
func RenderJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// RenderError writes an error response. Unwraps APIError; falls back to 500 for unknown errors.
func RenderError(w http.ResponseWriter, r *http.Request, err error) {
	reqID := middleware.GetReqID(r.Context())
	var api *APIError
	if errors.As(err, &api) {
		payload := map[string]any{
			"error": map[string]any{
				"code":       api.Code,
				"message":    api.Message,
				"request_id": reqID,
			},
		}
		if api.Status >= 500 {
			slog.ErrorContext(r.Context(), "api error", "code", api.Code, "err", err, "request_id", reqID)
		} else {
			slog.DebugContext(r.Context(), "api error", "code", api.Code, "err", err, "request_id", reqID)
		}
		RenderJSON(w, api.Status, payload)
		return
	}
	slog.ErrorContext(r.Context(), "unhandled error", "err", err, "request_id", reqID)
	RenderJSON(w, http.StatusInternalServerError, map[string]any{
		"error": map[string]any{
			"code":       "internal",
			"message":    "internal server error",
			"request_id": reqID,
		},
	})
}

// DecodeJSON decodes a request body into v. Limits size to 5 MiB.
func DecodeJSON(r *http.Request, v any) error {
	const maxBody = 5 << 20
	r.Body = http.MaxBytesReader(nil, r.Body, maxBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return Wrap(ErrBadRequest, err)
	}
	return nil
}
