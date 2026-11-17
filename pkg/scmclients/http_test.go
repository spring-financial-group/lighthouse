package scmclients_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jenkins-x/lighthouse/pkg/filebrowser"
	"github.com/jenkins-x/lighthouse/pkg/scmclients"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fileToken = "s3cr3t-file-token"

type recordedRequest struct {
	user         string
	password     string
	hasBasicAuth bool
	carriesToken bool
}

var (
	withToken = recordedRequest{
		user:         "bot",
		password:     fileToken,
		hasBasicAuth: true,
		carriesToken: true,
	}
	withoutToken = recordedRequest{}
)

type recorder struct {
	server   *httptest.Server
	requests []recordedRequest
}

func newRecorder(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *recorder {
	t.Helper()
	rec := &recorder{}
	rec.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		rec.requests = append(rec.requests, recordedRequest{
			user:         user,
			password:     password,
			hasBasicAuth: ok,
			carriesToken: carriesToken(r, password),
		})
		if handler != nil {
			handler(w, r)
		}
	}))
	t.Cleanup(rec.server.Close)
	return rec
}

func (r *recorder) port(t *testing.T) string {
	t.Helper()
	u, err := url.Parse(r.server.URL)
	require.NoError(t, err)
	return u.Port()
}

func carriesToken(r *http.Request, basicAuthPassword string) bool {
	if strings.Contains(basicAuthPassword, fileToken) || strings.Contains(r.URL.String(), fileToken) {
		return true
	}
	for _, values := range r.Header {
		for _, v := range values {
			if strings.Contains(v, fileToken) {
				return true
			}
		}
	}
	return false
}

func newFileBrowsers(t *testing.T, server string) *filebrowser.FileBrowsers {
	t.Helper()
	t.Setenv("GIT_KIND", "")
	t.Setenv("GIT_SERVER", "")
	t.Setenv("GIT_USER", "")
	t.Setenv("GIT_TOKEN", fileToken)
	cs := newClientSet(t, configGetter("fake", server, "bot"), scmclients.Options{})
	return cs.FileBrowsers
}

func TestGetURLAuthenticatesToGitServer(t *testing.T) {
	gitServer := newRecorder(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("pipeline"))
	})
	fb := newFileBrowsers(t, gitServer.server.URL)

	data, err := fb.GetURL(gitServer.server.URL + "/org/repo/raw/main/pipeline.yaml")
	require.NoError(t, err)

	assert.Equal(t, "pipeline", string(data))
	assert.Equal(t, []recordedRequest{withToken}, gitServer.requests)
}

func TestGetURLOmitsTokenForOtherHostname(t *testing.T) {
	server := newRecorder(t, nil)
	port := server.port(t)
	fb := newFileBrowsers(t, "http://localhost:"+port)

	_, err := fb.GetURL("http://127.0.0.1:" + port + "/pipeline.yaml")
	require.NoError(t, err)

	assert.Equal(t, []recordedRequest{withoutToken}, server.requests)
}

func TestGetURLOmitsTokenForOtherPort(t *testing.T) {
	gitServer := newRecorder(t, nil)
	other := newRecorder(t, nil)
	fb := newFileBrowsers(t, gitServer.server.URL)

	_, err := fb.GetURL(other.server.URL + "/pipeline.yaml")
	require.NoError(t, err)

	assert.Empty(t, gitServer.requests)
	assert.Equal(t, []recordedRequest{withoutToken}, other.requests)
}

func TestGetURLOmitsTokenAfterRedirectOffGitServer(t *testing.T) {
	other := newRecorder(t, nil)
	gitServer := newRecorder(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.server.URL+"/pipeline.yaml", http.StatusFound)
	})
	fb := newFileBrowsers(t, gitServer.server.URL)

	_, err := fb.GetURL(gitServer.server.URL + "/pipeline.yaml")
	require.NoError(t, err)

	assert.Equal(t, []recordedRequest{withToken}, gitServer.requests)
	assert.Equal(t, []recordedRequest{withoutToken}, other.requests)
}

func TestGetURLOmitsTokenOnSchemeMismatch(t *testing.T) {
	server := newRecorder(t, nil)
	fb := newFileBrowsers(t, "https://127.0.0.1:"+server.port(t))

	_, err := fb.GetURL(server.server.URL + "/pipeline.yaml")
	require.NoError(t, err)

	assert.Equal(t, []recordedRequest{withoutToken}, server.requests)
}
