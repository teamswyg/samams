package httpserver

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"server/internal/domain/shared"
)

// JSON helpers shared by all handlers.

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func WriteOK(w http.ResponseWriter, data any) {
	WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "data": data})
}

func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]any{
		"ok":    false,
		"error": map[string]string{"code": http.StatusText(status), "message": msg},
	})
}

// DecodeJSON reads JSON request body into dst.
func DecodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return errors.New("empty body")
	}
	return json.NewDecoder(r.Body).Decode(dst)
}

// MapDomainError translates domain errors to HTTP status codes.
func MapDomainError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	var ve shared.ValidationError
	var nf shared.NotFoundError
	var ce shared.ConflictError
	switch {
	case errors.As(err, &ve):
		WriteError(w, http.StatusBadRequest, ve.Error())
	case errors.As(err, &nf):
		WriteError(w, http.StatusNotFound, nf.Error())
	case errors.As(err, &ce):
		WriteError(w, http.StatusConflict, ce.Error())
	default:
		log.Printf("[http] Internal error: %v", err)
		WriteError(w, http.StatusInternalServerError, "internal server error")
	}
}
