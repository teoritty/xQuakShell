package transport

import (
	"net/http"

	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/usecase"
)

func (s *server) handleRegisterInit(w http.ResponseWriter, r *http.Request) {
	var req syncwire.RegisterInit
	if !s.decode(w, r, s.cfg.MaxBody, &req) {
		return
	}
	account, err := syncwire.ParseID(req.Account)
	if err != nil {
		writeCode(w, http.StatusBadRequest, syncwire.CodeMalformed)
		return
	}
	resp, err := s.svc.RegisterStart(r.Context(), req.Invite, req.Username, account, req.OPAQUE)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, syncwire.RegisterInitReply{OPAQUE: resp})
}

func (s *server) handleRegisterFinish(w http.ResponseWriter, r *http.Request) {
	var req syncwire.RegisterFinish
	if !s.decode(w, r, s.cfg.MaxBody, &req) {
		return
	}
	reg, err := registrationFrom(req)
	if err != nil {
		writeCode(w, http.StatusBadRequest, syncwire.CodeMalformed)
		return
	}
	if err := s.svc.RegisterFinish(r.Context(), reg); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct{}{})
}

func registrationFrom(req syncwire.RegisterFinish) (usecase.Registration, error) {
	account, err := syncwire.ParseID(req.Account)
	if err != nil {
		return usecase.Registration{}, err
	}
	device, err := req.Device.ToDomain()
	if err != nil {
		return usecase.Registration{}, err
	}
	vault, err := req.Vault.ToDomain()
	if err != nil {
		return usecase.Registration{}, err
	}
	return usecase.Registration{Invite: req.Invite, Username: req.Username, Account: account, Record: req.Record,
		IdentityKey: req.IdentityKey, EncryptionKey: req.EncryptionKey, Bundle: req.Bundle, Device: device, Vault: vault}, nil
}

func (s *server) handleLoginInit(w http.ResponseWriter, r *http.Request) {
	var req syncwire.LoginInit
	if !s.decode(w, r, s.cfg.MaxBody, &req) {
		return
	}
	id, account, ke2, err := s.svc.LoginStart(r.Context(), req.Username, clientAddress(r), req.KE1)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, syncwire.LoginInitReply{LoginID: id, Account: account.String(), KE2: ke2})
}

func (s *server) handleLoginFinish(w http.ResponseWriter, r *http.Request) {
	var req syncwire.LoginFinish
	if !s.decode(w, r, s.cfg.MaxBody, &req) {
		return
	}
	res, err := s.svc.LoginFinish(r.Context(), req.LoginID, clientAddress(r), req.KE3)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, syncwire.LoginFinishReply{AccountToken: res.AccountToken, Bundle: res.Bundle,
		IdentityKey: res.IdentityKey, EncryptionKey: res.EncryptionKey})
}

func parseAccountDevice(account, device string) (vaultsync.ID, vaultsync.ID, bool) {
	a, err1 := syncwire.ParseID(account)
	d, err2 := syncwire.ParseID(device)
	return a, d, err1 == nil && err2 == nil
}

func (s *server) handleDeviceChallenge(w http.ResponseWriter, r *http.Request) {
	var req syncwire.DeviceChallenge
	if !s.decode(w, r, s.cfg.MaxBody, &req) {
		return
	}
	account, device, ok := parseAccountDevice(req.Account, req.Device)
	if !ok {
		writeCode(w, http.StatusBadRequest, syncwire.CodeMalformed)
		return
	}
	nonce, err := s.svc.DeviceChallenge(r.Context(), account, device)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, syncwire.DeviceChallengeReply{Nonce: nonce})
}

func (s *server) handleDeviceVerify(w http.ResponseWriter, r *http.Request) {
	var req syncwire.DeviceVerify
	if !s.decode(w, r, s.cfg.MaxBody, &req) {
		return
	}
	account, device, ok := parseAccountDevice(req.Account, req.Device)
	if !ok {
		writeCode(w, http.StatusBadRequest, syncwire.CodeMalformed)
		return
	}
	token, expires, err := s.svc.DeviceVerify(r.Context(), account, device, req.Nonce, req.Sig)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, syncwire.AccessReply{AccessToken: token, ExpiresUnixMilli: expires})
}

func (s *server) handleLookup(w http.ResponseWriter, r *http.Request) {
	p, ok := s.authenticate(w, r, usecase.TokenAccess)
	if !ok {
		return
	}
	res, err := s.svc.Lookup(r.Context(), p, r.URL.Query().Get("username"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, syncwire.Lookup{Account: res.Account.String(), IdentityKey: res.IdentityKey,
		EncryptionKey: res.EncryptionKey, DeviceLogSeq: res.DeviceLogSeq})
}

func (s *server) handleDeviceLog(w http.ResponseWriter, r *http.Request) {
	p, ok := s.authenticate(w, r, usecase.TokenAccess, usecase.TokenAccount)
	if !ok {
		return
	}
	account, ok := pathID(w, r)
	if !ok {
		return
	}
	entries, err := s.svc.DeviceLog(r.Context(), p, account, queryUint(r, "since"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	keys, err := s.svc.AccountKeys(r.Context(), p, account)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := syncwire.DeviceLog{Entries: []syncwire.DeviceEntry{}, Suite: uint16(keys.Suite), IdentityKey: keys.IdentityKey,
		EncryptionKey: keys.EncryptionKey}
	for i := range entries {
		out.Entries = append(out.Entries, syncwire.DeviceEntryFrom(&entries[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleDeviceLogAppend accepts an account token as well as an access token: a new device appends
// its own certificate right after the OPAQUE login, before it has an access token to show.
func (s *server) handleDeviceLogAppend(w http.ResponseWriter, r *http.Request) {
	p, ok := s.authenticate(w, r, usecase.TokenAccess, usecase.TokenAccount)
	if !ok {
		return
	}
	var req syncwire.DeviceLogAppend
	if !s.decode(w, r, s.cfg.MaxBody, &req) {
		return
	}
	entries := make([]vaultsync.DeviceEntry, 0, len(req.Entries))
	for _, d := range req.Entries {
		e, err := d.ToDomain()
		if err != nil {
			writeCode(w, http.StatusBadRequest, syncwire.CodeMalformed)
			return
		}
		entries = append(entries, e)
	}
	if err := s.svc.AppendDeviceLog(r.Context(), p, entries); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct{}{})
}
