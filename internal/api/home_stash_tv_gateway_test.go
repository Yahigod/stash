package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stashapp/stash/pkg/session"
)

const homeStashTVGatewayTestToken = "sender-token-with-at-least-thirty-two-characters"

func homeStashTVGatewayTestRequest(method, path string, body io.Reader) *http.Request {
	request := httptest.NewRequest(method, "http://stash.test"+path, body)
	request.Host = "stash.test"
	request.Header.Set(homeStashTVGatewayCSRFHeader, "1")
	request.Header.Set("Origin", "http://stash.test")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	return request
}

func homeStashTVGatewayTestHandler(t *testing.T, upstream http.Handler) http.Handler {
	t.Helper()
	server := httptest.NewServer(upstream)
	t.Cleanup(server.Close)
	upstreamURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := server.Client()
	client.Timeout = 500 * time.Millisecond
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return newHomeStashTVGateway(
		homeStashTVGatewayConfig{upstreamURL: upstreamURL, senderToken: homeStashTVGatewayTestToken},
		client,
		func(*http.Request) bool { return true },
		func(string, string, int, time.Duration) {},
	)
}

func TestHomeStashTVGatewayMountedRouteStripsPublicPrefix(t *testing.T) {
	upstreamPath := make(chan string, 1)
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath <- r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"v":1,"receivers":[]}`)
	})
	router := chi.NewRouter()
	mountHomeStashTVGateway(router, homeStashTVGatewayTestHandler(t, upstream))

	recorder := httptest.NewRecorder()
	request := homeStashTVGatewayTestRequest(
		http.MethodGet,
		homeStashTVGatewayPrefix+"/v1/receivers",
		nil,
	)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("mounted gateway returned %d: %s", recorder.Code, recorder.Body.String())
	}
	if path := <-upstreamPath; path != "/api/v1/receivers" {
		t.Fatalf("unexpected upstream path %q", path)
	}
}

func TestHomeStashTVGatewayConfigIsOptionalButComplete(t *testing.T) {
	values := map[string]string{}
	getenv := func(key string) string { return values[key] }
	loaded, err := loadHomeStashTVGatewayConfig(getenv)
	if err != nil || loaded != nil {
		t.Fatalf("empty optional configuration = %#v, %v", loaded, err)
	}

	values[homeStashTVGatewayURLVariable] = "http://bridge.test:8791"
	if _, err := loadHomeStashTVGatewayConfig(getenv); err == nil {
		t.Fatal("partial gateway configuration was accepted")
	}
	values[homeStashTVGatewayTokenFileVariable] = filepath.Join(t.TempDir(), "missing")
	if _, err := loadHomeStashTVGatewayConfig(getenv); err == nil {
		t.Fatal("missing sender token file was accepted")
	}
}

func TestHomeStashTVGatewayConfigPinsOneOriginAndReadsPrivateFile(t *testing.T) {
	directory := t.TempDir()
	tokenFile := filepath.Join(directory, "sender-token")
	if err := os.WriteFile(tokenFile, []byte(homeStashTVGatewayTestToken+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{
		homeStashTVGatewayURLVariable:       "http://bridge.test:8791/",
		homeStashTVGatewayTokenFileVariable: tokenFile,
	}
	loaded, err := loadHomeStashTVGatewayConfig(func(key string) string { return values[key] })
	if err != nil {
		t.Fatal(err)
	}
	if loaded.upstreamURL.String() != "http://bridge.test:8791" || loaded.senderToken != homeStashTVGatewayTestToken {
		t.Fatalf("unexpected normalized configuration: %s", loaded)
	}
	if strings.Contains(loaded.String(), homeStashTVGatewayTestToken) {
		t.Fatal("configuration string exposed the sender token")
	}

	for _, invalid := range []string{
		"http://user:secret@bridge.test:8791",
		"http://bridge.test:8791/admin",
		"http://bridge.test:8791?upstream=http://other.test",
		"file:///tmp/socket",
	} {
		values[homeStashTVGatewayURLVariable] = invalid
		if _, err := loadHomeStashTVGatewayConfig(func(key string) string { return values[key] }); err == nil {
			t.Fatalf("invalid upstream %q was accepted", invalid)
		}
	}
}

func TestHomeStashTVGatewayRejectsSymlinkAndWritableTokenFiles(t *testing.T) {
	directory := t.TempDir()
	tokenFile := filepath.Join(directory, "sender-token")
	if err := os.WriteFile(tokenFile, []byte(homeStashTVGatewayTestToken), 0o666); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tokenFile, 0o666); err != nil {
			t.Fatal(err)
		}
		if _, err := readHomeStashTVGatewayToken(tokenFile); err == nil {
			t.Fatal("group/other-writable token file was accepted")
		}
	}
	if err := os.Chmod(tokenFile, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "sender-token-link")
	if err := os.Symlink(tokenFile, link); err == nil {
		if _, err := readHomeStashTVGatewayToken(link); err == nil {
			t.Fatal("sender token symlink was accepted")
		}
	}
}

func TestHomeStashTVGatewayForwardsOnlyReviewedSenderRoutes(t *testing.T) {
	var upstreamHits atomic.Int32
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHits.Add(1)
		if r.URL.Path != "/api/v1/receivers" || r.Method != http.MethodGet {
			t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+homeStashTVGatewayTestToken {
			t.Errorf("unexpected sender authorization: %q", got)
		}
		if r.Header.Get("Origin") != "" || r.Header.Get(homeStashTVGatewayCSRFHeader) != "" {
			t.Error("browser-only headers leaked to the bridge")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"v":1,"receivers":[{"receiver_id":"receiver-1","device_name":"Living Room TV","revoked":false,"created_at_ms":1,"profiles":[{"id":"normal-stash","name":"Normal Stash"}],"playback_state":null,"online":true,"protocol_version":1,"app_version":"1.0.0","last_seen_ms":2}]}`)
	})
	handler := homeStashTVGatewayTestHandler(t, upstream)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, homeStashTVGatewayTestRequest(http.MethodGet, "/v1/receivers", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Living Room TV") {
		t.Fatalf("receiver response = %d %s", recorder.Code, recorder.Body.String())
	}

	for _, denied := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v1/pairings"},
		{http.MethodGet, "/v1/pairings/id"},
		{http.MethodGet, "/v1/receivers/connect"},
		{http.MethodDelete, "/v1/receivers/receiver-1"},
		{http.MethodPost, "/v1/commands/id"},
		{http.MethodGet, "/v1/commands/id/extra"},
		{http.MethodGet, "/admin"},
	} {
		recorder = httptest.NewRecorder()
		handler.ServeHTTP(recorder, homeStashTVGatewayTestRequest(denied.method, denied.path, nil))
		if recorder.Code != http.StatusNotFound && recorder.Code != http.StatusBadRequest {
			t.Fatalf("denied route %s %s returned %d", denied.method, denied.path, recorder.Code)
		}
	}
	if upstreamHits.Load() != 1 {
		t.Fatalf("denied routes reached upstream; hits=%d", upstreamHits.Load())
	}
}

