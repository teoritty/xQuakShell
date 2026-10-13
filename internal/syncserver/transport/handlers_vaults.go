package transport

import (
	"net/http"
	"strconv"

	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/usecase"
)

// vaultRequest authenticates an access token and parses the vault id; every vault endpoint starts
// here, so none can forget either step.
func (s *server) vaultRequest(w http.ResponseWriter, r *http.Request) (usecase.Principal, vaultsync.ID, bool) {
	p, ok := s.authenticate(w, r, usecase.TokenAccess)
	if !ok {
		return usecase.Principal{}, vaultsync.ID{}, false
	}
	vault, ok := pathID(w, r)
	return p, vault, ok
}

func (s *server) handleVaults(w http.ResponseWriter, r *http.Request) {
	p, ok := s.authenticate(w, r, usecase.TokenAccess)
	if !ok {
		return
	}
	vaults, err := s.svc.Vaults(r.Context(), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := syncwire.VaultList{Vaults: []syncwire.VaultSummary{}}
	for _, v := range vaults {
		out.Vaults = append(out.Vaults, syncwire.VaultSummary{Vault: v.ID.String(), Seq: v.Seq, MembershipSeq: v.MembershipSeq})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) handleVaultCreate(w http.ResponseWriter, r *http.Request) {
	p, ok := s.authenticate(w, r, usecase.TokenAccess)
	if !ok {
		return
	}
	var req syncwire.VaultCreate
	if !s.decode(w, r, s.cfg.MaxBody, &req) {
		return
	}
	op, err := req.Op.ToDomain()
	if err != nil {
		writeCode(w, http.StatusBadRequest, syncwire.CodeMalformed)
		return
	}
	if err := s.svc.CreateVault(r.Context(), p, op); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct{}{})
}

func (s *server) handleMembership(w http.ResponseWriter, r *http.Request) {
	p, vault, ok := s.vaultRequest(w, r)
	if !ok {
		return
	}
	ops, err := s.svc.Membership(r.Context(), p, vault, queryUint(r, "since"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := syncwire.Membership{Ops: []syncwire.MemberOp{}}
	for i := range ops {
		out.Ops = append(out.Ops, syncwire.MemberOpFrom(&ops[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) handleMembershipAppend(w http.ResponseWriter, r *http.Request) {
	p, vault, ok := s.vaultRequest(w, r)
	if !ok {
		return
	}
	var req syncwire.MembershipAppend
	if !s.decode(w, r, s.cfg.MaxBody, &req) {
		return
	}
	ops := make([]vaultsync.MemberOp, 0, len(req.Ops))
	for _, d := range req.Ops {
		op, err := d.ToDomain()
		if err != nil {
			writeCode(w, http.StatusBadRequest, syncwire.CodeMalformed)
			return
		}
		ops = append(ops, op)
	}
	if err := s.svc.AppendMembership(r.Context(), p, vault, ops); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct{}{})
}

func (s *server) handleCommit(w http.ResponseWriter, r *http.Request) {
	p, vault, ok := s.vaultRequest(w, r)
	if !ok {
		return
	}
	var req syncwire.Commit
	if !s.decode(w, r, s.cfg.MaxCommitBody, &req) {
		return
	}
	c, err := req.ToDomain()
	if err != nil {
		writeCode(w, http.StatusBadRequest, syncwire.CodeMalformed)
		return
	}
	seq, err := s.svc.Commit(r.Context(), p, vault, req.ExpectSeq, c)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, syncwire.CommitReply{Seq: seq})
}

func (s *server) handleChanges(w http.ResponseWriter, r *http.Request) {
	p, vault, ok := s.vaultRequest(w, r)
	if !ok {
		return
	}
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil {
		limit = 100
	}
	commits, more, err := s.svc.Changes(r.Context(), p, vault, queryUint(r, "since"), limit)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := syncwire.Changes{Commits: []syncwire.CommitEntry{}, More: more}
	for _, sc := range commits {
		out.Commits = append(out.Commits, syncwire.CommitEntryFrom(sc.Seq, &sc.Commit))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	p, vault, ok := s.vaultRequest(w, r)
	if !ok {
		return
	}
	head, records, err := s.svc.Snapshot(r.Context(), p, vault)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := syncwire.Snapshot{Records: envelopesFrom(records)}
	if head != nil {
		h := syncwire.HeadFrom(head)
		out.Head = &h
	}
	writeJSON(w, http.StatusOK, out)
}

func envelopesFrom(es []vaultsync.Envelope) []syncwire.Envelope {
	out := make([]syncwire.Envelope, 0, len(es))
	for i := range es {
		out = append(out, syncwire.EnvelopeFrom(&es[i]))
	}
	return out
}

// handleVersions returns one record's retained versions for the trash. A malformed record id is
// not found, like a malformed vault id.
func (s *server) handleVersions(w http.ResponseWriter, r *http.Request) {
	p, vault, ok := s.vaultRequest(w, r)
	if !ok {
		return
	}
	record, err := syncwire.ParseID(r.PathValue("rid"))
	if err != nil {
		writeCode(w, http.StatusNotFound, syncwire.CodeNotFound)
		return
	}
	versions, err := s.svc.Versions(r.Context(), p, vault, record)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, syncwire.Versions{Records: envelopesFrom(versions)})
}
