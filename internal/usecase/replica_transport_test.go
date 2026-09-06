package usecase

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
)

// fakeRemote answers replica.fetch and replica.push the way a well-behaved plugin would: it serves
// whatever bytes it holds in ReplicaChunkBytes pieces, and accepts a push only against the token it
// currently holds. Written as a real participant rather than a stub, so a test that passes here
// says the two sides actually agree on the protocol.
type fakeRemote struct {
	stored  []byte
	token   string
	pending []byte

	calls     []string
	chunkSize int // 0 means ReplicaChunkBytes
	failWith  error
}

func (r *fakeRemote) Call(_ context.Context, _, method string, params json.RawMessage) (json.RawMessage, error) {
	r.calls = append(r.calls, method)
	if r.failWith != nil {
		return nil, r.failWith
	}
	switch method {
	case "replica.fetch":
		return r.fetch(params)
	case "replica.push":
		return r.push(params)
	}
	return nil, fmt.Errorf("unexpected method %q", method)
}

func (r *fakeRemote) fetch(params json.RawMessage) (json.RawMessage, error) {
	var in struct {
		Offset int `json:"offset"`
	}
	if err := json.Unmarshal(params, &in); err != nil {
		return nil, err
	}
	if len(r.stored) == 0 {
		return json.RawMessage(`{}`), nil
	}
	size := r.chunkSize
	if size == 0 {
		size = ReplicaChunkBytes
	}
	end := min(in.Offset+size, len(r.stored))
	return json.Marshal(map[string]any{
		"token":       r.token,
		"totalBytes":  len(r.stored),
		"chunkBase64": base64.StdEncoding.EncodeToString(r.stored[in.Offset:end]),
	})
}

func (r *fakeRemote) push(params json.RawMessage) (json.RawMessage, error) {
	var in struct {
		ExpectedToken string `json:"expectedToken"`
		Offset        int    `json:"offset"`
		TotalBytes    int    `json:"totalBytes"`
		ChunkBase64   string `json:"chunkBase64"`
	}
	if err := json.Unmarshal(params, &in); err != nil {
		return nil, err
	}
	if in.ExpectedToken != r.token {
		return json.RawMessage(`{"conflict":true}`), nil
	}
	chunk, err := base64.StdEncoding.DecodeString(in.ChunkBase64)
	if err != nil {
		return nil, err
	}
	if in.Offset == 0 {
		r.pending = nil
	}
	r.pending = append(r.pending, chunk...)
	if len(r.pending) < in.TotalBytes {
		return json.RawMessage(`{}`), nil
	}
	r.stored, r.pending = r.pending, nil
	r.token = fmt.Sprintf("v%d", len(r.calls))
	return json.Marshal(map[string]string{"token": r.token})
}

// scriptedRemote answers with whatever the test dictates, call by call, so a hostile or broken
// protocol can be reproduced exactly.
type scriptedRemote struct {
	replies []string
	sent    []json.RawMessage
	n       int
}

func (s *scriptedRemote) Call(_ context.Context, _, _ string, params json.RawMessage) (json.RawMessage, error) {
	s.sent = append(s.sent, params)
	if s.n >= len(s.replies) {
		return nil, errors.New("the transport was called more times than the test scripted")
	}
	reply := s.replies[s.n]
	s.n++
	return json.RawMessage(reply), nil
}

func fetchFrom(t *testing.T, replies ...string) ([]byte, string, error) {
	t.Helper()
	return NewPluginReplicaTransport(&scriptedRemote{replies: replies}).
		Fetch(context.Background(), "com.example.sync")
}

func chunk(payload []byte, token string, total int) string {
	return fmt.Sprintf(`{"token":%q,"totalBytes":%d,"chunkBase64":%q}`,
		token, total, base64.StdEncoding.EncodeToString(payload))
}

// The round trip against a remote that behaves is the contract in one test: what one device pushes,
// another fetches back byte for byte.
func TestWhatIsPushedIsWhatComesBack(t *testing.T) {
	remote := &fakeRemote{token: ""}
	transport := NewPluginReplicaTransport(remote)
	sealed := bytes.Repeat([]byte("sealed replica bytes "), 20000) // several chunks

	token, err := transport.Push(context.Background(), "com.example.sync", sealed, "")
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	got, gotToken, err := transport.Fetch(context.Background(), "com.example.sync")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if !bytes.Equal(got, sealed) {
		t.Fatalf("fetched %d bytes, pushed %d", len(got), len(sealed))
	}
	if gotToken != token {
		t.Errorf("token = %q, want the one the push returned (%q)", gotToken, token)
	}
}