func TestHomeStashTVGatewayValidatesAndForwardsCommands(t *testing.T) {
	commandID := "3f0d3f0e-4d6f-4a14-8a40-9bdc8b75ce01"
	var postBody homeStashTVGatewayCommandRequest
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/commands":
			if err := json.NewDecoder(r.Body).Decode(&postBody); err != nil {
				t.Error(err)
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"v":1,"status":"pending","wake_status":"requested","command_id":"`+commandID+`","receiver_id":"receiver-1","expires_at_ms":20,"receiver_online":false}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/commands/"+commandID:
			_, _ = io.WriteString(w, `{"v":1,"command_id":"`+commandID+`","receiver_id":"receiver-1","state":"acknowledged","ack_status":"accepted","ack_error_code":null,"created_at_ms":10,"expires_at_ms":20}`)
		default:
			http.NotFound(w, r)
		}
	})
	handler := homeStashTVGatewayTestHandler(t, upstream)
	body := `{"receiver_id":"receiver-1","profile_id":"normal-stash","scene_ids":["9","2","7"],"start_index":1,"start_position_ms":5000,"policy":{"continue":true,"loop":true,"reshuffle":true}}`
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, homeStashTVGatewayTestRequest(http.MethodPost, "/v1/commands", strings.NewReader(body)))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("command response = %d %s", recorder.Code, recorder.Body.String())
	}
	if strings.Join(postBody.SceneIDs, ",") != "9,2,7" || postBody.StartIndex == nil || *postBody.StartIndex != 1 || postBody.Policy == nil || !postBody.Policy.Reshuffle {
		t.Fatalf("command boundary changed reviewed input: %#v", postBody)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, homeStashTVGatewayTestRequest(http.MethodGet, "/v1/commands/"+commandID, nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"ack_status":"accepted"`) {
		t.Fatalf("status response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestHomeStashTVGatewayCorrelatesCommandResponses(t *testing.T) {
	commandID := "3f0d3f0e-4d6f-4a14-8a40-9bdc8b75ce01"
	submission := homeStashTVGatewayCommandSubmission{
		Version:     1,
		Status:      "pending",
		WakeStatus:  "requested",
		CommandID:   commandID,
		ReceiverID:  "receiver-2",
		ExpiresAtMS: 20,
	}
	if validateHomeStashTVGatewayCommandSubmission(submission, "receiver-1") == nil {
		t.Fatal("command submission for a different receiver was accepted")
	}

	ack := "accepted"
	status := homeStashTVGatewayCommandStatus{
		Version:     1,
		CommandID:   commandID,
		ReceiverID:  "receiver-1",
		State:       "pending",
		AckStatus:   &ack,
		CreatedAtMS: 10,
		ExpiresAtMS: 20,
	}
	if validateHomeStashTVGatewayCommandStatus(status, commandID) == nil {
		t.Fatal("pending command with an acknowledgement was accepted")
	}
}

func TestHomeStashTVGatewayRequiresStashAndSameOriginBoundaries(t *testing.T) {
	var hits atomic.Int32
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	})
	server := httptest.NewServer(upstream)
	defer server.Close()
	upstreamURL, _ := url.Parse(server.URL)
	makeHandler := func(allowed bool) http.Handler {
		return newHomeStashTVGateway(
			homeStashTVGatewayConfig{upstreamURL: upstreamURL, senderToken: homeStashTVGatewayTestToken},
			server.Client(),
			func(*http.Request) bool { return allowed },
			func(string, string, int, time.Duration) {},
		)
	}

	request := homeStashTVGatewayTestRequest(http.MethodGet, "/v1/receivers", nil)
	recorder := httptest.NewRecorder()
	makeHandler(false).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request returned %d", recorder.Code)
	}

	for name, mutate := range map[string]func(*http.Request){
		"missing csrf": func(r *http.Request) { r.Header.Del(homeStashTVGatewayCSRFHeader) },
		"cross origin": func(r *http.Request) { r.Header.Set("Origin", "http://attacker.test") },
		"cross site":   func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") },
		"no evidence": func(r *http.Request) {
			r.Header.Del("Origin")
			r.Header.Del("Referer")
			r.Header.Del("Sec-Fetch-Site")
		},
	} {
		t.Run(name, func(t *testing.T) {
			request := homeStashTVGatewayTestRequest(http.MethodGet, "/v1/receivers", nil)
			mutate(request)
			recorder := httptest.NewRecorder()
			makeHandler(true).ServeHTTP(recorder, request)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("boundary violation returned %d", recorder.Code)
			}
		})
	}
	if hits.Load() != 0 {
		t.Fatalf("rejected browser requests reached upstream; hits=%d", hits.Load())
	}
}

func TestHomeStashTVGatewayAcceptsOnlySessionAuthentication(t *testing.T) {
	request := homeStashTVGatewayTestRequest(http.MethodGet, "/v1/receivers", nil)
	if homeStashTVGatewayRequestAllowed(request) {
		t.Fatal("request without an authenticated Stash session was accepted")
	}

	request = request.WithContext(session.SetCurrentUserID(context.Background(), "operator"))
	if !homeStashTVGatewayRequestAllowed(request) {
		t.Fatal("authenticated Stash session was rejected")
	}

	request.Header.Set(session.ApiKeyHeader, "api-key-must-not-authorize-gateway")
	if homeStashTVGatewayRequestAllowed(request) {
		t.Fatal("Stash API key authorized the browser-only gateway")
	}
	request.Header.Del(session.ApiKeyHeader)
	query := request.URL.Query()
	query.Set(session.ApiKeyParameter, "api-key-must-not-authorize-gateway")
	request.URL.RawQuery = query.Encode()
	if homeStashTVGatewayRequestAllowed(request) {
		t.Fatal("Stash API key query parameter authorized the gateway")
	}
}

func TestHomeStashTVGatewayRoutesReceiveExplicitUnauthorizedResponses(t *testing.T) {
	if !requiresUnauthorizedResponse("/api/home-stash-tv") ||
		!requiresUnauthorizedResponse("/api/home-stash-tv/v1/receivers") ||
		!requiresUnauthorizedResponse("/api/home-stash-tv/v1/commands/id") {
		t.Fatal("gateway route would redirect to the login page instead of returning unauthorized")
	}
	if requiresUnauthorizedResponse("/scenes") {
		t.Fatal("ordinary browser page was classified as an API response")
	}
}

func TestHomeStashTVGatewayRejectsSSRFFieldsAndOversizedBodies(t *testing.T) {
	var hits atomic.Int32
	handler := homeStashTVGatewayTestHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	for _, body := range []string{
		`{"receiver_id":"receiver-1","profile_id":"normal-stash","scene_ids":["1"],"upstream_url":"http://attacker.test"}`,
		`{"receiver_id":"receiver-1","profile_id":"normal-stash","scene_ids":["0"]}`,
		`{"receiver_id":"receiver-1","profile_id":"normal-stash","scene_ids":["1"],"start_index":1}`,
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, homeStashTVGatewayTestRequest(http.MethodPost, "/v1/commands", strings.NewReader(body)))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("invalid command returned %d: %s", recorder.Code, recorder.Body.String())
		}
	}
	recorder := httptest.NewRecorder()
	request := homeStashTVGatewayTestRequest(http.MethodGet, "/v1/receivers?upstream=http://attacker.test", nil)
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("client-supplied query returned %d: %s", recorder.Code, recorder.Body.String())
	}
	large := bytes.Repeat([]byte("x"), homeStashTVGatewayMaxRequestBytes+1)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, homeStashTVGatewayTestRequest(http.MethodPost, "/v1/commands", bytes.NewReader(large)))
	if recorder.Code != http.StatusBadRequest || hits.Load() != 0 {
		t.Fatalf("bounded request gate failed: status=%d hits=%d", recorder.Code, hits.Load())
	}
}

func TestHomeStashTVGatewayBoundsAndValidatesUpstreamResponses(t *testing.T) {
	for name, upstream := range map[string]http.Handler{
		"oversized": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(bytes.Repeat([]byte("x"), homeStashTVGatewayMaxResponseBytes+1))
		}),
		"unknown field": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"v":1,"receivers":[],"admin":true}`)
		}),
		"wrong protocol": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"v":2,"receivers":[]}`)
		}),
		"wrong status": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"v":1,"receivers":[]}`)
		}),
	} {
		t.Run(name, func(t *testing.T) {
			handler := homeStashTVGatewayTestHandler(t, upstream)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, homeStashTVGatewayTestRequest(http.MethodGet, "/v1/receivers", nil))
			if recorder.Code != http.StatusBadGateway {
				t.Fatalf("invalid upstream response returned %d", recorder.Code)
			}
		})
	}
}

