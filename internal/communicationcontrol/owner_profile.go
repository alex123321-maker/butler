package communicationcontrol

import (
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// OwnerProfile is immutable operator configuration, never a model argument or
// mutable database setting. Credential values are referenced by private files.
type OwnerProfile struct {
	Version                                    int
	Database, OwnerID, BindingID, WebBindingID string
	BindingVersion, WebBindingVersion          int64
	ExpectedBot                                string
	BotID, ChatID                              int64
	TokenFile, BotAPIProxyFile, ControlURL     string
	Mode                                       string
	AllowPrivate                               bool
	// Applied only when the pinned bot cursor is first created. Restarts never
	// overwrite its durable offset. The operator retains the cutover backlog.
	InitialUpdateID int64
}

func (p OwnerProfile) Validate() error {
	if p.Version != 1 || !validProfileID(p.Database) || p.Database == "postgres" || p.Database == "template0" || p.Database == "template1" || !validProfileID(p.OwnerID) || !validProfileID(p.BindingID) || !validProfileID(p.WebBindingID) || p.BindingID == p.WebBindingID || p.BindingVersion < 1 || p.WebBindingVersion < 1 || !validProfileID(p.ExpectedBot) || p.BotID <= 0 || p.ChatID <= 0 || !filepath.IsAbs(p.TokenFile) || !filepath.IsAbs(p.BotAPIProxyFile) || p.InitialUpdateID < 0 || p.InitialUpdateID == math.MaxInt64 || p.Mode != "active" && p.Mode != "drain" {
		return ErrDenied
	}
	_, err := NewClient(p.ControlURL, strings.Repeat("x", 32))
	return err
}

func validProfileID(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == ':' || r == '.') {
			return false
		}
	}
	return true
}

func LoadOwnerProfile(path string) (OwnerProfile, error) {
	var p OwnerProfile
	if !filepath.IsAbs(path) {
		return p, ErrDenied
	}
	f, err := os.Open(path)
	if err != nil {
		return p, ErrDenied
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16384 || runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return p, ErrDenied
	}
	raw, err := io.ReadAll(io.LimitReader(f, 16385))
	if err != nil || len(raw) > 16384 || StrictJSON(raw, &p) != nil || p.Validate() != nil {
		return OwnerProfile{}, ErrDenied
	}
	return p, nil
}
