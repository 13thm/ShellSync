package http

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/shellsync/daemon/internal/service"
)

// API error codes (design §6.8).
const (
	codeOK            = 0
	codeUnauthorized  = 40001
	codeForbidden     = 40003
	codeNotFound      = 40404
	codeValidation    = 40901
	codeConflict      = 40909
	codePairing       = 40910
	codePairingLocked = 40911
	codeInternal      = 50000
	codePTY           = 50001
)

// envelope is the unified response shape {code, data, message}.
type envelope struct {
	Code    int    `json:"code"`
	Data    any    `json:"data"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status, code int, data any, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Code: code, Data: data, Message: msg})
}

func ok(w http.ResponseWriter, data any) { writeJSON(w, http.StatusOK, codeOK, data, "ok") }

func created(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusCreated, codeOK, data, "created")
}

// fail writes a failure envelope mapping service/repository errors to codes.
func fail(w http.ResponseWriter, err error) {
	code, status, msg := errToCode(err)
	writeJSON(w, status, code, nil, msg)
}

func failCode(w http.ResponseWriter, code, status int, msg string) {
	writeJSON(w, status, code, nil, msg)
}

func errToCode(err error) (code, status int, msg string) {
	switch {
	case errors.Is(err, service.ErrInvalidTransition):
		return codeConflict, http.StatusConflict, err.Error()
	case errors.Is(err, service.ErrInvalidPairing):
		return codePairing, http.StatusBadRequest, err.Error()
	case errors.Is(err, service.ErrPairingLocked):
		return codePairingLocked, http.StatusTooManyRequests, err.Error()
	case errors.Is(err, sql.ErrNoRows):
		return codeNotFound, http.StatusNotFound, "resource not found"
	default:
		return codeInternal, http.StatusInternalServerError, err.Error()
	}
}