type homeStashTVGatewayBlockingTransport struct{}

func (homeStashTVGatewayBlockingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	<-r.Context().Done()
	return nil, r.Context().Err()
}

func TestHomeStashTVGatewayTimeoutIsBounded(t *testing.T) {
	upstreamURL, _ := url.Parse("http://bridge.invalid")
	handler := newHomeStashTVGateway(
		homeStashTVGatewayConfig{upstreamURL: upstreamURL, senderToken: homeStashTVGatewayTestToken},
		&http.Client{Transport: homeStashTVGatewayBlockingTransport{}},
		func(*http.Request) bool { return true },
		func(string, string, int, time.Duration) {},
	).(*homeStashTVGateway)
	handler.readTimeout = 20 * time.Millisecond
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, homeStashTVGatewayTestRequest(http.MethodGet, "/v1/receivers", nil))
	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("timeout returned %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestHomeStashTVGatewayCommandUsesLongerBoundedTimeout(t *testing.T) {
	commandID := "3f0d3f0e-4d6f-4a14-8a40-9bdc8b75ce01"
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/commands" {
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"v":1,"status":"pending","wake_status":"requested","command_id":"`+commandID+`","receiver_id":"receiver-1","expires_at_ms":20,"receiver_online":true}`)
			return
		}
		_, _ = io.WriteString(w, `{"v":1,"receivers":[]}`)
	})
	handler := homeStashTVGatewayTestHandler(t, upstream).(*homeStashTVGateway)
	handler.readTimeout = 20 * time.Millisecond
	handler.commandTimeout = 500 * time.Millisecond

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, homeStashTVGatewayTestRequest(http.MethodGet, "/v1/receivers", nil))
	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("slow receiver read returned %d: %s", recorder.Code, recorder.Body.String())
	}

	body := `{"receiver_id":"receiver-1","profile_id":"normal-stash","scene_ids":["1"]}`
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, homeStashTVGatewayTestRequest(http.MethodPost, "/v1/commands", strings.NewReader(body)))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("bounded foreground command returned %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestHomeStashTVGatewayCommandTimeoutIsBounded(t *testing.T) {
	upstreamURL, _ := url.Parse("http://bridge.invalid")
	handler := newHomeStashTVGateway(
		homeStashTVGatewayConfig{upstreamURL: upstreamURL, senderToken: homeStashTVGatewayTestToken},
		&http.Client{Transport: homeStashTVGatewayBlockingTransport{}},
		func(*http.Request) bool { return true },
		func(string, string, int, time.Duration) {},
	).(*homeStashTVGateway)
	handler.commandTimeout = 20 * time.Millisecond
	body := `{"receiver_id":"receiver-1","profile_id":"normal-stash","scene_ids":["1"]}`
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, homeStashTVGatewayTestRequest(http.MethodPost, "/v1/commands", strings.NewReader(body)))
	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("command timeout returned %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestHomeStashTVGatewayDoesNotFollowRedirectsOrLeakToken(t *testing.T) {
	var redirectedHits atomic.Int32
	redirected := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectedHits.Add(1)
	}))
	defer redirected.Close()
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", redirected.URL+"/api/v1/receivers")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTemporaryRedirect)
		_, _ = io.WriteString(w, `{"error":"bad `+homeStashTVGatewayTestToken+`"}`)
	})
	handler := homeStashTVGatewayTestHandler(t, upstream)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, homeStashTVGatewayTestRequest(http.MethodGet, "/v1/receivers", nil))
	if recorder.Code != http.StatusBadGateway || redirectedHits.Load() != 0 {
		t.Fatalf("redirect boundary failed: status=%d redirected=%d", recorder.Code, redirectedHits.Load())
	}
	if strings.Contains(recorder.Body.String(), homeStashTVGatewayTestToken) {
		t.Fatal("upstream error leaked the sender token")
	}
}

func TestHomeStashTVGatewaySameOriginAllowsRefererFallback(t *testing.T) {
	request := homeStashTVGatewayTestRequest(http.MethodGet, "/v1/receivers", nil)
	request.Header.Del("Origin")
	request.Header.Del("Sec-Fetch-Site")
	request.Header.Set("Referer", "http://stash.test/settings?tab=home-stash-tv")
	if !homeStashTVGatewaySameOrigin(request) {
		t.Fatal("same-origin Referer fallback was rejected")
	}
}
