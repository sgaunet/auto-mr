package git

import (
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/sgaunet/bullets"
)

// GetHTTPSAuthForTest exposes getHTTPSAuth to the external test package.
//
// Asserting on log output cannot prove the absence of a credential — it only shows
// what was written to a logger. These tests need to know which credential, if any,
// was actually handed to the transport, so the bridge returns the resolved auth
// method with the internal no-auth sentinel flattened to nil.
func GetHTTPSAuthForTest(rawURL string, logger *bullets.Logger, forgejoURL string) (transport.AuthMethod, error) {
	auth, err := getHTTPSAuth(rawURL, logger, forgejoURL)
	if err != nil {
		return nil, err
	}
	if auth == nil {
		return nil, nil
	}
	if _, isNoAuth := auth.method.(*noAuthMethod); isNoAuth {
		return nil, nil
	}
	return auth.method, nil
}
