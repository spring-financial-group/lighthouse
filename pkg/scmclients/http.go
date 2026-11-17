package scmclients

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/jenkins-x/go-scm/scm/transport"
)

// serverAuthClient only sends creds to server, as the URLs it fetches can come from an untrusted pull request.
func serverAuthClient(server *url.URL, creds Credentials) *http.Client {
	return &http.Client{
		Transport: &serverOnlyTransport{
			server: server,
			auth: &transport.Auth{
				Source:     creds,
				Credential: transport.BasicAuthCredential(creds.CloneUser()),
			},
		},
	}
}

type serverOnlyTransport struct {
	server *url.URL
	auth   http.RoundTripper
}

func (t *serverOnlyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme == t.server.Scheme && strings.EqualFold(r.URL.Host, t.server.Host) {
		return t.auth.RoundTrip(r)
	}
	return http.DefaultTransport.RoundTrip(r)
}
