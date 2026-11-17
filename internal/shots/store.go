// Package shots keeps the screenshot collection on disk: the PNG files in
// day folders, the kept folder, thumbnails, and an index with metadata.
package shots

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Choo-AI-Dev/screenmit/internal/imgclip"
)

// KeptDir holds pinned screenshots; files there are never deleted by age.
const KeptDir = "보관함"

const (
	metaDir   = ".screenmit"
	indexName = "index.json"
	dayLayout = "2006-01-02"
)

var (
	dayDirRe    = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	fileTimeRe  = regexp.MustCompile(`(\d{4}-\d{2}-\d{2})_(\d{2})-(\d{2})-(\d{2})`)
	ErrNotFound = errors.New("not found")
)

// Shot is one screenshot.
type Shot struct {
	ID     string    `json:"id"`
	File   string    `json:"file,omitempty"` // relative to the root, "/" separated; empty for another PC's shot
	Time   time.Time `json:"time"`
	W      int       `json:"w"`
	H      int       `json:"h"`
	Bytes  int64     `json:"bytes"`
	App    string    `json:"app,omitempty"`
	Exe    string    `json:"exe,omitempty"`
	Title  string    `json:"title,omitempty"`
	Note   string    `json:"note,omitempty"`
	Pinned bool      `json:"pinned,omitempty"`
	PC     string    `json:"pc,omitempty"`     // name of the PC that took it
	Remote bool      `json:"remote,omitempty"` // taken on another PC (lives on Drive)

	DriveID string `json:"driveId,omitempty"`
	ThumbID string `json:"thumbId,omitempty"`
	Account string `json:"account,omitempty"` // Drive account the ids belong to
	Updated int64  `json:"updated,omitempty"` // unix ms of the last pin/note change
	Dirty   bool   `json:"dirty,omitempty"`   // pin/note change not yet sent to Drive
}

// Tombstone is a Drive file to delete because the shot was deleted here.
type Tombstone struct {
	ID      string `json:"id"`
	DriveID string `json:"driveId"`
	ThumbID string `json:"thumbId,omitempty"`
	Account string `json:"account"`
}

// Meta describes a new capture.
type Meta struct {
	Time  time.Time
	App   string
	Exe   string
	Title string
	PC    string
}

type indexFile struct {
	Version    int         `json:"version"`
	Shots      []*Shot     `json:"shots"`
	Tombstones []Tombstone `json:"tombstones,omitempty"`
}

// Store is safe for concurrent use.
type Store struct {
	root string

	mu    sync.Mutex
	shots map[string]*Shot
	tombs []Tombstone

	// Now is the clock (tests replace it).
	Now func() time.Time
	// OnChange is called (outside the lock) after the collection changes.
	OnChange func()

	saveMu    sync.Mutex
	saveTimer *time.Timer
}