// A payload larger than one RPC frame is the ordinary case, not the exception: a scope holding a
// handful of SSH keys already exceeds it. If the transport ever sent one frame per replica, this is
// the test that would say so.
func TestAReplicaLargerThanOneFrameIsMovedInChunks(t *testing.T) {
	remote := &fakeRemote{}
	transport := NewPluginReplicaTransport(remote)
	sealed := bytes.Repeat([]byte("x"), 3*ReplicaChunkBytes+17)

	if _, err := transport.Push(context.Background(), "com.example.sync", sealed, ""); err != nil {
		t.Fatalf("Push: %v", err)
	}

	pushes := 0
	for _, method := range remote.calls {
		if method == "replica.push" {
			pushes++
		}
	}
	if pushes != 4 {
		t.Fatalf("%d push calls for %d bytes, want one per chunk of %d", pushes, len(sealed), ReplicaChunkBytes)
	}
	got, _, err := transport.Fetch(context.Background(), "com.example.sync")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !bytes.Equal(got, sealed) {
		t.Fatal("the reassembled replica does not match what was sent")
	}
}

// No chunk may exceed what a frame can carry once base64 has inflated it by a third and the JSON
// envelope has been added, or the plugin's own read fails on a frame it cannot accept.
func TestAChunkFitsInsideAnRPCFrame(t *testing.T) {
	remote := &fakeRemote{}
	transport := NewPluginReplicaTransport(remote)

	if _, err := transport.Push(context.Background(), "com.example.sync", bytes.Repeat([]byte("x"), 2*ReplicaChunkBytes), ""); err != nil {
		t.Fatalf("Push: %v", err)
	}

	encoded := base64.StdEncoding.EncodedLen(ReplicaChunkBytes)
	if encoded >= domainplugin.MaxFrameBytes {
		t.Fatalf("a %d-byte chunk encodes to %d, which does not fit a %d-byte frame",
			ReplicaChunkBytes, encoded, domainplugin.MaxFrameBytes)
	}
}

// A remote with nothing stored yet is what the first device to synchronise sees. It is not an
// error, and it must not read as "the other side deleted everything" either - the caller tells the
// two apart because there is no token.
func TestFetchFromAnEmptyRemoteIsNotAnError(t *testing.T) {
	sealed, token, err := fetchFrom(t, `{}`)

	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(sealed) != 0 || token != "" {
		t.Fatalf("sealed = %q, token = %q; want nothing", sealed, token)
	}
}

// The transport is assumed hostile, so what it declares is bounded before a single byte is read.
// Without a ceiling a plugin announces a replica of any size and the host allocates for it.
func TestAFetchDeclaringMoreThanTheCeilingIsRefusedBeforeReading(t *testing.T) {
	// The chunk is real and legal, so the size guard is the only thing that can refuse this. An
	// empty chunk here would be caught by the no-progress guard instead, and the test would pass
	// while proving nothing about the ceiling.
	remote := &scriptedRemote{replies: []string{
		chunk([]byte("legal chunk"), "v1", MaxReplicaBytes+1)}}

	_, _, err := NewPluginReplicaTransport(remote).Fetch(context.Background(), "com.example.sync")

	if !errors.Is(err, domain.ErrReplicaOpenFailed) {
		t.Fatalf("err = %v, want ErrReplicaOpenFailed", err)
	}
	if len(remote.sent) != 1 {
		t.Errorf("%d calls made; the size was announced on the first, so no more should follow", len(remote.sent))
	}
}

// The ceiling is a limit, not a target to stay under: a replica landing exactly on it is one the
// user is entitled to synchronise. Without this the comparison could be strict in the wrong
// direction and nothing else in the suite would notice.
func TestAReplicaExactlyAtTheCeilingIsAccepted(t *testing.T) {
	remote := &fakeRemote{}
	transport := NewPluginReplicaTransport(remote)

	if _, err := transport.Push(context.Background(), "com.example.sync", bytes.Repeat([]byte("x"), MaxReplicaBytes), ""); err != nil {
		t.Fatalf("a replica of exactly %d bytes was refused: %v", MaxReplicaBytes, err)
	}
	got, _, err := transport.Fetch(context.Background(), "com.example.sync")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(got) != MaxReplicaBytes {
		t.Fatalf("got %d bytes, want %d", len(got), MaxReplicaBytes)
	}
}

