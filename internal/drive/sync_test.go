package drive

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Choo-AI-Dev/screenmit/internal/drive/fakedrive"
	"github.com/Choo-AI-Dev/screenmit/internal/shots"
)

type memTokens struct{ t *Token }

func (m *memTokens) Load() (Token, error) {
	if m.t == nil {
		return Token{}, os.ErrNotExist
	}
	return *m.t, nil
}
func (m *memTokens) Save(t Token) error { m.t = &t; return nil }
func (m *memTokens) Clear() error       { m.t = nil; return nil }

type pc struct {
	name  string
	store *shots.Store
	sync  *Syncer
	set   Settings
}

func pic(seed, w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x*seed + y), uint8(y * seed), uint8(seed), 255})
		}
	}
	return img
}

func newPC(t *testing.T, srv *httptest.Server, name string) *pc {
	t.Helper()
	st, err := shots.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	creds := Credentials{ClientID: "cid", ClientSecret: "sec", AuthURL: srv.URL + "/o/oauth2/auth", TokenURL: srv.URL + "/token"}
	tok, err := Login(context.Background(), srv.Client(), creds, func(u string) error {
		resp, err := http.Get(u) // follows the redirect to our loopback listener
		if err == nil {
			resp.Body.Close()
		}
		return err
	})
	if err != nil {
		t.Fatal("login:", err)
	}
	api := NewAPI(srv.Client(), srv.URL, creds, &memTokens{})
	if err := api.SetToken(tok); err != nil {
		t.Fatal(err)
	}
	p := &pc{name: name, store: st, set: Settings{RetentionDays: 14}}
	s := NewSyncer(api)
	s.Store = func() *shots.Store { return p.store }
	s.PC = func() string { return p.name }
	s.Retention = func() Settings { return p.set }
	s.Adopt = func(x Settings) { p.set = x }
	p.sync = s
	return p
}

func (p *pc) syncOK(t *testing.T) {
	t.Helper()
	for i := 0; i < 10; i++ {
		more, err := p.sync.SyncOnce(context.Background())
		if err != nil {
			t.Fatalf("%s sync: %v", p.name, err)
		}
		if !more {
			return
		}
	}
	t.Fatal("sync never finished")
}

