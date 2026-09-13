package postgres

import (
	"github.com/jackc/pgx/v5"
	"strings"
)

// ProtectedConnection rejects TLS configurations with a plaintext fallback.
// Unix sockets rely on local filesystem access; TCP must verify the server.
func ProtectedConnection(c *pgx.ConnConfig) bool {
	if c == nil {
		return false
	}
	if strings.HasPrefix(c.Host, "/") {
		return true
	}
	if c.TLSConfig == nil || (c.TLSConfig.InsecureSkipVerify && c.TLSConfig.VerifyPeerCertificate == nil && c.TLSConfig.VerifyConnection == nil) {
		return false
	}
	for _, fallback := range c.Fallbacks {
		if fallback.TLSConfig == nil || (fallback.TLSConfig.InsecureSkipVerify && fallback.TLSConfig.VerifyPeerCertificate == nil && fallback.TLSConfig.VerifyConnection == nil) {
			return false
		}
	}
	return true
}
