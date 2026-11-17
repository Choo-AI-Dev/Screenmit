package imgclip

import "strings"

// Standard clipboard format ids (winuser.h).
const (
	CF_TEXT         = 1
	CF_BITMAP       = 2
	CF_OEMTEXT      = 7
	CF_DIB          = 8
	CF_UNICODETEXT  = 13
	CF_ENHMETAFILE  = 14
	CF_HDROP        = 15
	CF_DIBV5        = 17
	registeredFirst = 0xC000
)

// Format is one entry from EnumClipboardFormats. Name is set for registered
// formats (id >= 0xC000) only.
type Format struct {
	ID   uint32
	Name string
}

// Source says which clipboard format to read the picture from.
type Source int

const (
	SourceNone Source = iota
	SourcePNG
	SourceDIBV5
	SourceDIB
)

// Decision is what to do with one clipboard change.
type Decision struct {
	Source Source
	FmtID  uint32 // format to read when Source != SourceNone
	Skip   string // why we ignore it ("" when not skipped or not an image)
}

// Markers left by apps that copy documents (Office, browsers with text…).
// An image that comes with text plus one of these is a table or a passage of
// text, not a screenshot.
var docMarkers = []string{
	"rich text format", "html format", "biff12", "biff8", "biff5", "csv",
	"xml spreadsheet", "object descriptor", "embed source", "link source",
	"art::gvml clipformat", "hyperlink", "mswordbiff", "msodrawing",
}

// Formats apps set to ask clipboard managers not to record (password managers).
var privateMarkers = []string{
	"excludeclipboardcontentfrommonitorprocessing",
	"clipboard viewer ignore",
}

// Decide picks the picture format and applies the skip rules.
// noHistory is true when the clipboard carries CanIncludeInClipboardHistory = 0.
func Decide(formats []Format, skipDocs, noHistory bool) Decision {
	var png, dib, dibv5, bitmap, text, emf, doc, private bool
	var pngID uint32
	dibAt, dibv5At := -1, -1
	for i, f := range formats {
		switch f.ID {
		case CF_DIB:
			dib = true
			if dibAt < 0 {
				dibAt = i
			}
		case CF_DIBV5:
			dibv5 = true
			if dibv5At < 0 {
				dibv5At = i
			}
		case CF_BITMAP:
			bitmap = true
		case CF_TEXT, CF_UNICODETEXT, CF_OEMTEXT:
			text = true
		case CF_ENHMETAFILE:
			emf = true
		}
		if f.ID >= registeredFirst {
			n := strings.ToLower(f.Name)
			if n == "png" || n == "image/png" {
				if !png {
					png, pngID = true, f.ID
				}
			}
			for _, m := range docMarkers {
				if n == m {
					doc = true
				}
			}
			for _, m := range privateMarkers {
				if n == m {
					private = true
				}
			}
		}
	}
	if !png && !dib && !dibv5 && !bitmap {
		return Decision{}
	}
	if private || noHistory {
		return Decision{Skip: "private"}
	}
	if skipDocs && text && (emf || doc) {
		return Decision{Skip: "document"}
	}
	switch {
	case png:
		return Decision{Source: SourcePNG, FmtID: pngID}
	case dibv5 && (!dib || dibv5At < dibAt):
		// Windows lists the formats an app placed before the ones it can
		// synthesize, so whichever DIB flavour comes first is the original.
		return Decision{Source: SourceDIBV5, FmtID: CF_DIBV5}
	default:
		// CF_BITMAP alone: Windows synthesizes CF_DIB on request.
		return Decision{Source: SourceDIB, FmtID: CF_DIB}
	}
}
