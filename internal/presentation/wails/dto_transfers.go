package wails

import (
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/usecase"
)

// ConflictInfoDTO is the existing-target metadata shown in the conflict dialog.
type ConflictInfoDTO struct {
	Size    int64  `json:"size"`
	ModTime string `json:"modTime"`
	IsDir   bool   `json:"isDir"`
}

// PlannedFileDTO is one file of a planned transfer. Conflict is non-nil when the
// target already exists.
type PlannedFileDTO struct {
	Source     string           `json:"source"`
	Target     string           `json:"target"`
	Size       int64            `json:"size"`
	SrcModTime string           `json:"srcModTime"`
	Conflict   *ConflictInfoDTO `json:"conflict,omitempty"`
}

// TransferPlanDTO is the wire form of usecase.TransferPlan.
type TransferPlanDTO struct {
	Kind    string           `json:"kind"`
	OpID    string           `json:"opID"`
	DestDir string           `json:"destDir"`
	Dirs    []string         `json:"dirs"`
	Files   []PlannedFileDTO `json:"files"`
}

// ResolvedActionDTO is the caller's decision for one conflicting target.
type ResolvedActionDTO struct {
	Target  string `json:"target"`
	Action  string `json:"action"`
	NewName string `json:"newName,omitempty"`
}

// ExecutePlanDTO carries a plan and its per-conflict resolutions back for
// execution.
type ExecutePlanDTO struct {
	Plan        TransferPlanDTO     `json:"plan"`
	Resolutions []ResolvedActionDTO `json:"resolutions"`
}

func formatModTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339Nano)
}

func parseModTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	return time.Time{}
}

// transferPlanToDTO converts a domain/usecase plan to its wire form.
func transferPlanToDTO(plan *usecase.TransferPlan) TransferPlanDTO {
	if plan == nil {
		return TransferPlanDTO{}
	}
	dto := TransferPlanDTO{Kind: plan.Kind, OpID: plan.OpID, DestDir: plan.DestDir, Dirs: plan.Dirs}
	dto.Files = make([]PlannedFileDTO, 0, len(plan.Files))
	for _, f := range plan.Files {
		fd := PlannedFileDTO{
			Source:     f.Source,
			Target:     f.Target,
			Size:       f.Size,
			SrcModTime: formatModTime(f.SrcModTime),
		}
		if f.Conflict != nil {
			fd.Conflict = &ConflictInfoDTO{
				Size:    f.Conflict.Size,
				ModTime: formatModTime(f.Conflict.ModTime),
				IsDir:   f.Conflict.IsDir,
			}
		}
		dto.Files = append(dto.Files, fd)
	}
	return dto
}

// dtoToTransferPlan reconstructs a usecase plan from its wire form.
func dtoToTransferPlan(dto TransferPlanDTO) *usecase.TransferPlan {
	plan := &usecase.TransferPlan{Kind: dto.Kind, OpID: dto.OpID, DestDir: dto.DestDir, Dirs: dto.Dirs}
	plan.Files = make([]usecase.PlannedFile, 0, len(dto.Files))
	for _, fd := range dto.Files {
		f := usecase.PlannedFile{
			Source:     fd.Source,
			Target:     fd.Target,
			Size:       fd.Size,
			SrcModTime: parseModTime(fd.SrcModTime),
		}
		if fd.Conflict != nil {
			f.Conflict = &domain.FileStat{
				Exists:  true,
				IsDir:   fd.Conflict.IsDir,
				Size:    fd.Conflict.Size,
				ModTime: parseModTime(fd.Conflict.ModTime),
			}
		}
		plan.Files = append(plan.Files, f)
	}
	return plan
}

// dtoToResolutions maps resolution DTOs to a target->action map. Unknown action
// names are dropped, which the executor treats as an unresolved conflict (skip)
// — it never silently overwrites.
func dtoToResolutions(items []ResolvedActionDTO) map[string]usecase.ResolvedAction {
	out := make(map[string]usecase.ResolvedAction, len(items))
	for _, it := range items {
		action, ok := domain.ParseConflictAction(it.Action)
		if !ok || action == domain.ConflictAsk {
			continue
		}
		out[it.Target] = usecase.ResolvedAction{Action: action, NewName: it.NewName}
	}
	return out
}
