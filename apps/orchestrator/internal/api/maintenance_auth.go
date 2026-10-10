package api

import "net/http"

// OwnerMaintenance protects control-plane mutation routes, including provider
// login, kernel settings and process restart. Tool runtimes never receive this
// token. Ordinary GETs retain the deployment's existing read perimeter.
func OwnerMaintenance(auth *OwnerControlAuth, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if _, ok := auth.Authenticate(r); !ok {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "owner authentication required"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
