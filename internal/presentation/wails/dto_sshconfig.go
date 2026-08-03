package wails

import "xquakshell/internal/usecase"

// SSHConfigHostDTO is one importable host as the dialog renders it.
type SSHConfigHostDTO struct {
	Alias    string `json:"alias"`
	HostName string `json:"hostName"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	// KeyCount is how many private keys this host references, jump hops
	// included, so the row can warn before any key material is read.
	KeyCount int `json:"keyCount"`
	// JumpAliases names the ProxyJump chain in traversal order.
	JumpAliases []string `json:"jumpAliases"`
	// Duplicate marks a host already present in the vault.
	Duplicate bool `json:"duplicate"`
}

// SSHConfigNoticeDTO is a non-fatal parse finding.
//
// Only a machine-readable kind and a short target cross the bridge; the
// frontend turns the kind into a sentence. No message from the parser or the
// operating system is forwarded, so the UI cannot leak internal detail.
type SSHConfigNoticeDTO struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
}

// SSHConfigPreviewDTO is the result of inspecting a config file.
type SSHConfigPreviewDTO struct {
	Path         string               `json:"path"`
	Hosts        []SSHConfigHostDTO   `json:"hosts"`
	KeyFileCount int                  `json:"keyFileCount"`
	Notices      []SSHConfigNoticeDTO `json:"notices"`
}

// SSHConfigImportResultDTO reports what an import produced.
type SSHConfigImportResultDTO struct {
	Connections    []ConnectionDTO `json:"connections"`
	ImportedKeys   int             `json:"importedKeys"`
	FailedKeys     int             `json:"failedKeys"`
	SkippedAliases []string        `json:"skippedAliases"`
}

func sshConfigPreviewToDTO(p usecase.SSHConfigPreview) SSHConfigPreviewDTO {
	dto := SSHConfigPreviewDTO{
		Path:         p.Path,
		Hosts:        make([]SSHConfigHostDTO, 0, len(p.Hosts)),
		KeyFileCount: p.KeyFileCount,
		Notices:      make([]SSHConfigNoticeDTO, 0, len(p.Notices)),
	}
	for _, h := range p.Hosts {
		dto.Hosts = append(dto.Hosts, SSHConfigHostDTO{
			Alias:       h.Alias,
			HostName:    h.HostName,
			Port:        h.Port,
			User:        h.User,
			KeyCount:    h.KeyCount,
			JumpAliases: nonNilStrings(h.JumpAliases),
			Duplicate:   h.Duplicate,
		})
	}
	for _, n := range p.Notices {
		dto.Notices = append(dto.Notices, SSHConfigNoticeDTO{Kind: string(n.Kind), Target: n.Target})
	}
	return dto
}

func sshConfigImportResultToDTO(r usecase.SSHConfigImportResult) SSHConfigImportResultDTO {
	dto := SSHConfigImportResultDTO{
		Connections:    make([]ConnectionDTO, 0, len(r.Connections)),
		ImportedKeys:   r.ImportedKeys,
		FailedKeys:     r.FailedKeys,
		SkippedAliases: nonNilStrings(r.SkippedAliases),
	}
	for _, c := range r.Connections {
		dto.Connections = append(dto.Connections, ConnectionToDTO(c))
	}
	return dto
}

// nonNilStrings keeps optional lists as [] rather than null on the wire, so
// the frontend can iterate without a null check.
func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
