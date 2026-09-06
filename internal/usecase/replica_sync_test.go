package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"xquakshell/internal/domain"
)

// memoryRemote is a server that behaves: it holds one sealed blob and one token, and refuses a push
// that names a token it no longer has.
type memoryRemote struct {
	sealed []byte
	token  string

	pushes    int
	failFetch error
	failPush  error
}

func (r *memoryRemote) Fetch(context.Context, string) ([]byte, string, error) {
	if r.failFetch != nil {
		return nil, "", r.failFetch
	}
	return r.sealed, r.token, nil
}

func (r *memoryRemote) Push(_ context.Context, _ string, sealed []byte, expected string) (string, error) {
	if r.failPush != nil {
		return "", r.failPush
	}
	if expected != r.token {
		return "", domain.ErrReplicaConflict
	}
	r.pushes++
	r.sealed, r.token = sealed, expected+"+"
	return r.token, nil
}

// plainSealer stands in for the real one so a test can read what travelled. Sealing is covered where
// it lives, in internal/infra/vault; what matters here is that the service seals before pushing and
// opens after fetching, which a transparent sealer still proves.
type plainSealer struct {
	key   string
	seals int
}

func (s *plainSealer) Seal(doc domain.ReplicaDocument, key string) ([]byte, error) {
	if key == "" {
		return nil, domain.ErrReplicaKeyRequired
	}
	s.key, s.seals = key, s.seals+1
	return marshalDoc(doc)
}

func (s *plainSealer) Open(sealed []byte, key string) (domain.ReplicaDocument, error) {
	if key == "" {
		return domain.ReplicaDocument{}, domain.ErrReplicaKeyRequired
	}
	return unmarshalDoc(sealed)
}

// marshalDoc and unmarshalDoc are the transparent stand-in for sealing. A malformed payload comes
// back as ErrReplicaOpenFailed, the same way the real sealer reports one.
func marshalDoc(doc domain.ReplicaDocument) ([]byte, error) {
	return json.Marshal(doc)
}

func unmarshalDoc(sealed []byte) (domain.ReplicaDocument, error) {
	var doc domain.ReplicaDocument
	if err := json.Unmarshal(sealed, &doc); err != nil {
		return domain.ReplicaDocument{}, domain.ErrReplicaOpenFailed
	}
	return doc, nil
}

func syncFixture(t *testing.T) (*ReplicaSyncService, *replicaKeyVault, *memoryRemote, *plainSealer) {
	t.Helper()
	data := domain.NewVaultData()
	data.Folders = []domain.ConnectionFolder{
		{ID: "sync", Name: "Sync"},
		{ID: "personal", Name: "Personal"},
	}
	data.Connections = []domain.Connection{
		{ID: "mine", FolderID: "sync", Host: "a", Port: 22},
		{ID: "private", FolderID: "personal", Host: "b", Port: 22},
	}
	data.Settings.Plugins.ScopeRoots = []domain.ScopeRoot{{FolderID: "sync", PluginID: "com.example.sync"}}
	data.ReplicaKeys = map[string]string{"com.example.sync": "THEKEY"}

	vault := &replicaKeyVault{data: data}
	remote := &memoryRemote{}
	sealer := &plainSealer{}
	settings := NewPluginVaultSettings(vault)
	return NewReplicaSyncService(vault, settings, sealer, remote), vault, remote, sealer
}

func connIn(data *domain.VaultData, id string) *domain.Connection {
	for i := range data.Connections {
		if data.Connections[i].ID == id {
			return &data.Connections[i]
		}
	}
	return nil
}

func scopeVersion(t *testing.T, data *domain.VaultData) domain.VersionVector {
	t.Helper()
	for _, root := range data.Settings.Plugins.ScopeRoots {
		if root.PluginID == "com.example.sync" {
			return root.Version
		}
	}
	t.Fatal("the scope root disappeared")
	return nil
}

// The first device to synchronise finds nothing there and puts its scope up. Nothing arrives, and
// an empty remote must not be read as the other side having deleted everything.
func TestTheFirstSyncPushesTheScopeAndAppliesNothing(t *testing.T) {
	service, vault, remote, sealer := syncFixture(t)

	report, err := service.Sync(context.Background(), "com.example.sync")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if remote.pushes != 1 {
		t.Fatalf("%d pushes, want the local scope to have gone up", remote.pushes)
	}
	if sealer.key != "THEKEY" {
		t.Errorf("sealed with %q, want the plugin's replication key", sealer.key)
	}
	if !report.Pushed {
		t.Error("the report does not say the scope was pushed")
	}
	if len(report.Added)+len(report.Updated) != 0 {
		t.Errorf("an empty remote reported changes: %+v", report)
	}
	if connIn(vault.data, "mine") == nil || connIn(vault.data, "private") == nil {
		t.Error("the local vault changed on a first sync")
	}
}

