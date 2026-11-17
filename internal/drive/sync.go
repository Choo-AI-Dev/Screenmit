package drive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Choo-AI-Dev/screenmit/internal/shots"
)

// Layout on Drive (shared with the web viewer):
//
//	Screenmit/                     folder, appProperties screenmit=root
//	  settings.json                screenmit=settings  {"retentionDays":14,"updatedAt":ms}
//	  Screenmit_<day>_<time>.png   screenmit=shot, id, pc, t, w, h, app, exe, pin, u, thumb
//	                               description = {"title":…, "note":…}
//	  .thumbs/                     screenmit=thumbs
//	    <id>.jpg                   screenmit=thumb, id
const (
	KeyKind     = "screenmit"
	RootName    = "Screenmit"
	ThumbsName  = ".thumbs"
	SettingsMax = 4096
)

// Settings is the part of the configuration all PCs share.
type Settings struct {
	RetentionDays int   `json:"retentionDays"`
	UpdatedAt     int64 `json:"updatedAt"`
}

// Status is shown in the gallery.
type Status struct {
	Connected bool      `json:"connected"`
	Email     string    `json:"email,omitempty"`
	Syncing   bool      `json:"syncing"`
	LastSync  time.Time `json:"lastSync,omitempty"`
	Error     string    `json:"error,omitempty"`
	NeedLogin bool      `json:"needLogin,omitempty"`
	Pending   int       `json:"pending"` // own shots not on Drive yet
	PCs       []string  `json:"pcs,omitempty"`
}

// Syncer keeps the local store and Drive in step.
type Syncer struct {
	API   *API
	Store func() *shots.Store
	PC    func() string
	// Retention returns the local setting; Adopt is called when another PC
	// changed it more recently.
	Retention func() Settings
	Adopt     func(Settings)
	OnStatus  func()
	Now       func() time.Time

	// Batch limits per round, so one round never blocks for long.
	MaxUploads, MaxThumbs int

	kick chan struct{}

	mu      sync.Mutex
	status  Status
	root    string
	thumbs  string
	setting string
}

// NewSyncer wires a syncer; call Run in a goroutine.
func NewSyncer(api *API) *Syncer {
	return &Syncer{API: api, kick: make(chan struct{}, 1), Now: time.Now, MaxUploads: 12, MaxThumbs: 60}
}

// Kick asks for a sync soon.
func (s *Syncer) Kick() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

// Status returns a copy.
func (s *Syncer) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.status
	st.PCs = append([]string(nil), st.PCs...)
	return st
}

func (s *Syncer) setStatus(fn func(*Status)) {
	s.mu.Lock()
	fn(&s.status)
	s.mu.Unlock()
	if s.OnStatus != nil {
		s.OnStatus()
	}
}

// Run syncs until ctx ends: right away, on Kick, and every interval().
func (s *Syncer) Run(ctx context.Context, interval func() time.Duration) {
	for {
		more, err := s.SyncOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		wait := interval()
		if more && err == nil {
			wait = time.Second
		}
		if err != nil {
			wait = min(wait*2, 5*time.Minute)
		}
		select {
		case <-ctx.Done():
			return
		case <-s.kick:
			// Gather a burst of kicks (several captures in a row).
			select {
			case <-time.After(1500 * time.Millisecond):
			case <-ctx.Done():
				return
			}
		case <-time.After(wait):
		}
	}
}

// AccountKey ties Drive ids to one account and one root folder.
func (s *Syncer) accountKey(email string) string { return email + "|" + s.folder() }

