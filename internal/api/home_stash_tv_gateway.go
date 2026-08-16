package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/session"
)

const (
	homeStashTVGatewayURLVariable       = "STASH_TV_GATEWAY_URL"
	homeStashTVGatewayTokenFileVariable = "STASH_TV_GATEWAY_TOKEN_FILE"
	homeStashTVGatewayCSRFHeader        = "X-Stash-TV-CSRF"
	homeStashTVGatewayPrefix            = "/api/home-stash-tv"
	homeStashTVGatewayMaxRequestBytes   = 64 * 1024
	homeStashTVGatewayMaxResponseBytes  = 512 * 1024
	homeStashTVGatewayReadTimeout       = 5 * time.Second
	// Command submission can include the receiver bridge's bounded
	// 30-second foreground preparation before it returns an acknowledgement.
	homeStashTVGatewayCommandTimeout = 40 * time.Second
)

var (
	homeStashTVGatewayIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	homeStashTVGatewaySceneIDPattern    = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)
	homeStashTVGatewayCommandIDPattern  = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
)

type homeStashTVGatewayConfig struct {
	upstreamURL *url.URL
	senderToken string
}

type homeStashTVGateway struct {
	config         homeStashTVGatewayConfig
	client         *http.Client
	readTimeout    time.Duration
	commandTimeout time.Duration
	allowRequest   func(*http.Request) bool
	audit          func(method, route string, status int, duration time.Duration)
}

type homeStashTVGatewayStatusWriter struct {
	http.ResponseWriter
	status int
}

func (w *homeStashTVGatewayStatusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *homeStashTVGatewayStatusWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(body)
}

type homeStashTVGatewayProfile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type homeStashTVGatewayPlaybackState struct {
	CommandID       string   `json:"command_id"`
	State           string   `json:"state"`
	SceneID         *string  `json:"scene_id"`
	QueueIndex      *int     `json:"queue_index"`
	PositionMS      *int64   `json:"position_ms"`
	ErrorCode       *string  `json:"error_code"`
	SkippedSceneIDs []string `json:"skipped_scene_ids"`
	UpdatedAtMS     int64    `json:"updated_at_ms"`
}

type homeStashTVGatewayReceiver struct {
	ReceiverID      string                           `json:"receiver_id"`
	DeviceName      string                           `json:"device_name"`
	Revoked         bool                             `json:"revoked"`
	CreatedAtMS     int64                            `json:"created_at_ms"`
	Profiles        []homeStashTVGatewayProfile      `json:"profiles"`
	PlaybackState   *homeStashTVGatewayPlaybackState `json:"playback_state"`
	Online          bool                             `json:"online"`
	ProtocolVersion *int                             `json:"protocol_version"`
	AppVersion      *string                          `json:"app_version"`
	LastSeenMS      *int64                           `json:"last_seen_ms"`
}

type homeStashTVGatewayReceiversResponse struct {
	Version   int                          `json:"v"`
	Receivers []homeStashTVGatewayReceiver `json:"receivers"`
}

type homeStashTVGatewayPolicy struct {
	Continue  bool `json:"continue"`
	Loop      bool `json:"loop"`
	Reshuffle bool `json:"reshuffle"`
}

type homeStashTVGatewayCommandRequest struct {
	ReceiverID      string                    `json:"receiver_id"`
	ProfileID       string                    `json:"profile_id"`
	SceneIDs        []string                  `json:"scene_ids"`
	StartIndex      *int                      `json:"start_index,omitempty"`
	StartPositionMS *int64                    `json:"start_position_ms,omitempty"`
	Policy          *homeStashTVGatewayPolicy `json:"policy,omitempty"`
}

type homeStashTVGatewayCommandSubmission struct {
	Version        int    `json:"v"`
	Status         string `json:"status"`
	WakeStatus     string `json:"wake_status"`
	CommandID      string `json:"command_id"`
	ReceiverID     string `json:"receiver_id"`
	ExpiresAtMS    int64  `json:"expires_at_ms"`
	ReceiverOnline *bool  `json:"receiver_online,omitempty"`
}

