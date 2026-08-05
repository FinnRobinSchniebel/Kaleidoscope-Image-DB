package authutil

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
)

const (
	CookieModeInsecure  = "insecure"
	CookieModeSecure    = "secure"
	CookieModeCrossSite = "cross-site"
)

// CookieSecure and CookieSameSite are the Secure and SameSite attributes
// used on the refresh_token cookie in LoginUser/LogoutUser. Set once at
// startup via SetCookieSecurityMode; do not assign directly elsewhere.
var (
	CookieSecure   = false
	CookieSameSite = fiber.CookieSameSiteLaxMode
)

// SetCookieSecurityMode sets CookieSecure and CookieSameSite based on how
// the frontend and backend are deployed:
//
//   - "insecure" (default): no TLS, frontend and backend share a registrable
//     domain. Secure=false, SameSite=Lax.
//   - "secure": TLS in front of both (e.g. a reverse proxy), same
//     registrable domain. Secure=true, SameSite=Lax.
//   - "cross-site": frontend and backend on different registrable domains,
//     both served over HTTPS. Secure=true, SameSite=None.
//
// An empty mode defaults to "insecure". Browsers drop SameSite=None cookies
// that aren't also Secure, so the two are always set together here rather
// than as independent flags.
func SetCookieSecurityMode(mode string) error {
	if mode == "" {
		mode = CookieModeInsecure
	}

	switch mode {
	case CookieModeInsecure:
		CookieSecure = false
		CookieSameSite = fiber.CookieSameSiteLaxMode
	case CookieModeSecure:
		CookieSecure = true
		CookieSameSite = fiber.CookieSameSiteLaxMode
	case CookieModeCrossSite:
		CookieSecure = true
		CookieSameSite = fiber.CookieSameSiteNoneMode
	default:
		return fmt.Errorf("unknown COOKIE_SECURITY_MODE %q: must be one of %q, %q, %q",
			mode, CookieModeInsecure, CookieModeSecure, CookieModeCrossSite)
	}

	return nil
}