// SyncOnce does one round. more is true when work is left for another round.
func (s *Syncer) SyncOnce(ctx context.Context) (more bool, err error) {
	s.setStatus(func(st *Status) { st.Syncing = true })
	defer func() {
		s.setStatus(func(st *Status) {
			st.Syncing = false
			if err != nil {
				st.Error = friendly(err)
				st.NeedLogin = errors.Is(err, ErrNeedLogin)
				if st.NeedLogin {
					st.Connected = false
				}
			} else {
				st.Error, st.NeedLogin, st.LastSync = "", false, s.Now()
			}
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("sync: %v", err)
		}
		if IsNotFound(err) {
			// The Screenmit folder was deleted or moved: find or make it again.
			s.ResetFolders()
		}
	}()

	email := s.API.Email()
	if email == "" {
		if email, err = s.API.About(ctx); err != nil {
			return false, err
		}
	}
	if err := s.ensureFolders(ctx); err != nil {
		return false, err
	}
	key := s.accountKey(email)
	st := s.Store()
	st.ForgetAccount(key)
	s.setStatus(func(x *Status) { x.Connected, x.Email = true, email })

	if err := s.syncSettings(ctx); err != nil {
		return false, err
	}
	days := s.Retention().RetentionDays
	now := s.Now()

	// 1. Deletions made here.
	for _, t := range st.Tombstones() {
		if err := s.API.Delete(ctx, t.DriveID); err != nil {
			return false, err
		}
		_ = s.API.Delete(ctx, t.ThumbID)
		st.DropTombstone(t.DriveID)
	}

	// 2. What is on Drive. Expired ones are removed by whichever PC sees them.
	files, err := s.API.List(ctx, fmt.Sprintf("'%s' in parents and appProperties has { key='%s' and value='shot' } and trashed = false", s.folder(), KeyKind))
	if err != nil {
		return false, err
	}
	remote := map[string]File{}
	pcs := map[string]bool{}
	for _, f := range files {
		r := parseRemote(f)
		if r.ID == "" {
			continue
		}
		if !r.Pinned && shots.Expired(r.Time, now, days) {
			if err := s.API.Delete(ctx, f.ID); err != nil {
				return false, err
			}
			_ = s.API.Delete(ctx, r.ThumbID)
			continue
		}
		if _, dup := remote[r.ID]; dup {
			// Two PCs uploaded the same picture: keep the first.
			_ = s.API.Delete(ctx, f.ID)
			_ = s.API.Delete(ctx, r.ThumbID)
			continue
		}
		remote[r.ID] = f
		if r.PC != "" {
			pcs[r.PC] = true
		}
	}

	// 3. Our side: link, merge, upload, or notice deletions elsewhere.
	uploads, thumbs := 0, 0
	pending := 0
	for _, sh := range st.List() {
		f, onDrive := remote[sh.ID]
		delete(remote, sh.ID)
		if sh.PC != "" {
			pcs[sh.PC] = true
		}
		switch {
		case sh.DriveID != "" && !onDrive:
			// Missing from the search. Deleted on another PC (or expired)?
			// Ask for the file itself, since search results can lag.
			gone, err := s.API.Gone(ctx, sh.DriveID)
			if err != nil {
				return false, err
			}
			if gone {
				_ = st.RemoveLocal(sh.ID)
			}
		case onDrive:
			if err := s.merge(ctx, st, sh, f, key); err != nil {
				return false, err
			}
		case sh.Remote:
			// Known only from Drive but not there any more.
			_ = st.RemoveLocal(sh.ID)
		case !sh.Pinned && shots.Expired(sh.Time, now, days):
			// About to be cleaned up locally; don't upload.
		default:
			if uploads >= s.MaxUploads {
				pending++
				more = true
				continue
			}
			if err := s.upload(ctx, st, sh, key); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return false, err
			}
			uploads++
		}
	}

	// 4. New from other PCs.
	for id, f := range remote {
		if thumbs >= s.MaxThumbs {
			more = true
			break
		}
		r := parseRemote(f)
		var tb []byte
		if r.ThumbID != "" {
			tb, err = s.API.Download(ctx, r.ThumbID)
			if err != nil && !IsNotFound(err) {
				return false, err
			}
		}
		thumbs++
		r.ID, r.DriveID, r.Account = id, f.ID, key
		if err := st.AddRemote(r, tb); err != nil {
			return false, err
		}
	}

	list := make([]string, 0, len(pcs))
	for p := range pcs {
		list = append(list, p)
	}
	s.setStatus(func(x *Status) { x.Pending, x.PCs = pending, sortedStrings(list) })
	return more, nil
}

func (s *Syncer) merge(ctx context.Context, st *shots.Store, sh shots.Shot, f File, key string) error {
	r := parseRemote(f)
	if sh.DriveID != f.ID || sh.Account != key || sh.ThumbID != r.ThumbID {
		_ = st.Update(sh.ID, func(x *shots.Shot) { x.DriveID, x.ThumbID, x.Account = f.ID, r.ThumbID, key })
	}
	switch {
	case sh.Dirty && sh.Updated >= r.Updated:
		if err := s.API.Patch(ctx, f.ID, map[string]any{
			"appProperties": map[string]string{"pin": boolStr(sh.Pinned), "u": strconv.FormatInt(sh.Updated, 10)},
			"description":   describe(sh.Title, sh.Note),
		}); err != nil {
			return err
		}
		_ = st.Update(sh.ID, func(x *shots.Shot) {
			if x.Updated == sh.Updated {
				x.Dirty = false
			}
		})
	case r.Updated > sh.Updated:
		if err := st.SetPinned(sh.ID, r.Pinned, false); err != nil {
			log.Printf("pin from drive: %v", err)
		}
		_ = st.SetNote(sh.ID, r.Note, false)
		_ = st.Update(sh.ID, func(x *shots.Shot) { x.Updated, x.Dirty = r.Updated, false })
	}
	return nil
}