type homeStashTVGatewayCommandStatus struct {
	Version      int     `json:"v"`
	CommandID    string  `json:"command_id"`
	ReceiverID   string  `json:"receiver_id"`
	State        string  `json:"state"`
	AckStatus    *string `json:"ack_status"`
	AckErrorCode *string `json:"ack_error_code"`
	CreatedAtMS  int64   `json:"created_at_ms"`
	ExpiresAtMS  int64   `json:"expires_at_ms"`
}

func loadHomeStashTVGatewayConfig(getenv func(string) string) (*homeStashTVGatewayConfig, error) {
	upstreamValue := strings.TrimSpace(getenv(homeStashTVGatewayURLVariable))
	tokenFileValue := strings.TrimSpace(getenv(homeStashTVGatewayTokenFileVariable))
	if upstreamValue == "" && tokenFileValue == "" {
		return nil, nil
	}
	if upstreamValue == "" || tokenFileValue == "" {
		return nil, errors.New("Home Stash TV gateway requires both its fixed upstream and sender token file")
	}

	upstream, err := url.Parse(upstreamValue)
	if err != nil || (upstream.Scheme != "http" && upstream.Scheme != "https") || upstream.Host == "" || upstream.User != nil || (upstream.Path != "" && upstream.Path != "/") || upstream.RawQuery != "" || upstream.Fragment != "" {
		return nil, errors.New("Home Stash TV gateway upstream must be one HTTP(S) origin")
	}
	upstream.Path = ""
	upstream.RawPath = ""

	token, err := readHomeStashTVGatewayToken(tokenFileValue)
	if err != nil {
		return nil, err
	}
	return &homeStashTVGatewayConfig{upstreamURL: upstream, senderToken: token}, nil
}