// Open loads (or creates) the collection at root and reconciles the index
// with the files actually on disk.
func Open(root string) (*Store, error) {
	s := &Store{root: root, shots: map[string]*Shot{}, Now: time.Now}
	for _, d := range []string{root, s.meta(), s.meta("thumbs"), s.meta("remote")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	hideDir(s.meta())
	if b, err := os.ReadFile(s.meta(indexName)); err == nil {
		var ix indexFile
		if err := json.Unmarshal(b, &ix); err != nil {
			log.Printf("index unreadable, rebuilding: %v", err)
			_ = os.Rename(s.meta(indexName), s.meta(indexName+".broken"))
		} else {
			for _, sh := range ix.Shots {
				if sh != nil && sh.ID != "" {
					s.shots[sh.ID] = sh
				}
			}
			s.tombs = ix.Tombstones
		}
	}
	if err := s.rescan(); err != nil {
		return nil, err
	}
	if err := s.saveNow(); err != nil {
		return nil, err
	}
	return s, nil
}

// Root is the collection folder.
func (s *Store) Root() string { return s.root }

func (s *Store) meta(p ...string) string {
	return filepath.Join(append([]string{s.root, metaDir}, p...)...)
}

// ---------- adding ----------

// HashImage is the identity of a picture: the same pixels give the same id.
func HashImage(img image.Image) (string, *image.NRGBA) {
	n := imgclip.ToNRGBA(img)
	h := sha256.New()
	var hdr [8]byte
	binary.LittleEndian.PutUint32(hdr[0:], uint32(n.Rect.Dx()))
	binary.LittleEndian.PutUint32(hdr[4:], uint32(n.Rect.Dy()))
	h.Write(hdr[:])
	h.Write(n.Pix)
	return hex.EncodeToString(h.Sum(nil))[:16], n
}

// Add saves a capture. pngData may be nil (then img is encoded). It returns
// the shot and whether it was new (false when the same picture exists).
func (s *Store) Add(pngData []byte, img image.Image, m Meta) (Shot, bool, error) {
	id, n := HashImage(img)
	if sh, ok := s.Get(id); ok {
		return sh, false, nil
	}
	if pngData == nil {
		var buf bytes.Buffer
		enc := png.Encoder{CompressionLevel: png.DefaultCompression}
		if err := enc.Encode(&buf, n); err != nil {
			return Shot{}, false, err
		}
		pngData = buf.Bytes()
	}
	thumb, err := MakeThumb(n)
	if err != nil {
		return Shot{}, false, err
	}
	if m.Time.IsZero() {
		m.Time = s.Now()
	}

	s.mu.Lock()
	if sh, ok := s.shots[id]; ok {
		s.mu.Unlock()
		return *sh, false, nil
	}
	day := m.Time.Local().Format(dayLayout)
	rel, err := s.freeName(day, m.Time.Local())
	if err == nil {
		err = writeAtomic(filepath.Join(s.root, filepath.FromSlash(rel)), pngData)
	}
	if err != nil {
		s.mu.Unlock()
		return Shot{}, false, err
	}
	_ = writeAtomic(s.ThumbPath(id), thumb)
	sh := &Shot{
		ID: id, File: rel, Time: m.Time, W: n.Rect.Dx(), H: n.Rect.Dy(),
		Bytes: int64(len(pngData)), App: m.App, Exe: m.Exe, Title: m.Title, PC: m.PC,
	}
	s.shots[id] = sh
	out := *sh
	s.mu.Unlock()
	s.changed()
	return out, true, nil
}

// freeName picks "<day>/Screenmit_<day>_HH-MM-SS[_n].png". Caller holds mu.
func (s *Store) freeName(day string, t time.Time) (string, error) {
	if err := os.MkdirAll(filepath.Join(s.root, day), 0o755); err != nil {
		return "", err
	}
	base := "Screenmit_" + day + "_" + t.Format("15-04-05")
	for i := 1; i < 1000; i++ {
		name := base + ".png"
		if i > 1 {
			name = fmt.Sprintf("%s_%d.png", base, i)
		}
		rel := day + "/" + name
		if _, err := os.Stat(filepath.Join(s.root, day, name)); errors.Is(err, fs.ErrNotExist) && !s.fileTaken(rel) {
			return rel, nil
		}
	}
	return "", errors.New("no free file name")
}

func (s *Store) fileTaken(rel string) bool {
	for _, sh := range s.shots {
		if sh.File == rel {
			return true
		}
	}
	return false
}

// AddRemote records a shot from another PC with its thumbnail.
func (s *Store) AddRemote(sh Shot, thumb []byte) error {
	if sh.ID == "" {
		return errors.New("empty id")
	}
	if len(thumb) > 0 {
		if err := writeAtomic(s.ThumbPath(sh.ID), thumb); err != nil {
			return err
		}
	}
	s.mu.Lock()
	if _, ok := s.shots[sh.ID]; ok {
		s.mu.Unlock()
		return nil
	}
	sh.Remote = true
	sh.File = ""
	c := sh
	s.shots[sh.ID] = &c
	s.mu.Unlock()
	s.changed()
	return nil
}

// ---------- reading ----------

// List returns all shots, newest first.
func (s *Store) List() []Shot {
	s.mu.Lock()
	out := make([]Shot, 0, len(s.shots))
	for _, sh := range s.shots {
		out = append(out, *sh)
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Time.Equal(out[j].Time) {
			return out[i].Time.After(out[j].Time)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Get returns one shot.
func (s *Store) Get(id string) (Shot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sh, ok := s.shots[id]
	if !ok {
		return Shot{}, false
	}
	return *sh, true
}

// Tombstones returns Drive files waiting to be deleted.
func (s *Store) Tombstones() []Tombstone {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Tombstone(nil), s.tombs...)
}

// DropTombstone forgets a tombstone once Drive has deleted it.
func (s *Store) DropTombstone(driveID string) {
	s.mu.Lock()
	out := s.tombs[:0]
	for _, t := range s.tombs {
		if t.DriveID != driveID {
			out = append(out, t)
		}
	}
	s.tombs = out
	s.mu.Unlock()
	s.scheduleSave()
}

// ImagePath is the local file for a shot: the PNG for our own shots, the
// downloaded copy for another PC's (which may not exist yet).
func (s *Store) ImagePath(sh Shot) string {
	if sh.File != "" {
		return filepath.Join(s.root, filepath.FromSlash(sh.File))
	}
	return s.CachePath(sh.ID)
}

// CachePath is where a downloaded copy of another PC's shot goes.
func (s *Store) CachePath(id string) string { return s.meta("remote", safeID(id)+".png") }

// ThumbPath is the JPEG thumbnail.
func (s *Store) ThumbPath(id string) string { return s.meta("thumbs", safeID(id)+".jpg") }

// SaveCache stores a downloaded copy of another PC's shot.
func (s *Store) SaveCache(id string, data []byte) error {
	return writeAtomic(s.CachePath(id), data)
}

// EnsureThumb makes the thumbnail from the image file when it is missing.
func (s *Store) EnsureThumb(id string) (string, error) {
	p := s.ThumbPath(id)
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	sh, ok := s.Get(id)
	if !ok {
		return "", ErrNotFound
	}
	b, err := os.ReadFile(s.ImagePath(sh))
	if err != nil {
		return "", err
	}
	img, _, err := imgclip.DecodeImage(b)
	if err != nil {
		return "", err
	}
	t, err := MakeThumb(img)
	if err != nil {
		return "", err
	}
	return p, writeAtomic(p, t)
}

// Stats counts shots and bytes. Local is the space used on this PC.
func (s *Store) Stats() (count int, bytes, local int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sh := range s.shots {
		count++
		bytes += sh.Bytes
		if !sh.Remote {
			local += sh.Bytes
		}
	}
	return
}

// ---------- changing ----------

// Update edits a shot's sync fields (Drive ids, merged pin/note…). fn must
// not touch File; use SetPinned to move files.
func (s *Store) Update(id string, fn func(*Shot)) error {
	s.mu.Lock()
	sh, ok := s.shots[id]
	if !ok {
		s.mu.Unlock()
		return ErrNotFound
	}
	file := sh.File
	fn(sh)
	sh.File = file
	s.mu.Unlock()
	s.changed()
	return nil
}

// SetPinned keeps (or releases) a shot. Our own files move between the day
// folder and KeptDir. byUser marks the change for Drive.
func (s *Store) SetPinned(id string, pinned, byUser bool) error {
	s.mu.Lock()
	sh, ok := s.shots[id]
	if !ok {
		s.mu.Unlock()
		return ErrNotFound
	}
	if sh.Pinned == pinned {
		s.mu.Unlock()
		return nil
	}
	if sh.File != "" {
		name := filepath.Base(filepath.FromSlash(sh.File))
		dir := KeptDir
		if !pinned {
			dir = sh.Time.Local().Format(dayLayout)
		}
		if err := os.MkdirAll(filepath.Join(s.root, dir), 0o755); err != nil {
			s.mu.Unlock()
			return err
		}
		rel := dir + "/" + name
		for i := 2; s.exists(rel); i++ {
			ext := filepath.Ext(name)
			rel = fmt.Sprintf("%s/%s_%d%s", dir, strings.TrimSuffix(name, ext), i, ext)
		}
		if err := os.Rename(filepath.Join(s.root, filepath.FromSlash(sh.File)), filepath.Join(s.root, filepath.FromSlash(rel))); err != nil {
			s.mu.Unlock()
			return err
		}
		old := sh.File
		sh.File = rel
		s.removeEmptyDir(filepath.Dir(filepath.Join(s.root, filepath.FromSlash(old))))
	}
	sh.Pinned = pinned
	if byUser {
		sh.Updated = s.Now().UnixMilli()
		sh.Dirty = true
	}
	s.mu.Unlock()
	s.changed()
	return nil
}

func (s *Store) exists(rel string) bool {
	_, err := os.Stat(filepath.Join(s.root, filepath.FromSlash(rel)))
	return err == nil
}

// SetNote changes the memo.
func (s *Store) SetNote(id, note string, byUser bool) error {
	note = strings.TrimSpace(note)
	return s.Update(id, func(sh *Shot) {
		if sh.Note == note {
			return
		}
		sh.Note = note
		if byUser {
			sh.Updated = s.Now().UnixMilli()
			sh.Dirty = true
		}
	})
}

// Delete removes a shot because the user asked. If it is on Drive, a
// tombstone is left so the sync deletes it there too.
func (s *Store) Delete(id string) error {
	return s.remove(id, true)
}

// RemoveLocal forgets a shot that was deleted on another PC.
func (s *Store) RemoveLocal(id string) error {
	return s.remove(id, false)
}

func (s *Store) remove(id string, tomb bool) error {
	s.mu.Lock()
	sh, ok := s.shots[id]
	if !ok {
		s.mu.Unlock()
		return ErrNotFound
	}
	s.dropFiles(sh)
	delete(s.shots, id)
	if tomb && sh.DriveID != "" {
		s.tombs = append(s.tombs, Tombstone{ID: sh.ID, DriveID: sh.DriveID, ThumbID: sh.ThumbID, Account: sh.Account})
	}
	s.mu.Unlock()
	s.changed()
	return nil
}

// dropFiles deletes a shot's files. Caller holds mu.
func (s *Store) dropFiles(sh *Shot) {
	if sh.File != "" {
		p := filepath.Join(s.root, filepath.FromSlash(sh.File))
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			log.Printf("delete %s: %v", p, err)
		}
		s.removeEmptyDir(filepath.Dir(p))
	}
	_ = os.Remove(s.ThumbPath(sh.ID))
	_ = os.Remove(s.CachePath(sh.ID))
}

// removeEmptyDir deletes an empty day folder (never the root or KeptDir).
func (s *Store) removeEmptyDir(dir string) {
	if !dayDirRe.MatchString(filepath.Base(dir)) || filepath.Dir(dir) != filepath.Clean(s.root) {
		return
	}
	if ents, err := os.ReadDir(dir); err == nil && len(ents) == 0 {
		_ = os.Remove(dir)
	}
}

// Expired reports whether a shot from t is past a retention of days:
// a shot from day D is kept through day D+days-1.
func Expired(t, now time.Time, days int) bool {
	if days <= 0 {
		return false
	}
	return DaysLeft(t, now, days) <= 0
}

// DaysLeft is how many days (including today) a shot from t still has.
func DaysLeft(t, now time.Time, days int) int {
	d := civilDay(t.Local())
	today := civilDay(now.Local())
	return days - int(today.Sub(d).Hours()/24+0.5)
}

func civilDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// Cleanup deletes shots past the retention that are not kept. It returns
// how many were removed. Drive copies are removed by the sync, by age.
func (s *Store) Cleanup(days int) int {
	now := s.Now()
	s.mu.Lock()
	n := 0
	for id, sh := range s.shots {
		if sh.Pinned || !Expired(sh.Time, now, days) {
			continue
		}
		s.dropFiles(sh)
		delete(s.shots, id)
		n++
	}
	s.mu.Unlock()
	// Files in day folders that are not in the index (copied in by hand are
	// picked up by rescan, so these are leftovers like temp files): leave them.
	if n > 0 {
		s.changed()
	}
	return n
}

// ForgetAccount clears Drive ids that belong to another account, so a
// switch of account uploads again instead of deleting anything.
func (s *Store) ForgetAccount(keep string) {
	s.mu.Lock()
	changed := false
	for id, sh := range s.shots {
		if sh.DriveID != "" && sh.Account != keep {
			if sh.Remote {
				s.dropFiles(sh)
				delete(s.shots, id)
			} else {
				sh.DriveID, sh.ThumbID, sh.Account = "", "", ""
			}
			changed = true
		}
	}
	out := s.tombs[:0]
	for _, t := range s.tombs {
		if t.Account == keep {
			out = append(out, t)
		}
	}
	s.tombs = out
	s.mu.Unlock()
	if changed {
		s.changed()
	}
}

// ---------- disk reconciliation ----------

// rescan makes the index match the folders: new PNGs become shots, missing
// files drop out, and location decides "kept".
func (s *Store) rescan() error {
	onDisk := map[string]bool{}
	ents, err := os.ReadDir(s.root)
	if err != nil {
		return err
	}
	for _, e := range ents {
		if !e.IsDir() || !(dayDirRe.MatchString(e.Name()) || e.Name() == KeptDir) {
			continue
		}
		files, err := os.ReadDir(filepath.Join(s.root, e.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.EqualFold(filepath.Ext(f.Name()), ".png") {
				continue
			}
			onDisk[e.Name()+"/"+f.Name()] = true
		}
	}

	known := map[string]*Shot{}
	for id, sh := range s.shots {
		if sh.Remote {
			continue
		}
		if !onDisk[sh.File] {
			log.Printf("file gone, dropping %s (%s)", id, sh.File)
			_ = os.Remove(s.ThumbPath(id))
			if sh.DriveID != "" {
				s.tombs = append(s.tombs, Tombstone{ID: id, DriveID: sh.DriveID, ThumbID: sh.ThumbID, Account: sh.Account})
			}
			delete(s.shots, id)
			continue
		}
		known[sh.File] = sh
		kept := strings.HasPrefix(sh.File, KeptDir+"/")
		if kept != sh.Pinned {
			sh.Pinned = kept
			sh.Updated = s.Now().UnixMilli()
			sh.Dirty = true
		}
	}

	for rel := range onDisk {
		if known[rel] != nil {
			continue
		}
		p := filepath.Join(s.root, filepath.FromSlash(rel))
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			log.Printf("skip unreadable %s: %v", rel, err)
			continue
		}
		id, n := HashImage(img)
		if other, dup := s.shots[id]; dup {
			if other.Remote {
				// We have the original of a shot we only knew from Drive.
				other.Remote, other.File = false, rel
			}
			continue
		}
		t := timeFromName(filepath.Base(p))
		if t.IsZero() {
			if st, err := os.Stat(p); err == nil {
				t = st.ModTime()
			}
		}
		s.shots[id] = &Shot{
			ID: id, File: rel, Time: t, W: n.Rect.Dx(), H: n.Rect.Dy(), Bytes: int64(len(b)),
			Pinned: strings.HasPrefix(rel, KeptDir+"/"),
		}
	}
	return nil
}

func timeFromName(name string) time.Time {
	m := fileTimeRe.FindStringSubmatch(name)
	if m == nil {
		return time.Time{}
	}
	t, err := time.ParseInLocation("2006-01-02 15 04 05", m[1]+" "+m[2]+" "+m[3]+" "+m[4], time.Local)
	if err != nil {
		return time.Time{}
	}
	return t
}

// ---------- saving ----------

func (s *Store) changed() {
	s.scheduleSave()
	if s.OnChange != nil {
		s.OnChange()
	}
}

func (s *Store) scheduleSave() {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	if s.saveTimer != nil {
		s.saveTimer.Stop()
	}
	s.saveTimer = time.AfterFunc(400*time.Millisecond, func() {
		if err := s.saveNow(); err != nil {
			log.Printf("save index: %v", err)
		}
	})
}

// Flush writes the index now (call before exit).
func (s *Store) Flush() error {
	s.saveMu.Lock()
	if s.saveTimer != nil {
		s.saveTimer.Stop()
		s.saveTimer = nil
	}
	s.saveMu.Unlock()
	return s.saveNow()
}

func (s *Store) saveNow() error {
	s.mu.Lock()
	ix := indexFile{Version: 1, Tombstones: append([]Tombstone(nil), s.tombs...)}
	for _, sh := range s.shots {
		c := *sh
		ix.Shots = append(ix.Shots, &c)
	}
	s.mu.Unlock()
	sort.Slice(ix.Shots, func(i, j int) bool { return ix.Shots[i].Time.After(ix.Shots[j].Time) })
	b, err := json.MarshalIndent(ix, "", " ")
	if err != nil {
		return err
	}
	return writeAtomic(s.meta(indexName), b)
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}

func safeID(id string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, id)
}
