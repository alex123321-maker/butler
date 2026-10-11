package generatedtools

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/butler/butler/internal/security/workspace"
)

// Developer receives synthetic acceptance inputs only. Its workcopy directory
// comes from the operator, never a model path. Creating/testing a package grants
// no registry membership; the separate durable catalog performs admission.
type Developer struct {
	TempDir string
	Profile Profile
}

func (d Developer) Test(ctx context.Context, raw []byte, n Need) (Report, error) {
	if d.Profile.Validate() != nil || n.Validate() != nil {
		return Report{}, ErrPackage
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(n.Limits.Millis)*time.Millisecond)
	defer cancel()
	copy, e := d.Workcopy(ctx, raw)
	if e != nil {
		return Report{}, e
	}
	p, e := Parse(copy)
	if e != nil {
		return Report{}, e
	}
	return Check(ctx, p, n, d.Profile)
}

func (d Developer) Workcopy(ctx context.Context, raw []byte) ([]byte, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	p, e := Parse(raw)
	if e != nil {
		return nil, e
	}
	base, e := filepath.Abs(d.TempDir)
	if e != nil || d.TempDir == "" {
		return nil, ErrPackage
	}
	work, e := os.MkdirTemp(base, "converter-")
	if e != nil {
		return nil, ErrPackage
	}
	defer os.RemoveAll(work) // fixed operator base and os-created basename only
	root, e := os.OpenRoot(work)
	if e != nil {
		return nil, ErrPackage
	}
	defer root.Close()
	file, e := root.OpenFile("candidate.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return nil, ErrPackage
	}
	_, e = file.Write(p.Raw)
	closeErr := file.Close()
	if e != nil || closeErr != nil {
		return nil, ErrPackage
	}
	copy, e := workspace.ReadRegular(work, "candidate.json", MaxPackageBytes)
	if e != nil || HashBytes(copy) != p.Digest {
		return nil, ErrPackage
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	return copy, nil
}
