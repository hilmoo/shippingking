// Package api implements the RajaOngkir-compatible HTTP handlers.
package api

import (
	"encoding/json"
	"net/http"
)

type meta struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
	Status  string `json:"status"`
}

type envelope struct {
	Meta meta `json:"meta"`
	Data any  `json:"data"`
}

func writeJSON(w http.ResponseWriter, code int, message string, data any) {
	status := "success"
	if code < 200 || code > 299 {
		status = "error"
		data = nil
	}
	b, err := json.Marshal(envelope{Meta: meta{Message: message, Code: code, Status: status}, Data: data})
	if err != nil {
		code = http.StatusInternalServerError
		b = []byte(`{"meta":{"message":"Internal Server Error","code":500,"status":"error"},"data":null}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(append(b, '\n'))
}

func ok(w http.ResponseWriter, message string, data any) {
	writeJSON(w, http.StatusOK, message, data)
}

func fail(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, message, nil)
}