// Pushing more than the ceiling fails here, loudly, rather than at whatever limit the plugin's own
// server happens to have - where the user would see it as an opaque failure from a third party.
func TestPushingMoreThanTheCeilingIsRefused(t *testing.T) {
	remote := &fakeRemote{}

	_, err := NewPluginReplicaTransport(remote).Push(
		context.Background(), "com.example.sync", bytes.Repeat([]byte("x"), MaxReplicaBytes+1), "")

	if err == nil {
		t.Fatal("a replica over the ceiling was pushed")
	}
	if len(remote.calls) != 0 {
		t.Errorf("%d calls made; nothing should reach the plugin", len(remote.calls))
	}
}

// A chunk that overruns what was announced is a plugin answering a question it was not asked, and
// growing the buffer past the ceiling one chunk at a time is exactly how a declared-size check gets
// bypassed.
func TestAChunkOverrunningTheDeclaredSizeIsRefused(t *testing.T) {
	_, _, err := fetchFrom(t, chunk(bytes.Repeat([]byte("x"), 40), "v1", 10))

	if !errors.Is(err, domain.ErrReplicaOpenFailed) {
		t.Fatalf("err = %v, want ErrReplicaOpenFailed", err)
	}
}

// A chunk larger than the protocol allows would not have fitted in the frame that carried it, so a
// plugin sending one is not speaking this protocol.
func TestAnOversizedChunkIsRefused(t *testing.T) {
	oversized := bytes.Repeat([]byte("x"), ReplicaChunkBytes+1)

	_, _, err := fetchFrom(t, chunk(oversized, "v1", 4*ReplicaChunkBytes))

	if !errors.Is(err, domain.ErrReplicaOpenFailed) {
		t.Fatalf("err = %v, want ErrReplicaOpenFailed", err)
	}
}

// An empty chunk before the end makes no progress, and a transport that trusted the remote to
// finish would loop on it forever - a hostile plugin hanging the unlock path with one short reply.
func TestAChunkThatMakesNoProgressIsRefused(t *testing.T) {
	_, _, err := fetchFrom(t, chunk(nil, "v1", 100))

	if !errors.Is(err, domain.ErrReplicaOpenFailed) {
		t.Fatalf("err = %v, want ErrReplicaOpenFailed", err)
	}
}

// The token pins the version being read. If it changes mid-read the remaining chunks belong to a
// different document, and splicing them onto the first half would produce bytes that were never
// sealed together.
func TestTheVersionChangingMidReadIsRefused(t *testing.T) {
	total := ReplicaChunkBytes + 10
	first := bytes.Repeat([]byte("a"), ReplicaChunkBytes)
	second := bytes.Repeat([]byte("b"), 10)

	_, _, err := fetchFrom(t, chunk(first, "v1", total), chunk(second, "v2", total))

	if !errors.Is(err, domain.ErrReplicaOpenFailed) {
		t.Fatalf("err = %v, want ErrReplicaOpenFailed when the version moves mid-read", err)
	}
}

// The size a fetch announces is also part of the version being read. A remote that revises it
// halfway is not serving one document.
func TestTheDeclaredSizeChangingMidReadIsRefused(t *testing.T) {
	first := bytes.Repeat([]byte("a"), ReplicaChunkBytes)
	second := bytes.Repeat([]byte("b"), 10)

	_, _, err := fetchFrom(t,
		chunk(first, "v1", ReplicaChunkBytes+10),
		chunk(second, "v1", ReplicaChunkBytes+999))

	if !errors.Is(err, domain.ErrReplicaOpenFailed) {
		t.Fatalf("err = %v, want ErrReplicaOpenFailed when the size moves mid-read", err)
	}
}

// A fetch that names bytes but no version cannot be pushed against later: the next push would have
// no token to compare and swap on, making it a blind overwrite.
func TestAFetchWithBytesButNoTokenIsRefused(t *testing.T) {
	_, _, err := fetchFrom(t, chunk([]byte("payload"), "", 7))

	if !errors.Is(err, domain.ErrReplicaOpenFailed) {
		t.Fatalf("err = %v, want ErrReplicaOpenFailed", err)
	}
}