// What the first device pushed is what the second one gets - the whole point, tested end to end
// through both directions of the same service.
func TestASecondDeviceReceivesWhatTheFirstPushed(t *testing.T) {
	first, _, remote, _ := syncFixture(t)
	if _, err := first.Sync(context.Background(), "com.example.sync"); err != nil {
		t.Fatalf("first Sync: %v", err)
	}

	_, secondVault, _, _ := syncFixture(t)
	secondVault.data.Connections = []domain.Connection{{ID: "theirs", FolderID: "sync", Host: "c", Port: 22}}
	secondService := NewReplicaSyncService(
		secondVault, NewPluginVaultSettings(secondVault), &plainSealer{}, remote)

	report, err := secondService.Sync(context.Background(), "com.example.sync")
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}

	if connIn(secondVault.data, "mine") == nil {
		t.Fatal("the first device's connection did not arrive on the second")
	}
	if !slices.Contains(report.Added, "mine") {
		t.Errorf("Added = %v, want the connection that arrived", report.Added)
	}
	if connIn(secondVault.data, "theirs") == nil {
		t.Error("the second device's own connection was lost")
	}
}

// Nothing outside the folder leaves the machine, whatever the remote does. This is the invariant the
// whole design exists for, checked on the bytes that actually travelled.
func TestNothingOutsideTheScopeIsEverSent(t *testing.T) {
	service, _, remote, _ := syncFixture(t)

	if _, err := service.Sync(context.Background(), "com.example.sync"); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	sent, err := unmarshalDoc(remote.sealed)
	if err != nil {
		t.Fatalf("what was pushed does not parse: %v", err)
	}
	for _, conn := range sent.Connections {
		if conn.ID == "private" {
			t.Fatal("a connection from outside the scope was sent")
		}
	}
	for _, folder := range sent.Folders {
		if folder.ID == "personal" {
			t.Fatal("a folder from outside the scope was sent")
		}
	}
}

// A plugin with no replication key has not been set up yet. That is an ordinary state, reported
// rather than retried, and nothing may be pushed under a key that does not exist.
func TestSyncingWithoutAKeyIsReportedNotAttempted(t *testing.T) {
	service, vault, remote, _ := syncFixture(t)
	vault.data.ReplicaKeys = nil

	_, err := service.Sync(context.Background(), "com.example.sync")

	if !errors.Is(err, domain.ErrReplicaKeyRequired) {
		t.Fatalf("err = %v, want ErrReplicaKeyRequired", err)
	}
	if remote.pushes != 0 {
		t.Fatal("a scope was pushed with no key to seal it")
	}
}

// A plugin with no scope has nothing to synchronise, and pushing an empty document would tell the
// other devices this one had deleted everything.
func TestSyncingAPluginWithNoScopeIsRefused(t *testing.T) {
	service, vault, remote, _ := syncFixture(t)
	vault.data.Settings.Plugins.ScopeRoots = nil

	if _, err := service.Sync(context.Background(), "com.example.sync"); err == nil {
		t.Fatal("a plugin with no scope was synchronised")
	}
	if remote.pushes != 0 {
		t.Fatal("something was pushed for a plugin with no scope")
	}
}

// The version has to advance and be written down, or the very next sync reports the divergence that
// was just resolved and the two devices never converge.
func TestTheVersionAdvancesAndIsRecorded(t *testing.T) {
	service, vault, _, _ := syncFixture(t)

	if _, err := service.Sync(context.Background(), "com.example.sync"); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	version := scopeVersion(t, vault.data)
	if len(version) == 0 {
		t.Fatal("the scope's version was not recorded")
	}
	deviceID := vault.data.Settings.Plugins.ReplicaDeviceID
	if deviceID == "" {
		t.Fatal("no device id was minted")
	}
	if version[deviceID] == 0 {
		t.Fatalf("version = %v, want this device's counter to have advanced", version)
	}
}