func readHomeStashTVGatewayToken(filename string) (string, error) {
	if !filepath.IsAbs(filename) || filepath.Clean(filename) != filename {
		return "", errors.New("Home Stash TV sender token file path must be absolute and canonical")
	}
	before, err := os.Lstat(filename)
	if err != nil {
		return "", errors.New("Home Stash TV sender token file failed its identity or mode gate")
	}
	unsafeMode := runtime.GOOS != "windows" && before.Mode()&0o022 != 0
	if !before.Mode().IsRegular() || unsafeMode || before.Size() < 32 || before.Size() > 1024 {
		return "", errors.New("Home Stash TV sender token file failed its identity or mode gate")
	}

	file, err := os.Open(filename)
	if err != nil {
		return "", errors.New("Home Stash TV sender token file could not be opened")
	}
	defer file.Close()

	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return "", errors.New("Home Stash TV sender token file changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, 1025))
	if err != nil || len(data) > 1024 {
		return "", errors.New("Home Stash TV sender token file could not be read safely")
	}
	after, err := os.Lstat(filename)
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(opened, after) {
		return "", errors.New("Home Stash TV sender token file changed while reading")
	}

	token := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	if len(token) < 32 || len(token) > 512 || strings.TrimSpace(token) != token || strings.IndexFunc(token, func(r rune) bool { return r < 0x21 || r > 0x7e }) != -1 {
		return "", errors.New("Home Stash TV sender token file has an invalid value")
	}
	return token, nil
}

func newHomeStashTVGatewayFromEnvironment() (http.Handler, error) {
	gatewayConfig, err := loadHomeStashTVGatewayConfig(os.Getenv)
	if err != nil || gatewayConfig == nil {
		return nil, err
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	client := &http.Client{
		Transport: transport,
		Timeout:   homeStashTVGatewayCommandTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return newHomeStashTVGateway(*gatewayConfig, client, homeStashTVGatewayRequestAllowed, nil), nil
}

func newHomeStashTVGateway(gatewayConfig homeStashTVGatewayConfig, client *http.Client, allowRequest func(*http.Request) bool, audit func(method, route string, status int, duration time.Duration)) http.Handler {
	if audit == nil {
		audit = func(method, route string, status int, duration time.Duration) {
			logger.Infof("Home Stash TV gateway request method=%s route=%s status=%d duration_ms=%d", method, route, status, duration.Milliseconds())
		}
	}
	return &homeStashTVGateway{
		config:         gatewayConfig,
		client:         client,
		readTimeout:    homeStashTVGatewayReadTimeout,
		commandTimeout: homeStashTVGatewayCommandTimeout,
		allowRequest:   allowRequest,
		audit:          audit,
	}
}

func homeStashTVGatewayRequestAllowed(r *http.Request) bool {
	if r.Header.Get(session.ApiKeyHeader) != "" || r.URL.Query().Get(session.ApiKeyParameter) != "" {
		return false
	}
	userID := session.GetCurrentUserID(r.Context())
	return userID != nil && *userID != ""
}

func (g *homeStashTVGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	route := "denied"
	statusWriter := &homeStashTVGatewayStatusWriter{ResponseWriter: w}
	defer func() {
		status := statusWriter.status
		if status == 0 {
			status = http.StatusInternalServerError
		}
		g.audit(r.Method, route, status, time.Since(started))
	}()

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if g.allowRequest == nil || !g.allowRequest(r) {
		writeHomeStashTVGatewayError(statusWriter, http.StatusUnauthorized, "Stash authentication is required.")
		return
	}
	if r.Header.Get(homeStashTVGatewayCSRFHeader) != "1" || !homeStashTVGatewaySameOrigin(r) {
		writeHomeStashTVGatewayError(statusWriter, http.StatusForbidden, "Same-origin request required.")
		return
	}
	if r.URL.RawQuery != "" {
		writeHomeStashTVGatewayError(statusWriter, http.StatusBadRequest, "Query parameters are not supported.")
		return
	}

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/receivers":
		route = "receivers"
		g.forwardReceivers(statusWriter, r)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/commands":
		route = "commands"
		g.forwardCommand(statusWriter, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/commands/"):
		route = "command-status"
		g.forwardCommandStatus(statusWriter, r)
	default:
		route = "not-found"
		writeHomeStashTVGatewayError(statusWriter, http.StatusNotFound, "Not found.")
	}
}

func homeStashTVGatewaySameOrigin(r *http.Request) bool {
	site := r.Header.Get("Sec-Fetch-Site")
	if site != "" && site != "same-origin" {
		return false
	}

	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	expected := scheme + "://" + r.Host
	for _, candidate := range []string{r.Header.Get("Origin"), r.Header.Get("Referer")} {
		if candidate == "" {
			continue
		}
		parsed, err := url.Parse(candidate)
		if err != nil || parsed.Scheme+"://"+parsed.Host != expected {
			return false
		}
		return true
	}
	return site == "same-origin"
}

func (g *homeStashTVGateway) forwardReceivers(w http.ResponseWriter, r *http.Request) {
	var response homeStashTVGatewayReceiversResponse
	status, err := g.upstreamJSON(r.Context(), g.readTimeout, http.MethodGet, "/api/v1/receivers", nil, &response)
	if err != nil {
		writeHomeStashTVGatewayUpstreamError(w, status, err)
		return
	}
	if status != http.StatusOK || validateHomeStashTVGatewayReceivers(response) != nil {
		writeHomeStashTVGatewayError(w, http.StatusBadGateway, "Bridge response was invalid.")
		return
	}
	writeHomeStashTVGatewayJSON(w, http.StatusOK, response)
}

func (g *homeStashTVGateway) forwardCommand(w http.ResponseWriter, r *http.Request) {
	var request homeStashTVGatewayCommandRequest
	if err := decodeHomeStashTVGatewayRequest(w, r, &request); err != nil || validateHomeStashTVGatewayCommandRequest(request) != nil {
		writeHomeStashTVGatewayError(w, http.StatusBadRequest, "Invalid TV command.")
		return
	}
	var response homeStashTVGatewayCommandSubmission
	status, err := g.upstreamJSON(r.Context(), g.commandTimeout, http.MethodPost, "/api/v1/commands", request, &response)
	if err != nil {
		writeHomeStashTVGatewayUpstreamError(w, status, err)
		return
	}
	if status != http.StatusAccepted || validateHomeStashTVGatewayCommandSubmission(response, request.ReceiverID) != nil {
		writeHomeStashTVGatewayError(w, http.StatusBadGateway, "Bridge response was invalid.")
		return
	}
	writeHomeStashTVGatewayJSON(w, http.StatusAccepted, response)
}

func (g *homeStashTVGateway) forwardCommandStatus(w http.ResponseWriter, r *http.Request) {
	commandID := strings.TrimPrefix(r.URL.Path, "/v1/commands/")
	if !homeStashTVGatewayCommandIDPattern.MatchString(commandID) {
		writeHomeStashTVGatewayError(w, http.StatusBadRequest, "Invalid command ID.")
		return
	}
	var response homeStashTVGatewayCommandStatus
	status, err := g.upstreamJSON(r.Context(), g.readTimeout, http.MethodGet, "/api/v1/commands/"+commandID, nil, &response)
	if err != nil {
		writeHomeStashTVGatewayUpstreamError(w, status, err)
		return
	}
	if status != http.StatusOK || validateHomeStashTVGatewayCommandStatus(response, commandID) != nil {
		writeHomeStashTVGatewayError(w, http.StatusBadGateway, "Bridge response was invalid.")
		return
	}
	writeHomeStashTVGatewayJSON(w, http.StatusOK, response)
}

func decodeHomeStashTVGatewayRequest(w http.ResponseWriter, r *http.Request, target any) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" || r.ContentLength < 0 || r.ContentLength > homeStashTVGatewayMaxRequestBytes {
		return errors.New("invalid request content type or length")
	}
	r.Body = http.MaxBytesReader(w, r.Body, homeStashTVGatewayMaxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request must contain exactly one JSON value")
	}
	return nil
}

func (g *homeStashTVGateway) upstreamJSON(ctx context.Context, timeout time.Duration, method, path string, requestBody any, responseBody any) (int, error) {
	requestContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(requestContext, method, g.config.upstreamURL.String()+path, body)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+g.config.senderToken)
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := g.client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, homeStashTVGatewayMaxResponseBytes+1)
	encoded, err := io.ReadAll(limited)
	if err != nil || len(encoded) > homeStashTVGatewayMaxResponseBytes {
		return response.StatusCode, errors.New("upstream response exceeded its safe boundary")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response.StatusCode, errors.New("upstream rejected the request")
	}
	if mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		return response.StatusCode, errors.New("upstream response was not JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(responseBody); err != nil {
		return response.StatusCode, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return response.StatusCode, errors.New("upstream returned multiple JSON values")
	}
	return response.StatusCode, nil
}

