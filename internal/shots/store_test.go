package shots

import (
	"bytes"
	"image"
	"image/color"
	_ "image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func pic(seed int, w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x + seed), uint8(y * seed), uint8(seed * 3), 255})
		}
	}
	return img
}

func at(day string, hms string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", day+" "+hms, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

func open(t *testing.T, dir string, now time.Time) *Store {
	t.Helper()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Now = func() time.Time { return now }
	return s
}

func TestAddDedupAndLayout(t *testing.T) {
	dir := t.TempDir()
	now := at("2026-11-16", "14:03:12")
	s := open(t, dir, now)
	changes := 0
	s.OnChange = func() { changes++ }

	sh, added, err := s.Add(nil, pic(1, 800, 600), Meta{Time: now, App: "PowerPoint", Title: "보고서.pptx", PC: "회사 PC"})
	if err != nil || !added {
		t.Fatal(added, err)
	}
	if sh.File != "2026-11-16/Screenmit_2026-11-16_14-03-12.png" {
		t.Fatalf("file %q", sh.File)
	}
	if _, err := os.Stat(filepath.Join(dir, "2026-11-16", "Screenmit_2026-11-16_14-03-12.png")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.ThumbPath(sh.ID)); err != nil {
		t.Fatal("no thumb", err)
	}
	// same pixels again: no new shot
	_, added, _ = s.Add(nil, pic(1, 800, 600), Meta{Time: now.Add(time.Second)})
	if added {
		t.Fatal("dup added")
	}
	// different picture, same second: suffix
	sh2, added, _ := s.Add(nil, pic(2, 800, 600), Meta{Time: now})
	if !added || sh2.File != "2026-11-16/Screenmit_2026-11-16_14-03-12_2.png" {
		t.Fatalf("second %q", sh2.File)
	}
	if got := s.List(); len(got) != 2 || changes != 2 {
		t.Fatalf("list %d changes %d", len(got), changes)
	}
}

func TestPersistAndRescan(t *testing.T) {
	dir := t.TempDir()
	now := at("2026-11-16", "09:00:00")
	s := open(t, dir, now)
	a, _, _ := s.Add(nil, pic(1, 50, 40), Meta{Time: now, App: "Excel"})
	b, _, _ := s.Add(nil, pic(2, 50, 40), Meta{Time: now.Add(time.Minute)})
	s.SetNote(a.ID, "  시험 범위  ", true)
	s.Update(b.ID, func(sh *Shot) { sh.DriveID, sh.Account = "drv-b", "me@x" })
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}

	// user deletes b in Explorer, drops in a PNG of their own, and deletes the index thumbnail
	os.Remove(filepath.Join(dir, filepath.FromSlash(b.File)))
	os.Remove(s.ThumbPath(a.ID))
	c, _, _ := s.Add(nil, pic(3, 30, 30), Meta{Time: now})
	cFile := filepath.Join(dir, filepath.FromSlash(c.File))
	data, _ := os.ReadFile(cFile)
	s.Delete(c.ID)
	os.MkdirAll(filepath.Join(dir, "2026-11-15"), 0o755)
	os.WriteFile(filepath.Join(dir, "2026-11-15", "mine.png"), nil, 0o644) // broken file is skipped
	os.WriteFile(filepath.Join(dir, "2026-11-15", "Screenmit_2026-11-15_08-30-00.png"), data, 0o644)
	s.Flush()

	s2 := open(t, dir, now)
	got := s2.List()
	if len(got) != 2 {
		t.Fatalf("want 2, got %+v", got)
	}
	ga, ok := s2.Get(a.ID)
	if !ok || ga.Note != "시험 범위" || ga.App != "Excel" || !ga.Dirty {
		t.Fatalf("a = %+v", ga)
	}
	if _, err := s2.EnsureThumb(a.ID); err != nil {
		t.Fatal("thumb rebuild", err)
	}
	gc, ok := s2.Get(c.ID)
	if !ok || gc.File != "2026-11-15/Screenmit_2026-11-15_08-30-00.png" || gc.Time.Format("2006-01-02 15:04") != "2026-11-15 08:30" {
		t.Fatalf("c = %+v", gc)
	}
	// b was on Drive: deleting the file leaves a tombstone
	if ts := s2.Tombstones(); len(ts) != 1 || ts[0].DriveID != "drv-b" {
		t.Fatalf("tombs %+v", ts)
	}
}

