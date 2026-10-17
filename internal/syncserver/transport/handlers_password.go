package transport

import (
	"net/http"

	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/syncserver/usecase"
)

// The password routes accept either token: an account token right after a password login, or a
// device's access token for a reset from a device that is still signed in (spec §5.7). What actually
// authorises the change is the account key's signature, checked in the use case.

func (s *server) handlePasswordInit(w http.ResponseWriter, r *http.Request) {
	p, ok := s.authenticate(w, r, usecase.TokenAccess, usecase.TokenAccount)
	if !ok {
		return
	}
	var req syncwire.PasswordInit
	if !s.decode(w, r, s.cfg.MaxBody, &req) {
		return
	}
	resp, err := s.svc.PasswordStart(r.Context(), p, req.OPAQUE)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, syncwire.PasswordInitReply{OPAQUE: resp})
}

func (s *server) handlePasswordFinish(w http.ResponseWriter, r *http.Request) {
	p, ok := s.authenticate(w, r, usecase.TokenAccess, usecase.TokenAccount)
	if !ok {
		return
	}
	var req syncwire.PasswordFinish
	if !s.decode(w, r, s.cfg.MaxBody, &req) {
		return
	}
	if err := s.svc.PasswordFinish(r.Context(), p, req.Record, req.Bundle, req.Sig); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct{}{})
}
