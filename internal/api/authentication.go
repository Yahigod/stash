package api

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/session"
)

const (
	tripwireActivatedErrMsg = "Stash is exposed to the public internet without authentication, and is not serving any more content to protect your privacy. " +
		"More information and fixes are available at https://discourse.stashapp.cc/t/-/1658"

	externalAccessErrMsg = "You have attempted to access Stash over the internet, and authentication is not enabled. " +
		"This is extremely dangerous! The whole world can see your your stash page and browse your files! " +
		"Stash is not answering any other requests to protect your privacy. " +
		"Please read the log entry or visit https://discourse.stashapp.cc/t/-/1658"
)

func allowUnauthenticated(r *http.Request) bool {
	// #2715 - allow access to UI files
	return strings.HasPrefix(r.URL.Path, loginEndpoint) || r.URL.Path == logoutEndpoint || r.URL.Path == "/css" || strings.HasPrefix(r.URL.Path, "/assets")
}

func requiresUnauthorizedResponse(requestPath string) bool {
	ext := path.Ext(requestPath)
	return requestPath == gqlEndpoint || requestPath == homeStashTVGatewayPrefix || strings.HasPrefix(requestPath, homeStashTVGatewayPrefix+"/") || (ext != "" && ext != ".html")
}

func authenticateHandler() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := config.GetInstance()

			// error if external access tripwire activated
			if accessErr := session.CheckExternalAccessTripwire(c); accessErr != nil {
				http.Error(w, tripwireActivatedErrMsg, http.StatusForbidden)
				return
			}

			r = session.SetLocalRequest(r)

			userID, err := manager.GetInstance().SessionStore.Authenticate(w, r)
			if err != nil {
				if !errors.Is(err, session.ErrUnauthorized) {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				// unauthorized error
				w.Header().Add("WWW-Authenticate", "FormBased")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			if err := session.CheckAllowPublicWithoutAuth(c, r); err != nil {
				var accessErr session.ExternalAccessError
				if errors.As(err, &accessErr) {
					session.LogExternalAccessError(accessErr)

					err := c.ActivatePublicAccessTripwire(net.IP(accessErr).String())
					if err != nil {
						logger.Errorf("Error activating public access tripwire: %v", err)
					}

					http.Error(w, externalAccessErrMsg, http.StatusForbidden)
				} else {
					logger.Errorf("Error checking external access security: %v", err)
					w.WriteHeader(http.StatusInternalServerError)
				}
				return
			}

			ctx := r.Context()

			if c.HasCredentials() {
				// authentication is required
				if userID == "" && !allowUnauthenticated(r) {
					// API and non-webpage requests receive an explicit authentication
					// failure so browser clients cannot mistake a login-page redirect for
					// an unavailable backend and downgrade their transport.
					if requiresUnauthorizedResponse(r.URL.Path) {
						w.Header().Add("WWW-Authenticate", "FormBased")
						w.WriteHeader(http.StatusUnauthorized)
						return
					}

					prefix := getProxyPrefix(r)

					// otherwise redirect to the login page
					returnURL := url.URL{
						Path:     prefix + r.URL.Path,
						RawQuery: r.URL.RawQuery,
					}
					q := make(url.Values)
					q.Set(returnURLParam, returnURL.String())
					u := url.URL{
						Path:     prefix + loginEndpoint,
						RawQuery: q.Encode(),
					}
					http.Redirect(w, r, u.String(), http.StatusFound)
					return
				}
			}

			ctx = session.SetCurrentUserID(ctx, userID)

			r = r.WithContext(ctx)

			next.ServeHTTP(w, r)
		})
	}
}
