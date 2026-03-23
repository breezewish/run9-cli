package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

type errorPayload struct {
	Error string `json:"error"`
}

// Credentials are the org-scoped API key credentials used by the CLI.
type Credentials struct {
	AK string
	SK string
}

// Error represents one portal-api request failure.
type Error struct {
	StatusCode int
	Message    string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("portal api request failed with status %d", e.StatusCode)
}

// Client is the CLI-facing portal-api HTTP client.
type Client struct {
	http *resty.Client
}

// NewClient creates one portal-api HTTP client rooted at the given endpoint.
func NewClient(endpoint string) *Client {
	baseURL := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	return &Client{
		http: resty.New().
			SetBaseURL(baseURL).
			SetDisableWarn(true).
			SetTimeout(30*time.Second).
			SetHeader("Accept", "application/json"),
	}
}

func (c *Client) WhoAmI(ctx context.Context, creds Credentials) (CurrentOrgIdentityView, error) {
	var view CurrentOrgIdentityView
	err := c.doJSON(ctx, http.MethodGet, "/whoami", creds, nil, nil, &view)
	return view, err
}

func (c *Client) CreateBox(ctx context.Context, creds Credentials, req CreateBoxRequest) (BoxView, error) {
	var view BoxView
	err := c.doJSON(ctx, http.MethodPost, "/boxes", creds, nil, req, &view)
	return view, err
}

func (c *Client) Boxes(ctx context.Context, creds Credentials, creator string, label string, state string) ([]BoxView, error) {
	query := map[string]string{}
	if strings.TrimSpace(creator) != "" {
		query["creator"] = strings.TrimSpace(creator)
	}
	if strings.TrimSpace(label) != "" {
		query["label"] = strings.TrimSpace(label)
	}
	if strings.TrimSpace(state) != "" {
		query["state"] = strings.TrimSpace(state)
	}

	var views []BoxView
	err := c.doJSON(ctx, http.MethodGet, "/boxes", creds, query, nil, &views)
	return views, err
}

func (c *Client) Box(ctx context.Context, creds Credentials, boxID string) (BoxView, error) {
	var view BoxView
	err := c.doJSON(ctx, http.MethodGet, "/boxes/"+url.PathEscape(strings.TrimSpace(boxID)), creds, nil, nil, &view)
	return view, err
}

func (c *Client) StopBox(ctx context.Context, creds Credentials, boxID string) (BoxView, error) {
	var view BoxView
	err := c.doJSON(ctx, http.MethodPost, "/boxes/"+url.PathEscape(strings.TrimSpace(boxID))+"/stop", creds, nil, nil, &view)
	return view, err
}

func (c *Client) CommitBox(ctx context.Context, creds Credentials, boxID string) (SnapView, error) {
	var view SnapView
	err := c.doJSON(ctx, http.MethodPost, "/boxes/"+url.PathEscape(strings.TrimSpace(boxID))+"/commit", creds, nil, nil, &view)
	return view, err
}

func (c *Client) RemoveBox(ctx context.Context, creds Credentials, boxID string) (BoxView, error) {
	var view BoxView
	err := c.doJSON(ctx, http.MethodDelete, "/boxes/"+url.PathEscape(strings.TrimSpace(boxID)), creds, nil, nil, &view)
	return view, err
}

func (c *Client) ImportSnap(ctx context.Context, creds Credentials, imageRef string) (SnapView, error) {
	var view SnapView
	err := c.doJSON(ctx, http.MethodPost, "/snaps/import", creds, nil, ImportSnapRequest{ImageRef: strings.TrimSpace(imageRef)}, &view)
	return view, err
}

func (c *Client) Snaps(ctx context.Context, creds Credentials, attached string) ([]SnapView, error) {
	query := map[string]string{}
	if strings.TrimSpace(attached) != "" {
		query["attached"] = strings.TrimSpace(attached)
	}

	var views []SnapView
	err := c.doJSON(ctx, http.MethodGet, "/snaps", creds, query, nil, &views)
	return views, err
}

func (c *Client) Snap(ctx context.Context, creds Credentials, snapID string) (SnapView, error) {
	var view SnapView
	err := c.doJSON(ctx, http.MethodGet, "/snaps/"+url.PathEscape(strings.TrimSpace(snapID)), creds, nil, nil, &view)
	return view, err
}

func (c *Client) RemoveSnap(ctx context.Context, creds Credentials, snapID string) (SnapView, error) {
	var view SnapView
	err := c.doJSON(ctx, http.MethodDelete, "/snaps/"+url.PathEscape(strings.TrimSpace(snapID)), creds, nil, nil, &view)
	return view, err
}