// Each read asks for the bytes that are missing, so a remote serving them in any legal chunking
// still reassembles. A transport that assumed a fixed chunk size would silently drop or duplicate.
func TestEachReadAsksForWhatIsStillMissing(t *testing.T) {
	remote := &scriptedRemote{replies: []string{
		chunk([]byte("abc"), "v1", 9),
		chunk([]byte("de"), "v1", 9),
		chunk([]byte("fghi"), "v1", 9),
	}}

	sealed, _, err := NewPluginReplicaTransport(remote).Fetch(context.Background(), "com.example.sync")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if string(sealed) != "abcdefghi" {
		t.Fatalf("reassembled %q", sealed)
	}
	wantOffsets := []int{0, 3, 5}
	for i, params := range remote.sent {
		var in struct {
			Offset int `json:"offset"`
		}
		if err := json.Unmarshal(params, &in); err != nil {
			t.Fatalf("params: %v", err)
		}
		if in.Offset != wantOffsets[i] {
			t.Errorf("read %d asked for offset %d, want %d", i, in.Offset, wantOffsets[i])
		}
	}
}

// A payload that is not what it claims to be is refused rather than passed on as empty.
func TestAMalformedFetchIsRefused(t *testing.T) {
	for _, reply := range []string{
		`{"token":"v1","totalBytes":7,"chunkBase64":"not base64 at all!!"}`,
		`not json`,
		`{"token":"v1","totalBytes":-1,"chunkBase64":""}`,
	} {
		if _, _, err := fetchFrom(t, reply); err == nil {
			t.Errorf("%s: fetch succeeded, want an error", reply)
		}
	}
}

// Push carries the version the bytes were built from, so the remote can refuse if it has moved on.
func TestPushSendsTheExpectedTokenAndReturnsTheNewOne(t *testing.T) {
	remote := &scriptedRemote{replies: []string{`{"token":"v8"}`}}

	token, err := NewPluginReplicaTransport(remote).Push(
		context.Background(), "com.example.sync", []byte("sealed"), "v7")
	if err != nil {
		t.Fatalf("Push: %v", err)
	}

	if token != "v8" {
		t.Errorf("token = %q, want the version the remote now holds", token)
	}
	var sent struct {
		SealedChunk   string `json:"chunkBase64"`
		ExpectedToken string `json:"expectedToken"`
		TotalBytes    int    `json:"totalBytes"`
		Offset        int    `json:"offset"`
	}
	if err := json.Unmarshal(remote.sent[0], &sent); err != nil {
		t.Fatalf("params: %v", err)
	}
	if sent.ExpectedToken != "v7" {
		t.Errorf("expectedToken = %q, want the version that was read", sent.ExpectedToken)
	}
	if sent.TotalBytes != 6 || sent.Offset != 0 {
		t.Errorf("totalBytes = %d, offset = %d; want 6 and 0", sent.TotalBytes, sent.Offset)
	}
	if decoded, _ := base64.StdEncoding.DecodeString(sent.SealedChunk); string(decoded) != "sealed" {
		t.Errorf("the sealed bytes were not sent verbatim: %q", decoded)
	}
}

// Another device wrote first. That is not a failure to report to the user - it means read again and
// merge - and overwriting is the one thing compare-and-swap exists to prevent.
func TestARefusedPushIsAConflictRatherThanAFailure(t *testing.T) {
	transport := NewPluginReplicaTransport(&scriptedRemote{replies: []string{`{"conflict":true}`}})

	_, err := transport.Push(context.Background(), "com.example.sync", []byte("sealed"), "v7")

	if !errors.Is(err, domain.ErrReplicaConflict) {
		t.Fatalf("err = %v, want ErrReplicaConflict", err)
	}
}

// The conflict can arrive on any chunk, not just the first: another device can write while this one
// is still uploading. Carrying on would push the rest of a replica onto a version that moved.
func TestAConflictOnALaterChunkStopsTheUpload(t *testing.T) {
	remote := &scriptedRemote{replies: []string{`{}`, `{"conflict":true}`}}
	sealed := bytes.Repeat([]byte("x"), ReplicaChunkBytes+1)

	_, err := NewPluginReplicaTransport(remote).Push(context.Background(), "com.example.sync", sealed, "v7")

	if !errors.Is(err, domain.ErrReplicaConflict) {
		t.Fatalf("err = %v, want ErrReplicaConflict", err)
	}
	if remote.n != 2 {
		t.Errorf("%d calls made, want the upload to stop at the conflict", remote.n)
	}
}

