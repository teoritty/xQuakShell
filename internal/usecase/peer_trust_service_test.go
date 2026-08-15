package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/usecase"
)

type fakeTrustRepo struct {
	entries map[string]domain.PeerTrustEntry
	order   []string
	puts    int
	removes int
}

func newFakeTrustRepo() *fakeTrustRepo {
	return &fakeTrustRepo{entries: map[string]domain.PeerTrustEntry{}}
}

func (r *fakeTrustRepo) key(scope, subject string) string { return scope + "\x00" + subject }

func (r *fakeTrustRepo) Find(scope, subject string) (*domain.PeerTrustEntry, error) {
	e, ok := r.entries[r.key(scope, subject)]
	if !ok {
		return nil, nil
	}
	return &e, nil
}

func (r *fakeTrustRepo) Put(_ context.Context, e domain.PeerTrustEntry) error {
	r.puts++
	k := r.key(e.Scope, e.Subject)
	if _, seen := r.entries[k]; !seen {
		r.order = append(r.order, k)
	}
	r.entries[k] = e
	return nil
}

func (r *fakeTrustRepo) List() ([]domain.PeerTrustEntry, error) {
	out := make([]domain.PeerTrustEntry, 0, len(r.order))
	for _, k := range r.order {
		if e, ok := r.entries[k]; ok {
			out = append(out, e)
		}
	}
	return out, nil
}

func (r *fakeTrustRepo) Remove(_ context.Context, scope, subject string) error {
	r.removes++
	delete(r.entries, r.key(scope, subject))
	return nil
}

// fakeTrustSessions stands in for the session lifecycle. It records what the service asked for; the
// rules the real implementation enforces (set-if-absent, admissible states) are covered where they
// live, in peer_trust_lifecycle_test.go.
type fakeTrustSessions struct {
	conn     domain.Connection
	connErr  error
	prompt   *usecase.PeerTrustPrompt
	beginErr error
	retries  int
	rejects  int
}

func (s *fakeTrustSessions) ConnectionRecordForSession(context.Context, string) (domain.Connection, error) {
	return s.conn, s.connErr
}

func (s *fakeTrustSessions) BeginPeerTrustPrompt(_ string, p *usecase.PeerTrustPrompt) error {
	if s.beginErr != nil {
		return s.beginErr
	}
	s.prompt = p
	return nil
}

func (s *fakeTrustSessions) GetPeerTrustPrompt(string) (*usecase.PeerTrustPrompt, error) {
	return s.prompt, nil
}

func (s *fakeTrustSessions) RejectPeerTrust(string) error {
	s.rejects++
	s.prompt = nil
	return nil
}

func (s *fakeTrustSessions) RetrySession(context.Context, string) error {
	s.retries++
	s.prompt = nil
	return nil
}

// fakeProtocolLookup stands in for the plugin protocol registry: the same source of a default port
// the core uses for session.connect.
type fakeProtocolLookup map[string]int

func (l fakeProtocolLookup) DefaultPortForProtocol(protocol string) (int, bool) {
	port, ok := l[protocol]
	return port, ok
}

func newTrustService(repo domain.PeerTrustRepository, sessions *fakeTrustSessions) *usecase.PeerTrustService {
	return usecase.NewPeerTrustService(repo, sessions, fakeProtocolLookup{"rdp": 3389})
}

func rdpConnection() domain.Connection {
	return domain.Connection{ID: "c1", Name: "prod", Host: "10.0.0.5", Port: 3389, Protocol: "rdp"}
}

// promptFingerprint answers the question the way the UI does: with the fingerprint it was shown.
func promptFingerprint(t *testing.T, sessions *fakeTrustSessions) string {
	t.Helper()
	if sessions.prompt == nil {
		t.Fatal("no pending prompt to answer")
	}
	return sessions.prompt.Fingerprint
}