func (s *Syncer) upload(ctx context.Context, st *shots.Store, sh shots.Shot, key string) error {
	data, err := os.ReadFile(st.ImagePath(sh))
	if err != nil {
		return err
	}
	var thumbID string
	if p, err := st.EnsureThumb(sh.ID); err == nil {
		if tb, err := os.ReadFile(p); err == nil {
			tf, err := s.API.Upload(ctx, File{
				Name: sh.ID + ".jpg", Parents: []string{s.thumbFolder()},
				AppProperties: map[string]string{KeyKind: "thumb", "id": sh.ID},
			}, "image/jpeg", tb)
			if err != nil {
				return err
			}
			thumbID = tf.ID
		}
	}
	pc := sh.PC
	if pc == "" {
		pc = s.PC()
	}
	name := sh.File[strings.LastIndex(sh.File, "/")+1:]
	f, err := s.API.Upload(ctx, File{
		Name:    name,
		Parents: []string{s.folder()},
		AppProperties: map[string]string{
			KeyKind: "shot", "id": sh.ID, "pc": clip(pc, 90), "t": strconv.FormatInt(sh.Time.UnixMilli(), 10),
			"w": strconv.Itoa(sh.W), "h": strconv.Itoa(sh.H), "app": clip(sh.App, 100), "exe": clip(sh.Exe, 100),
			"pin": boolStr(sh.Pinned), "u": strconv.FormatInt(sh.Updated, 10), "thumb": thumbID,
		},
		Description: describe(sh.Title, sh.Note),
	}, "image/png", data)
	if err != nil {
		_ = s.API.Delete(ctx, thumbID)
		return err
	}
	return st.Update(sh.ID, func(x *shots.Shot) {
		x.DriveID, x.ThumbID, x.Account = f.ID, thumbID, key
		if x.PC == "" {
			x.PC = pc
		}
		if x.Updated == sh.Updated {
			x.Dirty = false
		}
	})
}

// Fetch downloads another PC's full picture into the local cache.
func (s *Syncer) Fetch(ctx context.Context, sh shots.Shot) (string, error) {
	st := s.Store()
	p := st.ImagePath(sh)
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	if sh.DriveID == "" {
		return "", shots.ErrNotFound
	}
	b, err := s.API.Download(ctx, sh.DriveID)
	if err != nil {
		return "", err
	}
	if err := st.SaveCache(sh.ID, b); err != nil {
		return "", err
	}
	return st.CachePath(sh.ID), nil
}

// ensureFolders finds or creates Screenmit/ and Screenmit/.thumbs.
func (s *Syncer) ensureFolders(ctx context.Context) error {
	s.mu.Lock()
	have := s.root != "" && s.thumbs != ""
	s.mu.Unlock()
	if have {
		return nil
	}
	roots, err := s.API.List(ctx, fmt.Sprintf("appProperties has { key='%s' and value='root' } and trashed = false", KeyKind))
	if err != nil {
		return err
	}
	var root string
	if len(roots) > 0 {
		root = oldest(roots).ID
	} else {
		f, err := s.API.CreateFolder(ctx, File{Name: RootName, AppProperties: map[string]string{KeyKind: "root"}})
		if err != nil {
			return err
		}
		root = f.ID
	}
	th, err := s.API.List(ctx, fmt.Sprintf("'%s' in parents and appProperties has { key='%s' and value='thumbs' } and trashed = false", root, KeyKind))
	if err != nil {
		return err
	}
	var thumbs string
	if len(th) > 0 {
		thumbs = oldest(th).ID
	} else {
		f, err := s.API.CreateFolder(ctx, File{Name: ThumbsName, Parents: []string{root}, AppProperties: map[string]string{KeyKind: "thumbs"}})
		if err != nil {
			return err
		}
		thumbs = f.ID
	}
	s.mu.Lock()
	s.root, s.thumbs, s.setting = root, thumbs, ""
	s.mu.Unlock()
	return nil
}

