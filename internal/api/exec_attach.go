package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

type ExecAttachSocket struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func (s *ExecAttachSocket) ReadEvent() (ExecStreamEvent, error) {
	var event ExecStreamEvent
	if err := s.conn.ReadJSON(&event); err != nil {
		return ExecStreamEvent{}, err
	}
	return event, nil
}

func (s *ExecAttachSocket) WriteInput(input ExecAttachInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn.WriteJSON(input)
}

func (s *ExecAttachSocket) Close() error {
	if s == nil || s.conn == nil {
		return nil
	}
	return s.conn.Close()
}

func (c *Client) ExecAttach(ctx context.Context, creds Credentials, execID string) (*ExecAttachSocket, error) {
	attachURL, err := websocketURL(c.http.BaseURL, "/execs/"+url.PathEscape(strings.TrimSpace(execID))+"/attach")
	if err != nil {
		return nil, err
	}

	headers := http.Header{}
	headers.Set("Authorization", basicAuthHeader(creds))

	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, attachURL, headers)
	if err != nil {
		if resp != nil {
			defer resp.Body.Close()
			return nil, httpResponseError(resp)
		}
		return nil, err
	}
	return &ExecAttachSocket{conn: conn}, nil
}

func basicAuthHeader(creds Credentials) string {
	token := creds.AK + ":" + creds.SK
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(token))
}

func websocketURL(baseURL string, path string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", err
	}
	switch parsed.Scheme {
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	default:
		return "", errors.New("expected http or https endpoint")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + path
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func httpResponseError(resp *http.Response) error {
	if resp == nil {
		return &Error{Message: "request failed"}
	}
	body, err := io.ReadAll(resp.Body)
	if err == nil {
		var payload errorPayload
		if jsonErr := json.Unmarshal(body, &payload); jsonErr == nil && strings.TrimSpace(payload.Error) != "" {
			return &Error{StatusCode: resp.StatusCode, Message: strings.TrimSpace(payload.Error)}
		}
		message := strings.TrimSpace(string(body))
		if message != "" {
			return &Error{StatusCode: resp.StatusCode, Message: message}
		}
	}
	return &Error{StatusCode: resp.StatusCode, Message: strings.TrimSpace(resp.Status)}
}