// The headline property of this package: a subject that does not match the connection record is
// refused BEFORE the storage is touched and BEFORE anything is shown to a human.
func TestVerifyRejectsSubjectThatIsNotTheConnection(t *testing.T) {
	repo := newFakeTrustRepo()
	sessions := &fakeTrustSessions{conn: rdpConnection()}
	svc := newTrustService(repo, sessions)

	_, err := svc.Verify(context.Background(), "plug", "s1", "evil.example.com:3389", []byte{1})
	if err == nil {
		t.Fatal("a foreign subject was accepted")
	}
	if sessions.prompt != nil {
		t.Fatal("a dialog was raised for a foreign subject; the user would have seen the wrong host")
	}
	if repo.puts != 0 {
		t.Fatal("something was written to storage")
	}
}

// The port on a connection record may be zero: the core substitutes the manifest's defaultPort.
// Comparing against the raw value would reject a correct call from anyone who never typed a port.
func TestVerifyUsesEffectivePortWhenConnectionPortIsZero(t *testing.T) {
	conn := rdpConnection()
	conn.Port = 0
	sessions := &fakeTrustSessions{conn: conn}
	svc := newTrustService(newFakeTrustRepo(), sessions)

	trusted, err := svc.Verify(context.Background(), "plug", "s1", "10.0.0.5:3389", []byte{1})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if trusted {
		t.Fatal("an unknown subject was reported as trusted")
	}
	if sessions.prompt == nil {
		t.Fatal("no dialog was raised")
	}
}

func TestVerifyUnknownSubjectCreatesPrompt(t *testing.T) {
	sessions := &fakeTrustSessions{conn: rdpConnection()}
	svc := newTrustService(newFakeTrustRepo(), sessions)

	trusted, err := svc.Verify(context.Background(), "plug", "s1", "10.0.0.5:3389", []byte{9, 9})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if trusted {
		t.Fatal("an unknown subject was trusted")
	}
	if sessions.prompt == nil {
		t.Fatal("no dialog was raised")
	}
	if sessions.prompt.Mismatch {
		t.Fatal("a first connection was labelled as a key change")
	}
	if sessions.prompt.Fingerprint != domain.PeerFingerprint([]byte{9, 9}) {
		t.Fatalf("fingerprint %q was not derived from the material", sessions.prompt.Fingerprint)
	}
	// The subject in the dialog is the core's version, not the string the plugin sent.
	if sessions.prompt.Subject != "10.0.0.5:3389" {
		t.Fatalf("subject in the dialog = %q", sessions.prompt.Subject)
	}
}

