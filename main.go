// Screenmit keeps every screenshot you copy for a couple of weeks, on this
// PC and (optionally) in your own Google Drive so your other PCs see them.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
)

func main() {
	var o Options
	flag.StringVar(&o.Folder, "folder", "", "screenshot folder (default: Pictures\\Screenmit)")
	flag.StringVar(&o.ConfigDir, "config", "", "settings folder (default: %APPDATA%\\Screenmit)")
	flag.IntVar(&o.Port, "port", 0, "gallery port on 127.0.0.1")
	flag.BoolVar(&o.Background, "background", false, "start without opening the gallery (used at sign-in)")
	addPlatformFlags()
	flag.Parse()

	plat := newPlatform()
	dir := o.ConfigDir
	if dir == "" {
		dir = plat.ConfigDir()
	}
	setupLog(dir)
	if !plat.Claim(o.Background) {
		return // another Screenmit is running; it opened the gallery
	}
	log.Printf("Screenmit %s starting", Version)
	app, err := NewApp(plat, o)
	if err != nil {
		log.Printf("start: %v", err)
		plat.Fatal(err)
		os.Exit(1)
	}
	if err := plat.Run(app); err != nil {
		log.Printf("run: %v", err)
	}
	app.Shutdown()
	log.Printf("Screenmit stopped")
}

func setupLog(dir string) {
	_ = os.MkdirAll(dir, 0o755)
	p := filepath.Join(dir, "screenmit.log")
	if st, err := os.Stat(p); err == nil && st.Size() > 1<<20 {
		_ = os.Rename(p, p+".old")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	var w io.Writer = f
	if logToStderr {
		w = io.MultiWriter(f, os.Stderr)
	}
	log.SetOutput(w)
	log.SetFlags(log.LstdFlags)
	if c, err := os.OpenFile(filepath.Join(dir, "crash.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		fmt.Fprintf(c, "") // keep the handle; the runtime writes a crash report here
		_ = debug.SetCrashOutput(c, debug.CrashOptions{})
	}
}
