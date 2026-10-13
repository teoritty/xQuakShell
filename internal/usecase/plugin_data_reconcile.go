package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"slices"
	"strings"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
)

// syncedFileMode is the mode a synced file is mirrored with. A walk reports no mode, and a synced
// file is never executable whatever it was on the device that wrote it.
const syncedFileMode = 0o600

type diskFile struct {
	digest  string
	content []byte
}

// pluginPass is one reconcile of one plugin: the disk as scanned, the mirror and the device-local
// record of what was last on disk, and what the pass decided.
type pluginPass struct {
	m        domainplugin.Manifest
	dir      string
	disk     map[string]diskFile
	skipped  map[string]bool
	mirror   map[string]domain.SyncPluginFile
	last     map[string]string
	report   PluginDataReport
	pushes   map[string]*diskFile // nil value: deleted here
	recorded map[string]string    // path -> digest now on disk, "" when gone
}

// reconcileLocked runs one pass with the plugin's lock held.
func (s *PluginDataSync) reconcileLocked(ctx context.Context, pluginID string, mayWrite bool) (PluginDataReport, error) {
	m, ok := s.manifest(pluginID)
	if !ok {
		return PluginDataReport{Plugin: pluginID}, nil
	}
	data, err := s.d.Vault.GetData()
	if err != nil {
		return PluginDataReport{Plugin: pluginID}, err
	}
	if data.Sync == nil || data.Sync.Account == nil || slices.Contains(data.Sync.PluginDataOff, pluginID) {
		return PluginDataReport{Plugin: pluginID}, nil
	}
	p := &pluginPass{m: m, dir: s.d.DataDir(pluginID), skipped: map[string]bool{}, mirror: map[string]domain.SyncPluginFile{},
		last: map[string]string{}, report: PluginDataReport{Plugin: pluginID}, pushes: map[string]*diskFile{}, recorded: map[string]string{}}
	if err := s.scan(p); err != nil {
		return p.report, err
	}
	p.load(data.Sync)
	s.decide(p, mayWrite)
	if err := s.commit(ctx, p); err != nil {
		return p.report, err
	}
	s.setWaiting(pluginID, p.report.Waiting)
	if s.d.Reported != nil && (len(p.report.Conflicts) > 0 || len(p.report.Skipped) > 0) {
		s.d.Reported(p.report)
	}
	return p.report, nil
}

func (s *PluginDataSync) manifest(pluginID string) (domainplugin.Manifest, bool) {
	for _, m := range s.d.Catalog.SyncedManifests() {
		if m.ID == pluginID && m.Sync != nil && len(m.Sync.Paths) > 0 {
			return m, true
		}
	}
	return domainplugin.Manifest{}, false
}

// scan reads the declared files in path order, so the files the per-plugin limits leave out are the
// same ones on every pass.
func (s *PluginDataSync) scan(p *pluginPass) error {
	type found struct {
		rel  string
		size int64
	}
	var files []found
	err := s.d.Zone.Walk(p.dir, func(rel string, size int64) error {
		if p.m.Sync.Matches(rel) {
			files = append(files, found{rel, size})
		}
		return nil
	})
	if err != nil {
		return err
	}
	slices.SortFunc(files, func(a, b found) int { return strings.Compare(a.rel, b.rel) })
	p.disk = map[string]diskFile{}
	var total int64
	for _, f := range files {
		if f.size > domainplugin.MaxSyncFileBytes || len(p.disk) >= domainplugin.MaxSyncPluginFiles ||
			total+f.size > domainplugin.MaxSyncPluginBytes {
			p.skip(f.rel)
			continue
		}
		content, err := s.d.Zone.ReadRegularFile(p.dir+"/"+f.rel, domainplugin.MaxSyncFileBytes)
		if err != nil {
			// The file changed or became a link between the walk and the read; the next pass sees it.
			p.skip(f.rel)
			continue
		}
		total += int64(len(content))
		p.disk[f.rel] = diskFile{digest: digestHex(content), content: content}
	}
	return nil
}

func (p *pluginPass) skip(rel string) {
	if !p.skipped[rel] {
		p.skipped[rel] = true
		p.report.Skipped = append(p.report.Skipped, rel)
	}
}