func TestPinMovesFileAndCleanupKeepsIt(t *testing.T) {
	dir := t.TempDir()
	old := at("2026-11-01", "10:00:00")
	s := open(t, dir, old)
	keep, _, _ := s.Add(nil, pic(1, 20, 20), Meta{Time: old})
	drop, _, _ := s.Add(nil, pic(2, 20, 20), Meta{Time: old})
	fresh, _, _ := s.Add(nil, pic(3, 20, 20), Meta{Time: at("2026-11-16", "08:00:00")})
	s.AddRemote(Shot{ID: "remoteold", Time: old, PC: "집 PC", DriveID: "d1"}, []byte("jpg"))

	if err := s.SetPinned(keep.ID, true, true); err != nil {
		t.Fatal(err)
	}
	k, _ := s.Get(keep.ID)
	if k.File != KeptDir+"/"+filepath.Base(keep.File) || !k.Pinned || !k.Dirty {
		t.Fatalf("kept %+v", k)
	}

	// 14 days: shots from 11-01 live through 11-14; on 11-15 they go.
	s.Now = func() time.Time { return at("2026-11-14", "23:59:00") }
	if n := s.Cleanup(14); n != 0 {
		t.Fatalf("too early: %d", n)
	}
	s.Now = func() time.Time { return at("2026-11-15", "00:01:00") }
	if n := s.Cleanup(14); n != 2 {
		t.Fatalf("removed %d", n)
	}
	if _, ok := s.Get(drop.ID); ok {
		t.Fatal("drop survived")
	}
	if _, err := os.Stat(filepath.Join(dir, "2026-11-01")); !os.IsNotExist(err) {
		t.Fatal("empty day folder left behind")
	}
	if _, ok := s.Get(keep.ID); !ok {
		t.Fatal("kept was deleted")
	}
	if _, ok := s.Get(fresh.ID); !ok {
		t.Fatal("fresh was deleted")
	}
	if _, ok := s.Get("remoteold"); ok {
		t.Fatal("old remote survived")
	}

	// unpin goes back to its day folder
	if err := s.SetPinned(keep.ID, false, true); err != nil {
		t.Fatal(err)
	}
	k, _ = s.Get(keep.ID)
	if k.File != "2026-11-01/"+filepath.Base(keep.File) {
		t.Fatalf("unpinned to %q", k.File)
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(k.File))); err != nil {
		t.Fatal(err)
	}
}

func TestDaysLeft(t *testing.T) {
	now := at("2026-11-16", "12:00:00")
	cases := map[string]int{"2026-11-16": 14, "2026-11-15": 13, "2026-11-03": 1, "2026-11-02": 0}
	for d, want := range cases {
		if got := DaysLeft(at(d, "23:00:00"), now, 14); got != want {
			t.Errorf("%s: %d want %d", d, got, want)
		}
	}
}

func TestDeleteTombstoneAndForgetAccount(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	s := open(t, dir, now)
	a, _, _ := s.Add(nil, pic(1, 10, 10), Meta{Time: now})
	b, _, _ := s.Add(nil, pic(2, 10, 10), Meta{Time: now})
	s.Update(a.ID, func(sh *Shot) { sh.DriveID, sh.ThumbID, sh.Account = "da", "ta", "old@x" })
	s.Update(b.ID, func(sh *Shot) { sh.DriveID, sh.Account = "db", "old@x" })
	s.AddRemote(Shot{ID: "r1", Time: now, DriveID: "dr", Account: "old@x"}, nil)
	s.Delete(a.ID)
	if ts := s.Tombstones(); len(ts) != 1 || ts[0].ThumbID != "ta" {
		t.Fatalf("%+v", ts)
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(a.File))); !os.IsNotExist(err) {
		t.Fatal("file not deleted")
	}
	s.ForgetAccount("new@x")
	gb, _ := s.Get(b.ID)
	if gb.DriveID != "" || len(s.Tombstones()) != 0 {
		t.Fatal("account not forgotten")
	}
	if _, ok := s.Get("r1"); ok {
		t.Fatal("remote from old account kept")
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(b.File))); err != nil {
		t.Fatal("own file must never be deleted by account switch")
	}
}

func TestThumbSize(t *testing.T) {
	b, err := MakeThumb(pic(5, 3840, 1080))
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := image.Decode(bytesReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != ThumbMax || img.Bounds().Dy() != 1080*ThumbMax/3840 {
		t.Fatal(img.Bounds())
	}
}

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
