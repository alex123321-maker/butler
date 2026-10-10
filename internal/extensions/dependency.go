package extensions

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"strings"
)

// CheckInstalledRevisionTx is the owned lifecycle reader for final-result
// source dependencies. Runtime profile/hash checks remain in the Broker; this
// checks every mutable persisted installation field via its monotonic revision.
func CheckInstalledRevisionTx(ctx context.Context, tx pgx.Tx, owner, name, revision string) error {
	parts := strings.Split(name, ".")
	if len(parts) != 3 || parts[0] != "extension" {
		return ErrUnavailable
	}
	var hash, status, health string
	var generation int64
	var current, reviewed, selected bool
	err := tx.QueryRow(ctx, `SELECT i.manifest_hash,i.generation,i.status,i.health,COALESCE(i.healthy_until>clock_timestamp(),false),EXISTS(SELECT 1 FROM extension_reviews r WHERE r.manifest_hash=i.manifest_hash AND r.owner_id=i.owner_id),i.allowed_tools_json ? $3 FROM extension_installations i WHERE i.extension_id=$1 AND i.owner_id=$2 FOR SHARE OF i`, parts[1], owner, name).Scan(&hash, &generation, &status, &health, &current, &reviewed, &selected)
	if err != nil || status != "enabled" || health != "healthy" || !current || !reviewed || !selected || revision != fmt.Sprintf("%s:%d", hash, generation) {
		return ErrUnavailable
	}
	return nil
}