func (s *Syncer) folder() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.root
}

func (s *Syncer) thumbFolder() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.thumbs
}

func (s *Syncer) settingID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.setting
}

func (s *Syncer) setSettingID(id string) {
	s.mu.Lock()
	s.setting = id
	s.mu.Unlock()
}

// ResetFolders forgets the cached folder ids (after a 404, or a new login).
func (s *Syncer) ResetFolders() {
	s.mu.Lock()
	s.root, s.thumbs, s.setting = "", "", ""
	s.mu.Unlock()
}

func (s *Syncer) syncSettings(ctx context.Context) error {
	local := s.Retention()
	root, id := s.folder(), s.settingID()
	if id == "" {
		fs, err := s.API.List(ctx, fmt.Sprintf("'%s' in parents and appProperties has { key='%s' and value='settings' } and trashed = false", root, KeyKind))
		if err != nil {
			return err
		}
		if len(fs) > 0 {
			id = oldest(fs).ID
		}
	}
	if id == "" {
		b, _ := json.Marshal(local)
		f, err := s.API.Upload(ctx, File{Name: "settings.json", Parents: []string{root}, AppProperties: map[string]string{KeyKind: "settings"}}, "application/json", b)
		if err != nil {
			return err
		}
		s.setSettingID(f.ID)
		return nil
	}
	s.setSettingID(id)
	b, err := s.API.Download(ctx, id)
	if err != nil {
		if IsNotFound(err) {
			s.setSettingID("")
			return nil
		}
		return err
	}
	var remote Settings
	_ = json.Unmarshal(b, &remote)
	switch {
	case remote.UpdatedAt > local.UpdatedAt && remote.RetentionDays > 0:
		if remote.RetentionDays != local.RetentionDays || remote.UpdatedAt != local.UpdatedAt {
			s.Adopt(remote)
		}
	case local.UpdatedAt > remote.UpdatedAt || remote.RetentionDays <= 0:
		nb, _ := json.Marshal(local)
		return s.API.UpdateContent(ctx, id, "application/json", nb)
	}
	return nil
}

// parseRemote turns Drive metadata into a Shot (ID from appProperties).
func parseRemote(f File) shots.Shot {
	p := f.AppProperties
	ms, _ := strconv.ParseInt(p["t"], 10, 64)
	u, _ := strconv.ParseInt(p["u"], 10, 64)
	w, _ := strconv.Atoi(p["w"])
	h, _ := strconv.Atoi(p["h"])
	size, _ := strconv.ParseInt(f.Size, 10, 64)
	var d struct {
		Title string `json:"title"`
		Note  string `json:"note"`
	}
	_ = json.Unmarshal([]byte(f.Description), &d)
	return shots.Shot{
		ID: p["id"], Time: time.UnixMilli(ms), W: w, H: h, Bytes: size,
		App: p["app"], Exe: p["exe"], PC: p["pc"], Pinned: p["pin"] == "1",
		Updated: u, ThumbID: p["thumb"], DriveID: f.ID, Title: d.Title, Note: d.Note,
	}
}

func describe(title, note string) string {
	b, _ := json.Marshal(map[string]string{"title": title, "note": note})
	return string(b)
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// clip shortens s to n bytes on a rune boundary (Drive allows 124 bytes
// for key + value of each appProperty).
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func oldest(fs []File) File {
	o := fs[0]
	for _, f := range fs[1:] {
		if f.CreatedTime < o.CreatedTime {
			o = f
		}
	}
	return o
}

func sortedStrings(a []string) []string {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
	return a
}

func friendly(err error) string {
	var h *HTTPError
	switch {
	case errors.Is(err, ErrNeedLogin):
		return "구글 로그인이 필요해요"
	case errors.As(err, &h) && h.Status == 403 && strings.Contains(h.Body, "storageQuotaExceeded"):
		return "구글 드라이브 저장 공간이 가득 찼어요"
	case errors.As(err, &h) && h.Status == 403 && strings.Contains(h.Body, "accessNotConfigured"):
		return "Google Cloud 에서 Google Drive API 를 사용 설정해 주세요"
	case errors.As(err, &h):
		return fmt.Sprintf("드라이브 오류 (%d)", h.Status)
	case strings.Contains(err.Error(), "dial") || strings.Contains(err.Error(), "no such host") || strings.Contains(err.Error(), "timeout"):
		return "인터넷에 연결되지 않았어요"
	}
	return err.Error()
}