func TestVerifyKnownMatchingMaterialIsTrusted(t *testing.T) {
	repo := newFakeTrustRepo()
	if err := repo.Put(context.Background(), domain.PeerTrustEntry{
		Scope: "plug", Subject: "10.0.0.5:3389", Material: []byte{7},
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	sessions := &fakeTrustSessions{conn: rdpConnection()}
	svc := newTrustService(repo, sessions)

	trusted, err := svc.Verify(context.Background(), "plug", "s1", "10.0.0.5:3389", []byte{7})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !trusted {
		t.Fatal("matching material was not reported as trusted")
	}
	if sessions.prompt != nil {
		t.Fatal("a needless dialog: the user was asked about something already confirmed")
	}
}

// A changed material must read differently from a first connection: the user needs different
// words, because the action is different.
func TestVerifyChangedMaterialMarksMismatch(t *testing.T) {
	repo := newFakeTrustRepo()
	if err := repo.Put(context.Background(), domain.PeerTrustEntry{
		Scope: "plug", Subject: "10.0.0.5:3389", Material: []byte{7},
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	sessions := &fakeTrustSessions{conn: rdpConnection()}
	svc := newTrustService(repo, sessions)

	trusted, err := svc.Verify(context.Background(), "plug", "s1", "10.0.0.5:3389", []byte{8})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if trusted {
		t.Fatal("changed material was reported as trusted")
	}
	if sessions.prompt == nil || !sessions.prompt.Mismatch {
		t.Fatal("the change of material was not flagged")
	}
}

func TestVerifyRejectsEmptyAndOversizedMaterial(t *testing.T) {
	sessions := &fakeTrustSessions{conn: rdpConnection()}
	svc := newTrustService(newFakeTrustRepo(), sessions)

	if _, err := svc.Verify(context.Background(), "plug", "s1", "10.0.0.5:3389", nil); !errors.Is(err, domain.ErrPeerMaterialEmpty) {
		t.Fatalf("empty material: %v", err)
	}
	big := make([]byte, domain.MaxPeerTrustMaterial+1)
	if _, err := svc.Verify(context.Background(), "plug", "s1", "10.0.0.5:3389", big); !errors.Is(err, domain.ErrPeerMaterialTooLarge) {
		t.Fatalf("material over the ceiling: %v", err)
	}
	if sessions.prompt != nil {
		t.Fatal("a dialog was raised for material that was not accepted")
	}
}

// A refusal from the session lifecycle must reach the plugin. Swallowing it would turn "this
// session is already asking about something else" into a plain "not trusted", and the plugin would
// retry into the same wall forever.
func TestVerifySurfacesARefusedPrompt(t *testing.T) {
	sessions := &fakeTrustSessions{conn: rdpConnection(), beginErr: domain.ErrPeerTrustPromptBusy}
	svc := newTrustService(newFakeTrustRepo(), sessions)

	notified := 0
	svc.SetPromptNotifier(func(string, string, string, bool) { notified++ })

	trusted, err := svc.Verify(context.Background(), "plug", "s1", "10.0.0.5:3389", []byte{1})
	if !errors.Is(err, domain.ErrPeerTrustPromptBusy) {
		t.Fatalf("err = %v, want ErrPeerTrustPromptBusy", err)
	}
	if trusted {
		t.Fatal("a refused question reported the peer as trusted")
	}
	if notified != 0 {
		t.Fatal("the UI was told about a question that was never raised")
	}
}

// The decision comes from the pending question rather than from the arguments - the ResolveHostKey
// lesson: otherwise trust could be written outside a check the core itself started.
func TestResolveWritesWhatIsPendingAndRetries(t *testing.T) {
	repo := newFakeTrustRepo()
	sessions := &fakeTrustSessions{conn: rdpConnection()}
	svc := newTrustService(repo, sessions)

	if _, err := svc.Verify(context.Background(), "plug", "s1", "10.0.0.5:3389", []byte{4, 2}); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if err := svc.Resolve(context.Background(), "plug", "s1", "trust", promptFingerprint(t, sessions)); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	got, err := repo.Find("plug", "10.0.0.5:3389")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got == nil || string(got.Material) != string([]byte{4, 2}) {
		t.Fatalf("what was stored is not what the dialog showed: %+v", got)
	}
	if sessions.retries != 1 {
		t.Fatalf("session retries = %d, want 1", sessions.retries)
	}
}

// The user answers about the question they were shown. An answer naming another fingerprint is
// refused - the second lock on the consent door, and the one that compares the very string the
// user was looking at.
func TestResolveRefusesAnAnswerAboutAnotherFingerprint(t *testing.T) {
	repo := newFakeTrustRepo()
	sessions := &fakeTrustSessions{conn: rdpConnection()}
	svc := newTrustService(repo, sessions)

	if _, err := svc.Verify(context.Background(), "plug", "s1", "10.0.0.5:3389", []byte{4, 2}); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	err := svc.Resolve(context.Background(), "plug", "s1", "trust", domain.PeerFingerprint([]byte{9, 9}))
	if !errors.Is(err, domain.ErrPeerTrustPromptStale) {
		t.Fatalf("err = %v, want ErrPeerTrustPromptStale", err)
	}
	if repo.puts != 0 {
		t.Fatal("an answer about another question wrote trust")
	}
	if sessions.prompt == nil {
		t.Fatal("the pending question was dropped; the user can no longer answer the real one")
	}
}

// An empty fingerprint is not "no opinion", it is an answer that names nothing. Accepting it would
// leave the check in place while every caller could opt out of it.
func TestResolveRefusesAnEmptyFingerprint(t *testing.T) {
	repo := newFakeTrustRepo()
	sessions := &fakeTrustSessions{conn: rdpConnection()}
	svc := newTrustService(repo, sessions)

	if _, err := svc.Verify(context.Background(), "plug", "s1", "10.0.0.5:3389", []byte{1}); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if err := svc.Resolve(context.Background(), "plug", "s1", "trust", ""); !errors.Is(err, domain.ErrPeerTrustPromptStale) {
		t.Fatalf("err = %v, want ErrPeerTrustPromptStale", err)
	}
	if repo.puts != 0 {
		t.Fatal("an answer with no fingerprint wrote trust")
	}
}

func TestResolveWithoutPendingIsRefused(t *testing.T) {
	repo := newFakeTrustRepo()
	sessions := &fakeTrustSessions{conn: rdpConnection()}
	svc := newTrustService(repo, sessions)

	err := svc.Resolve(context.Background(), "plug", "s1", "trust", domain.PeerFingerprint([]byte{1}))
	if !errors.Is(err, domain.ErrPeerTrustNoPending) {
		t.Fatalf("err = %v, want ErrPeerTrustNoPending", err)
	}
	if repo.puts != 0 {
		t.Fatal("something was written with no dialog behind it")
	}
}

func TestResolveRejectsUnknownAction(t *testing.T) {
	repo := newFakeTrustRepo()
	sessions := &fakeTrustSessions{conn: rdpConnection()}
	svc := newTrustService(repo, sessions)

	if _, err := svc.Verify(context.Background(), "plug", "s1", "10.0.0.5:3389", []byte{1}); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if err := svc.Resolve(context.Background(), "plug", "s1", "maybe", promptFingerprint(t, sessions)); err == nil {
		t.Fatal("an unknown action was accepted")
	}
	if repo.puts != 0 {
		t.Fatal("an unknown action wrote something")
	}
	if sessions.prompt == nil {
		t.Fatal("the pending question was dropped on an unknown action; the decision is lost")
	}
}

// A refusal writes nothing, retries nothing, and fails the session instead of leaving it waiting.
func TestResolveRejectFailsTheSessionWithoutWriting(t *testing.T) {
	repo := newFakeTrustRepo()
	sessions := &fakeTrustSessions{conn: rdpConnection()}
	svc := newTrustService(repo, sessions)

	if _, err := svc.Verify(context.Background(), "plug", "s1", "10.0.0.5:3389", []byte{1}); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if err := svc.Resolve(context.Background(), "plug", "s1", "reject", promptFingerprint(t, sessions)); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if repo.puts != 0 {
		t.Fatal("a refusal wrote something")
	}
	if sessions.retries != 0 {
		t.Fatal("a refusal retried the session")
	}
	if sessions.rejects != 1 {
		t.Fatalf("session rejections = %d, want 1; a refusal must fail the session, not just close the dialog", sessions.rejects)
	}
}

// The scope is supplied by the caller from the session binding. The service must use that one both
// when searching and when writing - otherwise a plugin would land in someone else's.
func TestResolveWritesIntoTheCallersScope(t *testing.T) {
	repo := newFakeTrustRepo()
	sessions := &fakeTrustSessions{conn: rdpConnection()}
	svc := newTrustService(repo, sessions)

	if _, err := svc.Verify(context.Background(), "plug-a", "s1", "10.0.0.5:3389", []byte{1}); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if err := svc.Resolve(context.Background(), "plug-a", "s1", "trust", promptFingerprint(t, sessions)); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got, _ := repo.Find("plug-b", "10.0.0.5:3389"); got != nil {
		t.Fatal("the entry landed in a foreign scope")
	}
	if got, _ := repo.Find("plug-a", "10.0.0.5:3389"); got == nil {
		t.Fatal("the entry did not land in its own scope")
	}
}

// A session with no usable connection is a refusal, not trust by default.
func TestVerifyRefusesSessionWithoutUsableConnection(t *testing.T) {
	cases := map[string]domain.Connection{
		"no host":   {ID: "c", Port: 3389, Protocol: "rdp"},
		"wild port": {ID: "c", Host: "h", Port: 70000, Protocol: "rdp"},
	}
	for name, conn := range cases {
		t.Run(name, func(t *testing.T) {
			sessions := &fakeTrustSessions{conn: conn}
			svc := usecase.NewPeerTrustService(newFakeTrustRepo(), sessions, fakeProtocolLookup{})
			if _, err := svc.Verify(context.Background(), "plug", "s1", "h:3389", []byte{1}); err == nil {
				t.Fatal("an unusable connection was accepted")
			}
		})
	}
}

// A failure to read the connection must not turn into "the subject did not match": the causes are
// different, and so are the cures.
func TestVerifySurfacesConnectionLookupFailure(t *testing.T) {
	sessions := &fakeTrustSessions{connErr: domain.ErrSessionNotFound}
	svc := newTrustService(newFakeTrustRepo(), sessions)

	_, err := svc.Verify(context.Background(), "plug", "s1", "h:3389", []byte{1})
	if !errors.Is(err, domain.ErrSessionNotFound) {
		t.Fatalf("err = %v, want ErrSessionNotFound", err)
	}
}

// The port on a connection record may be blank - the core then substitutes the protocol default
// and sends the plugin exactly there. A trust subject must name that same address: otherwise trust
// is written against "host:0" and checked against "host:3389", and never matches.
func TestVerifyUsesTheSamePortTheCoreConnectsTo(t *testing.T) {
	sessions := &fakeTrustSessions{
		conn: domain.Connection{ID: "c1", Host: "10.0.0.5", Protocol: "rdp"},
	}
	svc := newTrustService(newFakeTrustRepo(), sessions)

	if _, err := svc.Verify(context.Background(), "plug", "s1", "10.0.0.5:3389", []byte{1}); err != nil {
		t.Fatalf("a subject on the protocol's default port was rejected: %v", err)
	}
	if sessions.prompt == nil {
		t.Fatal("no question was put to the user")
	}
	if sessions.prompt.Subject != "10.0.0.5:3389" {
		t.Fatalf("subject = %q, want 10.0.0.5:3389", sessions.prompt.Subject)
	}
}

// REGRESSION: the SSH default lives in the domain, not in the plugin registry. A private port
// table here would silently return zero and reject a correct call.
func TestVerifyKnowsTheDomainDefaultForSSH(t *testing.T) {
	sessions := &fakeTrustSessions{
		conn: domain.Connection{ID: "c1", Host: "h", Protocol: domain.ProtocolSSH},
	}
	svc := newTrustService(newFakeTrustRepo(), sessions)

	if _, err := svc.Verify(context.Background(), "plug", "s1", "h:22", []byte{1}); err != nil {
		t.Fatalf("a subject on the SSH default port was rejected: %v", err)
	}
}

// The notification is the only thing that raises the dialog. Without it a session goes into
// trust-required silently, and the user watches a connection waiting on an answer nobody asked for.
func TestVerifyNotifiesWhenAQuestionAppears(t *testing.T) {
	sessions := &fakeTrustSessions{conn: rdpConnection()}
	svc := newTrustService(newFakeTrustRepo(), sessions)

	var got struct {
		called                          int
		sessionID, subject, fingerprint string
		mismatch                        bool
	}
	svc.SetPromptNotifier(func(sessionID, subject, fingerprint string, mismatch bool) {
		got.called++
		got.sessionID, got.subject, got.fingerprint, got.mismatch = sessionID, subject, fingerprint, mismatch
	})

	if _, err := svc.Verify(context.Background(), "plug", "s1", "10.0.0.5:3389", []byte{1, 2, 3}); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.called != 1 {
		t.Fatalf("notifications = %d, want 1", got.called)
	}
	if got.sessionID != "s1" || got.subject != "10.0.0.5:3389" {
		t.Fatalf("notified about %q / %q", got.sessionID, got.subject)
	}
	if got.fingerprint != domain.PeerFingerprint([]byte{1, 2, 3}) {
		t.Fatalf("fingerprint = %q", got.fingerprint)
	}
	if got.mismatch {
		t.Fatal("a first meeting was described as a change of material")
	}
}

// A known peer passes silently: a dialog on every connection would teach the user to click
// "trust" without reading.
func TestVerifyDoesNotNotifyWhenAlreadyTrusted(t *testing.T) {
	repo := newFakeTrustRepo()
	material := []byte{9, 9, 9}
	if err := repo.Put(context.Background(), domain.PeerTrustEntry{
		Scope: "plug", Subject: "10.0.0.5:3389", Material: material,
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	sessions := &fakeTrustSessions{conn: rdpConnection()}
	svc := newTrustService(repo, sessions)

	notified := 0
	svc.SetPromptNotifier(func(string, string, string, bool) { notified++ })

	trusted, err := svc.Verify(context.Background(), "plug", "s1", "10.0.0.5:3389", material)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !trusted {
		t.Fatal("a known peer was not reported as trusted")
	}
	if notified != 0 {
		t.Fatal("a question was raised about a peer that is already trusted")
	}
}

// --- management: listing and revoking ---

// Without a way to revoke, a wrongly confirmed identity would be permanent.
func TestRevokeRemovesTheEntryAndIsAudited(t *testing.T) {
	repo := newFakeTrustRepo()
	if err := repo.Put(context.Background(), domain.PeerTrustEntry{
		Scope: "plug", Subject: "10.0.0.5:3389", Material: []byte{5},
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	svc := newTrustService(repo, &fakeTrustSessions{conn: rdpConnection()})
	var seen []usecase.PeerTrustAuditEntry
	svc.SetAuditRecorder(func(e usecase.PeerTrustAuditEntry) { seen = append(seen, e) })

	if err := svc.Revoke(context.Background(), "plug", "10.0.0.5:3389"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if got, _ := repo.Find("plug", "10.0.0.5:3389"); got != nil {
		t.Fatal("the entry survived the revocation")
	}
	if len(seen) != 1 || seen[0].Action != "trust.revoke" || !seen[0].Allowed {
		t.Fatalf("audit = %+v, want one allowed trust.revoke", seen)
	}
}

// A half-named coordinate must not reach the repository: with an empty scope the pair matches
// nothing, and a silent no-op reads to the user as a deletion that happened.
func TestRevokeRefusesAHalfNamedEntry(t *testing.T) {
	repo := newFakeTrustRepo()
	svc := newTrustService(repo, &fakeTrustSessions{conn: rdpConnection()})

	if err := svc.Revoke(context.Background(), "", "10.0.0.5:3389"); err == nil {
		t.Fatal("a revocation without a scope was accepted")
	}
	if err := svc.Revoke(context.Background(), "plug", ""); err == nil {
		t.Fatal("a revocation without a subject was accepted")
	}
	if repo.removes != 0 {
		t.Fatal("the repository was asked to remove a pair that names nothing")
	}
}

func TestListTrustedPeersReturnsWhatWasStored(t *testing.T) {
	repo := newFakeTrustRepo()
	for _, subject := range []string{"a:1", "b:2"} {
		if err := repo.Put(context.Background(), domain.PeerTrustEntry{
			Scope: "plug", Subject: subject, Material: []byte{1},
		}); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}
	svc := newTrustService(repo, &fakeTrustSessions{conn: rdpConnection()})

	entries, err := svc.ListTrustedPeers()
	if err != nil {
		t.Fatalf("ListTrustedPeers: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
}

// --- audit ---

// A decision is the security event this mechanism exists to produce. The plugin's RPC audit line
// records that a plugin asked; only this records what the human answered.
func TestAuditRecordsTheQuestionAndTheDecision(t *testing.T) {
	repo := newFakeTrustRepo()
	sessions := &fakeTrustSessions{conn: rdpConnection()}
	svc := newTrustService(repo, sessions)
	var seen []usecase.PeerTrustAuditEntry
	svc.SetAuditRecorder(func(e usecase.PeerTrustAuditEntry) { seen = append(seen, e) })

	material := []byte{1, 2, 3}
	if _, err := svc.Verify(context.Background(), "plug", "s1", "10.0.0.5:3389", material); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if err := svc.Resolve(context.Background(), "plug", "s1", "trust", domain.PeerFingerprint(material)); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(seen) != 2 {
		t.Fatalf("audit entries = %d, want 2 (question, decision)", len(seen))
	}
	if seen[0].Action != "trust.prompt" || !seen[0].Allowed {
		t.Fatalf("first entry = %+v, want an allowed trust.prompt", seen[0])
	}
	if seen[1].Action != "trust.decision" || !seen[1].Allowed {
		t.Fatalf("second entry = %+v, want an allowed trust.decision", seen[1])
	}
	for _, e := range seen {
		if e.Scope != "plug" || e.SessionID != "s1" || e.Subject != "10.0.0.5:3389" {
			t.Fatalf("entry does not identify what it is about: %+v", e)
		}
		if e.Fingerprint != domain.PeerFingerprint(material) {
			t.Fatalf("entry fingerprint = %q, want the one the user was shown", e.Fingerprint)
		}
		// The material is what the vault is for. An audit line carrying it would be a second
		// copy of the thing being protected.
		if strings.Contains(e.Subject+e.Fingerprint+e.Error, string(material)) {
			t.Fatalf("raw material leaked into an audit entry: %+v", e)
		}
	}
}

// A refusal is the entry an investigation needs most, so it must not be the one that is missing.
func TestAuditRecordsARejection(t *testing.T) {
	sessions := &fakeTrustSessions{conn: rdpConnection()}
	svc := newTrustService(newFakeTrustRepo(), sessions)
	var seen []usecase.PeerTrustAuditEntry
	svc.SetAuditRecorder(func(e usecase.PeerTrustAuditEntry) { seen = append(seen, e) })

	if _, err := svc.Verify(context.Background(), "plug", "s1", "10.0.0.5:3389", []byte{1}); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if err := svc.Resolve(context.Background(), "plug", "s1", "reject", promptFingerprint(t, sessions)); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(seen) != 2 {
		t.Fatalf("audit entries = %d, want 2", len(seen))
	}
	if seen[1].Action != "trust.decision" || seen[1].Allowed {
		t.Fatalf("the rejection was not recorded as denied: %+v", seen[1])
	}
}

// A question that was refused is recorded as denied rather than not recorded at all: a plugin
// probing a session that is already asking about something else is exactly what an incident
// review would want to see.
func TestAuditRecordsARefusedQuestion(t *testing.T) {
	sessions := &fakeTrustSessions{conn: rdpConnection(), beginErr: domain.ErrPeerTrustPromptBusy}
	svc := newTrustService(newFakeTrustRepo(), sessions)
	var seen []usecase.PeerTrustAuditEntry
	svc.SetAuditRecorder(func(e usecase.PeerTrustAuditEntry) { seen = append(seen, e) })

	if _, err := svc.Verify(context.Background(), "plug", "s1", "10.0.0.5:3389", []byte{1}); err == nil {
		t.Fatal("the refusal did not reach the caller")
	}
	if len(seen) != 1 || seen[0].Allowed {
		t.Fatalf("audit = %+v, want one denied entry", seen)
	}
	if seen[0].Error == "" {
		t.Fatal("a denied entry with no reason cannot be acted on")
	}
}