func validateHomeStashTVGatewayCommandRequest(value homeStashTVGatewayCommandRequest) error {
	if !homeStashTVGatewayIdentifierPattern.MatchString(value.ReceiverID) || !homeStashTVGatewayIdentifierPattern.MatchString(value.ProfileID) || len(value.SceneIDs) < 1 || len(value.SceneIDs) > 500 {
		return errors.New("invalid command identity or queue size")
	}
	for _, sceneID := range value.SceneIDs {
		if !homeStashTVGatewaySceneIDPattern.MatchString(sceneID) {
			return errors.New("invalid scene ID")
		}
	}
	if value.StartIndex != nil && (*value.StartIndex < 0 || *value.StartIndex >= len(value.SceneIDs)) {
		return errors.New("invalid start index")
	}
	if value.StartPositionMS != nil && (*value.StartPositionMS < 0 || *value.StartPositionMS > int64((7*24*time.Hour)/time.Millisecond)) {
		return errors.New("invalid start position")
	}
	return nil
}

func validateHomeStashTVGatewayReceivers(value homeStashTVGatewayReceiversResponse) error {
	if value.Version != 1 || value.Receivers == nil || len(value.Receivers) > 100 {
		return errors.New("invalid receiver response")
	}
	for _, receiver := range value.Receivers {
		if !homeStashTVGatewayIdentifierPattern.MatchString(receiver.ReceiverID) || len(receiver.DeviceName) < 1 || len(receiver.DeviceName) > 120 || receiver.Profiles == nil || len(receiver.Profiles) > 100 || receiver.CreatedAtMS < 0 {
			return errors.New("invalid receiver")
		}
		for _, profile := range receiver.Profiles {
			if !homeStashTVGatewayIdentifierPattern.MatchString(profile.ID) || len(profile.Name) < 1 || len(profile.Name) > 120 {
				return errors.New("invalid receiver profile")
			}
		}
		if receiver.ProtocolVersion != nil && *receiver.ProtocolVersion < 0 {
			return errors.New("invalid receiver protocol")
		}
		if receiver.AppVersion != nil && len(*receiver.AppVersion) > 120 {
			return errors.New("invalid receiver app version")
		}
		if receiver.PlaybackState != nil && validateHomeStashTVGatewayPlaybackState(*receiver.PlaybackState) != nil {
			return errors.New("invalid receiver playback state")
		}
	}
	return nil
}