// The device id is minted once. A value that changed between syncs would make this machine look
// like a third device that had seen nothing, and every later sync would read as a divergence.
func TestTheDeviceIdIsMintedOnceAndKept(t *testing.T) {
	service, vault, _, _ := syncFixture(t)

	if _, err := service.Sync(context.Background(), "com.example.sync"); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	first := vault.data.Settings.Plugins.ReplicaDeviceID
	if _, err := service.Sync(context.Background(), "com.example.sync"); err != nil {
		t.Fatalf("second Sync: %v", err)
	}

	if vault.data.Settings.Plugins.ReplicaDeviceID != first {
		t.Fatalf("the device id changed from %q to %q", first, vault.data.Settings.Plugins.ReplicaDeviceID)
	}
}

// Another device wrote between this one's read and its push. Nothing is overwritten and nothing is
// lost: the conflict is reported, and the next sync starts from what is actually there.
func TestAConflictingPushIsReportedRatherThanForced(t *testing.T) {
	service, _, remote, _ := syncFixture(t)
	remote.failPush = domain.ErrReplicaConflict

	_, err := service.Sync(context.Background(), "com.example.sync")

	if !errors.Is(err, domain.ErrReplicaConflict) {
		t.Fatalf("err = %v, want ErrReplicaConflict", err)
	}
}

// A transport that fails must fail the sync. Treated as an empty remote it would look exactly like
// the user having deleted everything on their other device.
func TestAFailingFetchFailsTheSync(t *testing.T) {
	service, _, remote, _ := syncFixture(t)
	remote.failFetch = errors.New("the network is down")

	if _, err := service.Sync(context.Background(), "com.example.sync"); err == nil {
		t.Fatal("a failed fetch was treated as an empty remote")
	}
}

// A document that cannot be opened with this key is refused, and nothing about the local vault
// changes. This is what a wrong key on the second device looks like, and what a hostile server
// serving noise looks like too.
func TestAReplicaThatWillNotOpenLeavesTheVaultAlone(t *testing.T) {
	service, vault, remote, _ := syncFixture(t)
	remote.sealed, remote.token = []byte("not a replica"), "v1"
	before := len(vault.data.Connections)

	if _, err := service.Sync(context.Background(), "com.example.sync"); err == nil {
		t.Fatal("an unopenable replica was applied")
	}
	if len(vault.data.Connections) != before {
		t.Fatal("the vault changed while applying a replica that could not be opened")
	}
}

// The attack that motivates the apply guards, driven through the real service: a server serving a
// document that reaches outside the folder must change nothing at all.
func TestAHostileRemoteCannotReachOutsideTheScope(t *testing.T) {
	service, vault, remote, _ := syncFixture(t)
	hostile := domain.ReplicaDocument{
		Scope:   "com.example.sync",
		Version: domain.VersionVector{"attacker": 99},
		Folders: []domain.ConnectionFolder{{ID: "sync", Name: "Sync"}},
		Connections: []domain.Connection{
			{ID: "private", FolderID: "personal", Host: "attacker.example.com", Port: 22},
		},
	}
	sealed, err := marshalDoc(hostile)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	remote.sealed, remote.token = sealed, "v1"

	if _, err := service.Sync(context.Background(), "com.example.sync"); !errors.Is(err, domain.ErrOutsideScope) {
		t.Fatalf("err = %v, want ErrOutsideScope", err)
	}
	if host := connIn(vault.data, "private").Host; host != "b" {
		t.Fatalf("the out-of-scope connection was rewritten to %q", host)
	}
}

// A remote that has dropped everything must not empty this device. The loss is reported so the user
// can be asked; nothing is deleted (I9).
func TestAWipedRemoteProposesRatherThanDeletes(t *testing.T) {
	service, vault, remote, _ := syncFixture(t)
	if _, err := service.Sync(context.Background(), "com.example.sync"); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	wiped := domain.ReplicaDocument{
		Scope:   "com.example.sync",
		Version: domain.VersionVector{vault.data.Settings.Plugins.ReplicaDeviceID: 99},
		Folders: []domain.ConnectionFolder{{ID: "sync", Name: "Sync"}},
	}
	sealed, err := marshalDoc(wiped)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// The token stays as it is: the remote did not move, it lost its contents.
	remote.sealed = sealed

	report, err := service.Sync(context.Background(), "com.example.sync")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if connIn(vault.data, "mine") == nil {
		t.Fatal("a wiped remote deleted the local connection")
	}
	if !slices.Contains(report.Removed, "mine") {
		t.Errorf("Removed = %v, want the loss proposed to the user", report.Removed)
	}
}