func (c *Client) ExecStream(ctx context.Context, creds Credentials, boxID string, req ExecBoxRequest) (string, io.ReadCloser, error) {
	resp, err := c.http.R().
		SetContext(ctx).
		SetBasicAuth(creds.AK, creds.SK).
		SetHeader("Content-Type", "application/json").
		SetBody(req).
		SetDoNotParseResponse(true).
		Execute(http.MethodPost, "/boxes/"+url.PathEscape(strings.TrimSpace(boxID))+"/execs/stream")
	if err != nil {
		return "", nil, err
	}
	if resp.IsError() {
		return "", nil, rawResponseError(resp)
	}
	return strings.TrimSpace(resp.Header().Get("X-Run9-Exec-ID")), resp.RawBody(), nil
}

func (c *Client) UploadArchive(ctx context.Context, creds Credentials, boxID string, boxAbsPath string, source io.Reader) (RuntimeRequestView, error) {
	var view RuntimeRequestView
	err := c.doStreamJSON(
		ctx,
		http.MethodPost,
		"/boxes/"+url.PathEscape(strings.TrimSpace(boxID))+"/files/upload",
		creds,
		map[string]string{"box_abs_path": strings.TrimSpace(boxAbsPath)},
		"application/x-tar",
		source,
		&view,
	)
	return view, err
}

func (c *Client) DownloadArchive(ctx context.Context, creds Credentials, boxID string, boxAbsPath string) (io.ReadCloser, error) {
	resp, err := c.http.R().
		SetContext(ctx).
		SetBasicAuth(creds.AK, creds.SK).
		SetDoNotParseResponse(true).
		SetQueryParams(map[string]string{
			"archive":      "tar",
			"box_abs_path": strings.TrimSpace(boxAbsPath),
		}).
		Execute(http.MethodGet, "/boxes/"+url.PathEscape(strings.TrimSpace(boxID))+"/files/download")
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, rawResponseError(resp)
	}
	return resp.RawBody(), nil
}

func (c *Client) doJSON(ctx context.Context, method string, path string, creds Credentials, query map[string]string, body any, out any) error {
	req := c.http.R().
		SetContext(ctx).
		SetBasicAuth(creds.AK, creds.SK).
		SetError(&errorPayload{})
	if out != nil {
		req.SetResult(out)
	}
	if query != nil {
		req.SetQueryParams(query)
	}
	if body != nil {
		req.SetHeader("Content-Type", "application/json")
		req.SetBody(body)
	}

	resp, err := req.Execute(method, path)
	if err != nil {
		return err
	}
	if resp.IsError() {
		return jsonResponseError(resp)
	}
	return nil
}

func (c *Client) doStreamJSON(ctx context.Context, method string, path string, creds Credentials, query map[string]string, contentType string, body io.Reader, out any) error {
	req := c.http.R().
		SetContext(ctx).
		SetBasicAuth(creds.AK, creds.SK).
		SetError(&errorPayload{})
	if out != nil {
		req.SetResult(out)
	}
	if query != nil {
		req.SetQueryParams(query)
	}
	if body != nil {
		req.SetBody(body)
	}
	if strings.TrimSpace(contentType) != "" {
		req.SetHeader("Content-Type", contentType)
	}

	resp, err := req.Execute(method, path)
	if err != nil {
		return err
	}
	if resp.IsError() {
		return jsonResponseError(resp)
	}
	return nil
}

func jsonResponseError(resp *resty.Response) error {
	if payload, ok := resp.Error().(*errorPayload); ok && strings.TrimSpace(payload.Error) != "" {
		return &Error{StatusCode: resp.StatusCode(), Message: strings.TrimSpace(payload.Error)}
	}
	message := strings.TrimSpace(resp.String())
	if message == "" {
		message = strings.TrimSpace(resp.Status())
	}
	return &Error{StatusCode: resp.StatusCode(), Message: message}
}

func rawResponseError(resp *resty.Response) error {
	body := resp.RawBody()
	if body == nil {
		return &Error{StatusCode: resp.StatusCode(), Message: strings.TrimSpace(resp.Status())}
	}
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		return &Error{StatusCode: resp.StatusCode(), Message: err.Error()}
	}

	var payload errorPayload
	if len(data) > 0 && json.Unmarshal(data, &payload) == nil && strings.TrimSpace(payload.Error) != "" {
		return &Error{StatusCode: resp.StatusCode(), Message: strings.TrimSpace(payload.Error)}
	}

	message := strings.TrimSpace(string(data))
	if message == "" {
		message = strings.TrimSpace(resp.Status())
	}
	return &Error{StatusCode: resp.StatusCode(), Message: message}
}