func TestTwoPCs(t *testing.T) {
	fd := fakedrive.New()
	fd.PageSize = 2 // exercise paging
	srv := httptest.NewServer(fd)
	defer srv.Close()

	work := newPC(t, srv, "회사 PC")
	home := newPC(t, srv, "집 PC")
	now := time.Now()

	a, _, _ := work.store.Add(nil, pic(1, 300, 200), shots.Meta{Time: now.Add(-time.Hour), App: "PowerPoint", Title: "CV04 정리.pptx", PC: "회사 PC"})
	b, _, _ := work.store.Add(nil, pic(2, 300, 200), shots.Meta{Time: now, App: "카카오톡", PC: "회사 PC"})
	work.syncOK(t)

	if n := len(fd.Files("shot")); n != 2 {
		t.Fatalf("drive shots = %d", n)
	}
	if n := len(fd.Files("thumb")); n != 2 {
		t.Fatalf("drive thumbs = %d", n)
	}
	if n := len(fd.Files("root")); n != 1 {
		t.Fatalf("roots = %d", n)
	}
	ga, _ := work.store.Get(a.ID)
	if ga.DriveID == "" || ga.ThumbID == "" || !strings.HasPrefix(ga.Account, "choo@example.com|") {
		t.Fatalf("not linked: %+v", ga)
	}

	// Home sees both, with thumbnails, and the PC list.
	home.syncOK(t)
	ha, ok := home.store.Get(a.ID)
	if !ok || !ha.Remote || ha.PC != "회사 PC" || ha.Title != "CV04 정리.pptx" || ha.App != "PowerPoint" || ha.W != 300 {
		t.Fatalf("home a = %+v", ha)
	}
	if _, err := os.Stat(home.store.ThumbPath(a.ID)); err != nil {
		t.Fatal("no thumb on home", err)
	}
	if st := home.sync.Status(); !st.Connected || len(st.PCs) != 1 || st.PCs[0] != "회사 PC" {
		t.Fatalf("status %+v", st)
	}

	// Full picture on demand.
	p, err := home.sync.Fetch(context.Background(), ha)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	want, _ := os.ReadFile(work.store.ImagePath(ga))
	if !bytes.Equal(got, want) {
		t.Fatal("fetched bytes differ")
	}

	// Home keeps a with a note; work picks it up and moves the file.
	time.Sleep(2 * time.Millisecond)
	home.store.SetPinned(a.ID, true, true)
	home.store.SetNote(a.ID, "시험 범위 슬라이드", true)
	home.syncOK(t)
	work.syncOK(t)
	wa, _ := work.store.Get(a.ID)
	if !wa.Pinned || wa.Note != "시험 범위 슬라이드" || !strings.HasPrefix(wa.File, shots.KeptDir+"/") || wa.Dirty {
		t.Fatalf("work a = %+v", wa)
	}
	if _, err := os.Stat(work.store.ImagePath(wa)); err != nil {
		t.Fatal(err)
	}

	// Work deletes b; home loses it too.
	work.store.Delete(b.ID)
	work.syncOK(t)
	if n := len(fd.Files("shot")); n != 1 {
		t.Fatalf("drive shots after delete = %d", n)
	}
	if n := len(fd.Files("thumb")); n != 1 {
		t.Fatalf("drive thumbs after delete = %d", n)
	}
	home.syncOK(t)
	if _, ok := home.store.Get(b.ID); ok {
		t.Fatal("home still has b")
	}

	// Home's own capture reaches work.
	c, _, _ := home.store.Add(nil, pic(3, 64, 64), shots.Meta{Time: now, PC: "집 PC"})
	home.syncOK(t)
	work.syncOK(t)
	if wc, ok := work.store.Get(c.ID); !ok || wc.PC != "집 PC" {
		t.Fatalf("work c = %+v", wc)
	}

	// Retention changed at home wins on work.
	home.set = Settings{RetentionDays: 7, UpdatedAt: time.Now().UnixMilli()}
	home.syncOK(t)
	work.syncOK(t)
	if work.set.RetentionDays != 7 {
		t.Fatalf("work retention %+v", work.set)
	}

	// Expired on Drive is deleted by whoever syncs; kept ones stay.
	old := time.Now().AddDate(0, 0, -20).UnixMilli()
	root := fd.Files("root")[0]["id"].(string)
	fd.Put("old.png", "image/png", []string{root}, map[string]string{"screenmit": "shot", "id": "oldone", "t": itoa(old), "pin": "0", "pc": "집 PC"}, "", []byte("x"))
	fd.Put("oldkept.png", "image/png", []string{root}, map[string]string{"screenmit": "shot", "id": "oldkept", "t": itoa(old), "pin": "1", "pc": "집 PC"}, "", []byte("x"))
	work.syncOK(t)
	ids := map[string]bool{}
	for _, f := range fd.Files("shot") {
		ids[f["appProperties"].(map[string]string)["id"]] = true
	}
	if ids["oldone"] || !ids["oldkept"] {
		t.Fatalf("expiry on drive: %v", ids)
	}

	// Access token expiry is handled by refresh.
	fd.ExpireAccess()
	work.syncOK(t)

	// Revoked login shows up as "need login".
	fd.RevokeAll()
	if _, err := work.sync.SyncOnce(context.Background()); err == nil {
		t.Fatal("want error")
	}
	if st := work.sync.Status(); !st.NeedLogin || st.Connected {
		t.Fatalf("status %+v", st)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestSearchLagDoesNotDeleteLocal(t *testing.T) {
	fd := fakedrive.New()
	srv := httptest.NewServer(fd)
	defer srv.Close()
	work := newPC(t, srv, "회사 PC")
	a, _, _ := work.store.Add(nil, pic(4, 40, 40), shots.Meta{Time: time.Now()})
	fd.SearchLag = time.Hour
	work.syncOK(t) // uploads
	work.syncOK(t) // the new file is hidden from search now
	if _, ok := work.store.Get(a.ID); !ok {
		t.Fatal("lagging search deleted a fresh local shot")
	}
	if _, err := os.Stat(filepath.Join(work.store.Root(), filepath.FromSlash(a.File))); err != nil {
		t.Fatal(err)
	}
	// and it was not uploaded twice
	fd.SearchLag = 0
	work.syncOK(t)
	if n := fd.Requests["POST /upload/drive/v3/files"]; n != 3 { // settings + thumb + png
		t.Fatalf("uploads = %d", n)
	}
}

func TestLargeUploadAndRelinkAfterReconnect(t *testing.T) {
	fd := fakedrive.New()
	srv := httptest.NewServer(fd)
	defer srv.Close()
	work := newPC(t, srv, "회사 PC")
	r := rand.New(rand.NewSource(1))
	big := image.NewNRGBA(image.Rect(0, 0, 1400, 1000))
	r.Read(big.Pix)
	for i := 3; i < len(big.Pix); i += 4 {
		big.Pix[i] = 255
	}
	a, _, err := work.store.Add(nil, big, shots.Meta{Time: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if a.Bytes <= 4<<20 {
		t.Fatalf("test picture too small: %d", a.Bytes)
	}
	work.syncOK(t)
	if fd.Requests["PUT /upload/session/*"] != 1 {
		t.Fatalf("resumable not used: %v", fd.Requests)
	}
	f := fd.Files("shot")[0]
	if f["size"] != itoa(a.Bytes) {
		t.Fatalf("size %v vs %d", f["size"], a.Bytes)
	}

	// Disconnect (ids forgotten) then reconnect: link by id, no second upload.
	work.store.ForgetAccount("")
	work.sync.ResetFolders()
	work.syncOK(t)
	if n := len(fd.Files("shot")); n != 1 {
		t.Fatalf("duplicate upload: %d", n)
	}
	if g, _ := work.store.Get(a.ID); g.DriveID == "" {
		t.Fatal("not relinked")
	}
}