// A push the transport accepted but answered without a token stored the bytes under a version
// nobody can name, so the next push has nothing to compare and swap against.
func TestAnAcceptedPushWithNoTokenIsRefused(t *testing.T) {
	transport := NewPluginReplicaTransport(&scriptedRemote{replies: []string{`{}`}})

	if _, err := transport.Push(context.Background(), "com.example.sync", []byte("sealed"), "v7"); err == nil {
		t.Fatal("a push answered with no token reported success")
	}
}

// Pushing nothing is how a wiped server would ask a healthy device to erase itself on the next
// fetch. Deletion never travels (I9), so an empty replica is not a thing this transport can send.
func TestPushingAnEmptyReplicaIsRefused(t *testing.T) {
	remote := &fakeRemote{}

	_, err := NewPluginReplicaTransport(remote).Push(context.Background(), "com.example.sync", nil, "v7")

	if !errors.Is(err, domain.ErrReplicaEmpty) {
		t.Fatalf("err = %v, want ErrReplicaEmpty", err)
	}
	if len(remote.calls) != 0 {
		t.Errorf("%d calls made; nothing should reach the plugin", len(remote.calls))
	}
}

// A plugin that fails outright fails the sync rather than being read as "nothing there".
func TestAFailingPluginFailsTheCall(t *testing.T) {
	boom := errors.New("transport is down")

	transport := NewPluginReplicaTransport(&fakeRemote{failWith: boom})

	if _, _, err := transport.Fetch(context.Background(), "com.example.sync"); !errors.Is(err, boom) {
		t.Errorf("Fetch err = %v, want the plugin's error", err)
	}
	if _, err := transport.Push(context.Background(), "com.example.sync", []byte("x"), ""); !errors.Is(err, boom) {
		t.Errorf("Push err = %v, want the plugin's error", err)
	}
}

// Composition can leave the caller unset, and a transport with nobody to call must say so rather
// than report an empty remote - which the merge would read as a deletion.
func TestATransportWithNoCallerFailsRatherThanReportsEmpty(t *testing.T) {
	transport := NewPluginReplicaTransport(nil)

	if _, _, err := transport.Fetch(context.Background(), "com.example.sync"); err == nil {
		t.Error("a transport with no caller reported an empty remote")
	}
	if _, err := transport.Push(context.Background(), "com.example.sync", []byte("x"), ""); err == nil {
		t.Error("a transport with no caller reported a successful push")
	}
}

// Cancellation has to reach the loop, not just the call inside it. A window closing mid-sync must
// stop the transfer rather than run it to completion against a remote that keeps answering.
func TestACancelledContextStopsTheTransfer(t *testing.T) {
	remote := &fakeRemote{stored: bytes.Repeat([]byte("x"), 4*ReplicaChunkBytes), token: "v1"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, _, err := NewPluginReplicaTransport(remote).Fetch(ctx, "com.example.sync"); !errors.Is(err, context.Canceled) {
		t.Errorf("Fetch err = %v, want context.Canceled", err)
	}
	if _, err := NewPluginReplicaTransport(remote).Push(ctx, "com.example.sync", bytes.Repeat([]byte("x"), 4*ReplicaChunkBytes), "v1"); !errors.Is(err, context.Canceled) {
		t.Errorf("Push err = %v, want context.Canceled", err)
	}
}

// The transport is what the replication service holds through the domain port.
func TestTheTransportSatisfiesTheDomainPort(t *testing.T) {
	var _ domain.ReplicaTransport = NewPluginReplicaTransport(nil)
}

// The method names are the wire contract a plugin author implements against, so they are pinned
// here rather than left to whatever the implementation happens to spell.
func TestTheWireMethodNames(t *testing.T) {
	remote := &fakeRemote{}
	transport := NewPluginReplicaTransport(remote)

	if _, err := transport.Push(context.Background(), "com.example.sync", []byte("sealed"), ""); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if _, _, err := transport.Fetch(context.Background(), "com.example.sync"); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if got := strings.Join(remote.calls, ","); got != "replica.push,replica.fetch" {
		t.Fatalf("methods called: %s", got)
	}
}
