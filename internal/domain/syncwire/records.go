package syncwire

import (
	"bytes"

	"xquakshell/internal/domain/vaultsync"
)

// Envelope is the wire form of vaultsync.Envelope.
type Envelope struct {
	Vault      string `json:"vault"`
	Record     string `json:"record"`
	Version    uint64 `json:"version"`
	Epoch      uint32 `json:"epoch"`
	Suite      uint16 `json:"suite"`
	Deleted    bool   `json:"deleted"`
	WrappedDEK []byte `json:"wrappedDek,omitempty"`
	Body       []byte `json:"body,omitempty"`
	Author     string `json:"author"`
	SigAlg     uint16 `json:"sigAlg"`
	Sig        []byte `json:"sig"`
}

// EnvelopeFrom converts a domain envelope for the wire.
func EnvelopeFrom(e *vaultsync.Envelope) Envelope {
	return Envelope{
		Vault: e.Vault.String(), Record: e.Record.String(), Version: e.Version, Epoch: e.Epoch,
		Suite: uint16(e.Suite), Deleted: e.Deleted, WrappedDEK: bytes.Clone(e.WrappedDEK),
		Body: bytes.Clone(e.Body), Author: e.Author.String(), SigAlg: uint16(e.SigAlg), Sig: bytes.Clone(e.Sig),
	}
}

// ToDomain converts and checks identifiers; structure and signature are checked by vaultsync.
func (d Envelope) ToDomain() (vaultsync.Envelope, error) {
	vault, err := parseID(d.Vault)
	if err != nil {
		return vaultsync.Envelope{}, err
	}
	record, err := parseID(d.Record)
	if err != nil {
		return vaultsync.Envelope{}, err
	}
	author, err := parseID(d.Author)
	if err != nil {
		return vaultsync.Envelope{}, err
	}
	return vaultsync.Envelope{
		Vault: vault, Record: record, Version: d.Version, Epoch: d.Epoch, Suite: vaultsync.SuiteID(d.Suite),
		Deleted: d.Deleted, WrappedDEK: d.WrappedDEK, Body: d.Body, Author: author,
		SigAlg: vaultsync.SigAlg(d.SigAlg), Sig: d.Sig,
	}, nil
}

// Head is the wire form of vaultsync.Head.
type Head struct {
	Vault               string `json:"vault"`
	Seq                 uint64 `json:"seq"`
	PrevHash            []byte `json:"prevHash"`
	MerkleRoot          []byte `json:"merkleRoot"`
	CurrentEpoch        uint32 `json:"currentEpoch"`
	MembershipSeq       uint64 `json:"membershipSeq"`
	MembershipHead      []byte `json:"membershipHead"`
	Commit              string `json:"commit"`
	Author              string `json:"author"`
	ClientTimeUnixMilli uint64 `json:"clientTimeUnixMilli"`
	SigAlg              uint16 `json:"sigAlg"`
	Sig                 []byte `json:"sig"`
}

// HeadFrom converts a domain head for the wire.
func HeadFrom(h *vaultsync.Head) Head {
	return Head{
		Vault: h.Vault.String(), Seq: h.Seq, PrevHash: bytes.Clone(h.PrevHash[:]), MerkleRoot: bytes.Clone(h.MerkleRoot[:]),
		CurrentEpoch: h.CurrentEpoch, MembershipSeq: h.MembershipSeq, MembershipHead: bytes.Clone(h.MembershipHead[:]),
		Commit: h.Commit.String(), Author: h.Author.String(), ClientTimeUnixMilli: h.ClientTimeUnixMilli,
		SigAlg: uint16(h.SigAlg), Sig: bytes.Clone(h.Sig),
	}
}

// ToDomain converts and checks identifiers and hash lengths.
func (d Head) ToDomain() (vaultsync.Head, error) {
	var h vaultsync.Head
	var err error
	if h.Vault, err = parseID(d.Vault); err != nil {
		return vaultsync.Head{}, err
	}
	if h.Commit, err = parseID(d.Commit); err != nil {
		return vaultsync.Head{}, err
	}
	if h.Author, err = parseID(d.Author); err != nil {
		return vaultsync.Head{}, err
	}
	if h.PrevHash, err = parseHash(d.PrevHash); err != nil {
		return vaultsync.Head{}, err
	}
	if h.MerkleRoot, err = parseHash(d.MerkleRoot); err != nil {
		return vaultsync.Head{}, err
	}
	if h.MembershipHead, err = parseHash(d.MembershipHead); err != nil {
		return vaultsync.Head{}, err
	}
	h.Seq, h.CurrentEpoch, h.MembershipSeq = d.Seq, d.CurrentEpoch, d.MembershipSeq
	h.ClientTimeUnixMilli, h.SigAlg, h.Sig = d.ClientTimeUnixMilli, vaultsync.SigAlg(d.SigAlg), d.Sig
	return h, nil
}