func validateHomeStashTVGatewayPlaybackState(value homeStashTVGatewayPlaybackState) error {
	allowedStates := map[string]bool{"resolving": true, "playing": true, "paused": true, "stopped": true, "completed": true, "failed": true}
	if !homeStashTVGatewayCommandIDPattern.MatchString(value.CommandID) || !allowedStates[value.State] || value.UpdatedAtMS < 0 || len(value.SkippedSceneIDs) > 500 {
		return errors.New("invalid playback state")
	}
	if value.SceneID != nil && !homeStashTVGatewaySceneIDPattern.MatchString(*value.SceneID) {
		return errors.New("invalid playback scene")
	}
	if value.QueueIndex != nil && (*value.QueueIndex < 0 || *value.QueueIndex >= 500) {
		return errors.New("invalid playback index")
	}
	if value.PositionMS != nil && *value.PositionMS < 0 {
		return errors.New("invalid playback position")
	}
	if value.ErrorCode != nil && len(*value.ErrorCode) > 120 {
		return errors.New("invalid playback error")
	}
	for _, sceneID := range value.SkippedSceneIDs {
		if !homeStashTVGatewaySceneIDPattern.MatchString(sceneID) {
			return errors.New("invalid skipped scene")
		}
	}
	return nil
}

func validateHomeStashTVGatewayCommandSubmission(value homeStashTVGatewayCommandSubmission, expectedReceiverID string) error {
	if value.Version != 1 || value.Status != "pending" || (value.WakeStatus != "requested" && value.WakeStatus != "failed") || !homeStashTVGatewayCommandIDPattern.MatchString(value.CommandID) || value.ReceiverID != expectedReceiverID || !homeStashTVGatewayIdentifierPattern.MatchString(value.ReceiverID) || value.ExpiresAtMS < 0 {
		return errors.New("invalid command submission")
	}
	return nil
}

func validateHomeStashTVGatewayCommandStatus(value homeStashTVGatewayCommandStatus, expectedCommandID string) error {
	states := map[string]bool{"pending": true, "acknowledged": true, "expired": true}
	if value.Version != 1 || value.CommandID != expectedCommandID || !homeStashTVGatewayCommandIDPattern.MatchString(value.CommandID) || !homeStashTVGatewayIdentifierPattern.MatchString(value.ReceiverID) || !states[value.State] || value.CreatedAtMS < 0 || value.ExpiresAtMS < value.CreatedAtMS {
		return errors.New("invalid command status")
	}
	if value.AckStatus != nil {
		acks := map[string]bool{"accepted": true, "duplicate": true, "expired": true, "rejected": true}
		if !acks[*value.AckStatus] {
			return errors.New("invalid command acknowledgement")
		}
	}
	if (value.State == "acknowledged") != (value.AckStatus != nil) {
		return errors.New("invalid command acknowledgement state")
	}
	if value.AckErrorCode != nil && len(*value.AckErrorCode) > 120 {
		return errors.New("invalid command error")
	}
	return nil
}

func writeHomeStashTVGatewayUpstreamError(w http.ResponseWriter, status int, err error) {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		writeHomeStashTVGatewayError(w, http.StatusGatewayTimeout, "Bridge request timed out.")
		return
	}
	if status == http.StatusBadRequest || status == http.StatusNotFound || status == http.StatusGone {
		writeHomeStashTVGatewayError(w, status, "Bridge rejected the request.")
		return
	}
	writeHomeStashTVGatewayError(w, http.StatusBadGateway, "Bridge is unavailable.")
}

func writeHomeStashTVGatewayError(w http.ResponseWriter, status int, message string) {
	writeHomeStashTVGatewayJSON(w, status, map[string]string{"error": message})
}

func writeHomeStashTVGatewayJSON(w http.ResponseWriter, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		http.Error(w, "Internal gateway error.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func (c homeStashTVGatewayConfig) String() string {
	return fmt.Sprintf("Home Stash TV gateway upstream=%s token=[redacted]", c.upstreamURL.Redacted())
}