func (p *pluginPass) load(vs *domain.VaultSync) {
	for _, f := range vs.PluginFiles {
		if f.Plugin == p.m.ID {
			p.mirror[f.Path] = f
		}
	}
	for _, f := range p.mirror {
		if d, ok := vs.PluginFilesOnDisk[f.Key()]; ok {
			p.last[f.Path] = d
		}
	}
	prefix := p.m.ID + "\n"
	for k, d := range vs.PluginFilesOnDisk {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			p.last[k[len(prefix):]] = d
		}
	}
}

// decide plans every file and writes the incoming ones the plan allows. A file this device's
// manifest does not declare is never written, whatever the mirror holds, and never pushed either:
// a path dropped from this device's manifest must not read as a deletion everywhere else.
func (s *PluginDataSync) decide(p *pluginPass, mayWrite bool) {
	for _, rel := range p.paths() {
		if p.skipped[rel] {
			continue
		}
		disk, mirror := p.disk[rel].digest, p.mirror[rel].SHA256
		action, conflict := planPluginFile(disk, mirror, p.last[rel], mayWrite)
		if conflict {
			p.report.Conflicts = append(p.report.Conflicts, rel)
		}
		switch {
		case action != actionNone && !p.m.Sync.Matches(rel):
			p.skip(rel)
		case action == actionPush:
			p.push(rel)
		case action == actionWait:
			p.report.Waiting++
		case action == actionWrite:
			if err := s.write(p, rel); err != nil {
				p.skip(rel)
				continue
			}
			p.report.Applied++
		}
	}
}

func (p *pluginPass) push(rel string) {
	f, ok := p.disk[rel]
	if ok {
		p.pushes[rel] = &f
	} else {
		p.pushes[rel] = nil
	}
	p.recorded[rel] = f.digest
	p.report.Pushed++
}

func (s *PluginDataSync) write(p *pluginPass, rel string) error {
	path := p.dir + "/" + rel
	f, ok := p.mirror[rel]
	if !ok {
		if err := s.d.Zone.RemoveFile(path); err != nil {
			return err
		}
		p.recorded[rel] = ""
		return nil
	}
	if err := s.d.Zone.WriteFileAtomic(path, f.Content, fs.FileMode(f.Mode&0o644)); err != nil {
		return err
	}
	p.recorded[rel] = f.SHA256
	return nil
}

// paths lists every file the disk, the mirror or the last record knows, in order.
func (p *pluginPass) paths() []string {
	set := map[string]bool{}
	for rel := range p.disk {
		set[rel] = true
	}
	for rel := range p.mirror {
		set[rel] = true
	}
	for rel := range p.last {
		set[rel] = true
	}
	out := make([]string, 0, len(set))
	for rel := range set {
		out = append(out, rel)
	}
	slices.Sort(out)
	return out
}

// commit records the pass in the vault: pushed files into the mirror, and what is now on disk into
// the device-local record. A pass that changed nothing writes nothing.
func (s *PluginDataSync) commit(ctx context.Context, p *pluginPass) error {
	if len(p.recorded) == 0 {
		return nil
	}
	err := s.d.Vault.UpdateData(ctx, func(d *domain.VaultData) error {
		if d.Sync == nil {
			return errors.New("plugin data: the vault stopped syncing")
		}
		if d.Sync.PluginFilesOnDisk == nil {
			d.Sync.PluginFilesOnDisk = map[string]string{}
		}
		for rel, digest := range p.recorded {
			key := domain.SyncPluginFile{Plugin: p.m.ID, Path: rel}.Key()
			if digest == "" {
				delete(d.Sync.PluginFilesOnDisk, key)
			} else {
				d.Sync.PluginFilesOnDisk[key] = digest
			}
		}
		for rel, f := range p.pushes {
			key := domain.SyncPluginFile{Plugin: p.m.ID, Path: rel}.Key()
			d.Sync.PluginFiles = slices.DeleteFunc(d.Sync.PluginFiles, func(x domain.SyncPluginFile) bool { return x.Key() == key })
			if f != nil {
				d.Sync.PluginFiles = append(d.Sync.PluginFiles, domain.SyncPluginFile{Plugin: p.m.ID, Path: rel, Mode: syncedFileMode,
					SHA256: f.digest, Content: f.content})
			}
		}
		return nil
	})
	if err == nil && len(p.pushes) > 0 && s.d.Changed != nil {
		s.d.Changed()
	}
	return err
}

func digestHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
