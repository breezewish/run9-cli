package cli

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/breezewish/run9-cli/internal/api"
	"github.com/breezewish/run9-cli/internal/config"
	"github.com/stretchr/testify/require"
)

func TestMainAuthLoginWhoAmIAndLogout(t *testing.T) {
	fixtureTime := time.Unix(1_700_000_000, 0).UTC()
	identity := api.CurrentOrgIdentityView{
		User: api.MeView{
			UserID:       "user-1",
			PrimaryEmail: "alice@example.com",
			DisplayName:  "Alice",
			CreatedAt:    fixtureTime,
		},
		Org: api.OrgView{
			OrgID:       "org-1",
			DisplayName: "Alice Personal",
			Kind:        api.OrgKind("personal"),
			Role:        api.MembershipRole("owner"),
			CreatedBy:   "user-1",
			CreatedAt:   fixtureTime,
		},
		AuthKind: "api_key",
	}

	var whoamiCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/whoami", r.URL.Path)
		ak, sk, ok := r.BasicAuth()
		require.True(t, ok)
		require.Equal(t, "ak-1", ak)
		require.Equal(t, "sk-1", sk)
		whoamiCalls++
		writeJSONResponse(t, w, http.StatusOK, identity)
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "cli.toml")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := Main(context.Background(), []string{
		"--config", configPath,
		"auth", "login",
		"--endpoint", server.URL,
		"--ak", "ak-1",
		"--sk", "sk-1",
	}, stdout, stderr)
	require.Equal(t, 0, exitCode)
	require.Empty(t, stderr.String())

	var loginIdentity api.CurrentOrgIdentityView
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &loginIdentity))
	require.Equal(t, identity, loginIdentity)

	cfg, err := config.Load(configPath)
	require.NoError(t, err)
	require.Equal(t, server.URL, cfg.Endpoint)
	require.Equal(t, "ak-1", cfg.AK)
	require.Equal(t, "sk-1", cfg.SK)
	require.Equal(t, "org-1", cfg.Identity.OrgID)

	stdout.Reset()
	stderr.Reset()
	exitCode = Main(context.Background(), []string{
		"--config", configPath,
		"auth", "whoami",
	}, stdout, stderr)
	require.Equal(t, 0, exitCode)
	require.Empty(t, stderr.String())
	require.Equal(t, 2, whoamiCalls)

	stdout.Reset()
	stderr.Reset()
	exitCode = Main(context.Background(), []string{
		"--config", configPath,
		"auth", "logout",
	}, stdout, stderr)
	require.Equal(t, 0, exitCode)
	require.Empty(t, stdout.String())
	require.Empty(t, stderr.String())

	_, exists, err := config.LoadOptional(configPath)
	require.NoError(t, err)
	require.False(t, exists)
}

func TestMainAuthLoginReusesSavedEndpoint(t *testing.T) {
	fixtureTime := time.Unix(1_700_000_000, 0).UTC()
	identity := api.CurrentOrgIdentityView{
		User: api.MeView{
			UserID:       "user-1",
			PrimaryEmail: "alice@example.com",
			DisplayName:  "Alice",
			CreatedAt:    fixtureTime,
		},
		Org: api.OrgView{
			OrgID:       "org-1",
			DisplayName: "Alice Personal",
			Kind:        api.OrgKind("personal"),
			Role:        api.MembershipRole("owner"),
			CreatedBy:   "user-1",
			CreatedAt:   fixtureTime,
		},
		AuthKind: "api_key",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/whoami", r.URL.Path)
		ak, sk, ok := r.BasicAuth()
		require.True(t, ok)
		require.Equal(t, "ak-2", ak)
		require.Equal(t, "sk-2", sk)
		writeJSONResponse(t, w, http.StatusOK, identity)
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "cli.toml")
	require.NoError(t, config.Save(configPath, config.File{
		Endpoint: server.URL,
		AK:       "ak-old",
		SK:       "sk-old",
	}))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := Main(context.Background(), []string{
		"--config", configPath,
		"auth", "login",
		"--ak", "ak-2",
		"--sk", "sk-2",
	}, stdout, stderr)
	require.Equal(t, 0, exitCode)
	require.Empty(t, stderr.String())

	cfg, err := config.Load(configPath)
	require.NoError(t, err)
	require.Equal(t, server.URL, cfg.Endpoint)
	require.Equal(t, "ak-2", cfg.AK)
	require.Equal(t, "sk-2", cfg.SK)
}

func TestMainBoxExecStreamsOutput(t *testing.T) {
	configPath := writeLoggedInConfig(t, "http://example.invalid")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/boxes/box-1/execs/stream", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		ak, sk, ok := r.BasicAuth()
		require.True(t, ok)
		require.Equal(t, "ak-1", ak)
		require.Equal(t, "sk-1", sk)

		var req api.ExecBoxRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, []string{"printf", "hello"}, req.Command)

		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("X-Run9-Exec-ID", "exec-1")
		encoder := json.NewEncoder(w)
		require.NoError(t, encoder.Encode(api.ExecStreamEvent{Type: "started"}))
		require.NoError(t, encoder.Encode(api.ExecStreamEvent{Type: "stdout", Data: []byte("hello\n")}))
		require.NoError(t, encoder.Encode(api.ExecStreamEvent{Type: "stderr", Data: []byte("warn\n")}))
		require.NoError(t, encoder.Encode(api.ExecStreamEvent{Type: "exit", ExitCode: 0}))
	}))
	defer server.Close()

	require.NoError(t, updateEndpoint(configPath, server.URL))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := Main(context.Background(), []string{
		"--config", configPath,
		"box", "exec", "box-1", "printf", "hello",
	}, stdout, stderr)
	require.Equal(t, 0, exitCode)
	require.Equal(t, "hello\n", stdout.String())
	require.Equal(t, "warn\n", stderr.String())
}

func TestMainBoxExecSupportsFlagsWithoutCommandDelimiter(t *testing.T) {
	configPath := writeLoggedInConfig(t, "http://example.invalid")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/boxes/my-box/execs/stream", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)

		var req api.ExecBoxRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, []string{"/bin/sh", "-lc", "echo hello"}, req.Command)
		require.Equal(t, "root", req.User)

		w.Header().Set("Content-Type", "application/x-ndjson")
		encoder := json.NewEncoder(w)
		require.NoError(t, encoder.Encode(api.ExecStreamEvent{Type: "exit", ExitCode: 0}))
	}))
	defer server.Close()

	require.NoError(t, updateEndpoint(configPath, server.URL))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := Main(context.Background(), []string{
		"--config", configPath,
		"box", "exec", "my-box", "--user", "root", "/bin/sh", "-lc", "echo hello",
	}, stdout, stderr)
	require.Equal(t, 0, exitCode)
	require.Empty(t, stdout.String())
	require.Empty(t, stderr.String())
}

func TestMainBoxExecSupportsExplicitCommandDelimiter(t *testing.T) {
	configPath := writeLoggedInConfig(t, "http://example.invalid")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/boxes/my-box/execs/stream", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)

		var req api.ExecBoxRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, []string{"--user", "remote-flag-value"}, req.Command)
		require.Empty(t, req.User)

		w.Header().Set("Content-Type", "application/x-ndjson")
		encoder := json.NewEncoder(w)
		require.NoError(t, encoder.Encode(api.ExecStreamEvent{Type: "exit", ExitCode: 0}))
	}))
	defer server.Close()

	require.NoError(t, updateEndpoint(configPath, server.URL))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := Main(context.Background(), []string{
		"--config", configPath,
		"box", "exec", "my-box", "--", "--user", "remote-flag-value",
	}, stdout, stderr)
	require.Equal(t, 0, exitCode)
	require.Empty(t, stdout.String())
	require.Empty(t, stderr.String())
}

func TestMainBoxCopyUploadsAndDownloadsArchives(t *testing.T) {
	configPath := writeLoggedInConfig(t, "http://example.invalid")

	localSource := filepath.Join(t.TempDir(), "source.txt")
	require.NoError(t, os.WriteFile(localSource, []byte("upload body"), 0o644))
	localDest := filepath.Join(t.TempDir(), "download.txt")

	var uploadedArchive []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/boxes/box-1/files/upload":
			require.Equal(t, "/work", r.URL.Query().Get("box_abs_path"))
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			uploadedArchive = body
			writeJSONResponse(t, w, http.StatusOK, api.RuntimeRequestView{
				RuntimeRequestID: "runtime-upload-1",
				State:            "prepared",
				SessionID:        "upload-1",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/boxes/box-1/files/download":
			require.Equal(t, "tar", r.URL.Query().Get("archive"))
			require.Equal(t, "/work/result.txt", r.URL.Query().Get("box_abs_path"))
			w.Header().Set("Content-Type", "application/x-tar")
			_, err := w.Write(tarSingleFile(t, "ignored.txt", []byte("download body")))
			require.NoError(t, err)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	require.NoError(t, updateEndpoint(configPath, server.URL))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := Main(context.Background(), []string{
		"--config", configPath,
		"box", "cp", localSource, "box-1:/work/renamed.txt",
	}, stdout, stderr)
	require.Equal(t, 0, exitCode)
	require.Empty(t, stdout.String())
	require.Empty(t, stderr.String())
	require.NotEmpty(t, uploadedArchive)

	entryName, entryBody := untarSingleFile(t, uploadedArchive)
	require.Equal(t, "renamed.txt", entryName)
	require.Equal(t, "upload body", string(entryBody))

	stdout.Reset()
	stderr.Reset()
	exitCode = Main(context.Background(), []string{
		"--config", configPath,
		"box", "cp", "box-1:/work/result.txt", localDest,
	}, stdout, stderr)
	require.Equal(t, 0, exitCode)
	require.Empty(t, stdout.String())
	require.Empty(t, stderr.String())

	data, err := os.ReadFile(localDest)
	require.NoError(t, err)
	require.Equal(t, "download body", string(data))
}

func writeLoggedInConfig(t *testing.T, endpoint string) string {
	t.Helper()

	configPath := filepath.Join(t.TempDir(), "cli.toml")
	require.NoError(t, config.Save(configPath, config.File{
		Endpoint: endpoint,
		AK:       "ak-1",
		SK:       "sk-1",
		Identity: config.Identity{
			AuthKind:       "api_key",
			UserID:         "user-1",
			PrimaryEmail:   "alice@example.com",
			DisplayName:    "Alice",
			OrgID:          "org-1",
			OrgDisplayName: "Alice Personal",
			OrgKind:        "personal",
			Role:           "owner",
		},
	}))
	return configPath
}

func updateEndpoint(configPath string, endpoint string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	cfg.Endpoint = endpoint
	return config.Save(configPath, cfg)
}

func writeJSONResponse(t *testing.T, w http.ResponseWriter, status int, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	require.NoError(t, json.NewEncoder(w).Encode(value))
}

func tarSingleFile(t *testing.T, name string, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name:     name,
		Mode:     0o644,
		Size:     int64(len(body)),
		Typeflag: tar.TypeReg,
	}))
	_, err := tw.Write(body)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	return buf.Bytes()
}

func untarSingleFile(t *testing.T, archive []byte) (string, []byte) {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(archive))
	hdr, err := tr.Next()
	require.NoError(t, err)
	require.Equal(t, byte(tar.TypeReg), hdr.Typeflag)
	body, err := io.ReadAll(tr)
	require.NoError(t, err)
	_, err = tr.Next()
	require.Equal(t, io.EOF, err)
	return hdr.Name, body
}

func TestParseBoxPathRejectsRelativePath(t *testing.T) {
	_, ok, err := parseBoxPath("box-1:relative/path")
	require.False(t, ok)
	require.EqualError(t, err, "box path must be <box-id>:/absolute/path")

	parsed, ok, err := parseBoxPath("box-1:/workspace")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, boxPath{BoxID: "box-1", AbsPath: "/workspace", RawPath: "/workspace"}, parsed)

	parsed, ok, err = parseBoxPath("box-1:/workspace/")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "/workspace", parsed.AbsPath)
	require.Equal(t, "/workspace/", parsed.RawPath)
}

func TestMainBoxCopyRejectsTwoLocalPaths(t *testing.T) {
	configPath := writeLoggedInConfig(t, "http://example.invalid")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := Main(context.Background(), []string{
		"--config", configPath,
		"box", "cp", "foo.txt", "bar.txt",
	}, stdout, stderr)
	require.Equal(t, 1, exitCode)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), "exactly one side of box cp must be")
}

func TestMainBoxListUsesFilters(t *testing.T) {
	configPath := writeLoggedInConfig(t, "http://example.invalid")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/boxes", r.URL.Path)
		values, err := url.ParseQuery(r.URL.RawQuery)
		require.NoError(t, err)
		require.Equal(t, "alice", values.Get("creator"))
		require.Equal(t, "team=portal", values.Get("label"))
		require.Equal(t, "ready", values.Get("state"))
		writeJSONResponse(t, w, http.StatusOK, []api.BoxView{})
	}))
	defer server.Close()

	require.NoError(t, updateEndpoint(configPath, server.URL))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := Main(context.Background(), []string{
		"--config", configPath,
		"box", "ls",
		"--creator", "alice",
		"--label", "team=portal",
		"--state", "ready",
	}, stdout, stderr)
	require.Equal(t, 0, exitCode)
	require.Empty(t, stderr.String())
	require.JSONEq(t, "[]", stdout.String())
}

func TestMainBoxCreateSupportsPositionalBoxIDAndDefaults(t *testing.T) {
	configPath := writeLoggedInConfig(t, "http://example.invalid")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/boxes", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)

		var req api.CreateBoxRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, "my-box", req.BoxID)
		require.Equal(t, defaultBoxShape, req.DesiredShape)
		require.Equal(t, defaultBoxImageRef, req.SourceImageRef)
		require.Empty(t, req.SourceSnapID)

		writeJSONResponse(t, w, http.StatusCreated, api.BoxView{
			BoxID:        "my-box",
			DesiredShape: defaultBoxShape,
			State:        api.BoxState("ready"),
		})
	}))
	defer server.Close()

	require.NoError(t, updateEndpoint(configPath, server.URL))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := Main(context.Background(), []string{
		"--config", configPath,
		"box", "create", "my-box",
	}, stdout, stderr)
	require.Equal(t, 0, exitCode)
	require.Empty(t, stderr.String())

	var view api.BoxView
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &view))
	require.Equal(t, "my-box", view.BoxID)
	require.Equal(t, defaultBoxShape, view.DesiredShape)
}

func TestMainBoxCreateSupportsPositionalBoxIDWithExplicitShapeAndImage(t *testing.T) {
	configPath := writeLoggedInConfig(t, "http://example.invalid")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/boxes", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)

		var req api.CreateBoxRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, "my-box", req.BoxID)
		require.Equal(t, "2c4g", req.DesiredShape)
		require.Equal(t, "public.ecr.aws/docker/library/alpine:3.20", req.SourceImageRef)

		writeJSONResponse(t, w, http.StatusCreated, api.BoxView{
			BoxID:        "my-box",
			DesiredShape: "2c4g",
			State:        api.BoxState("ready"),
		})
	}))
	defer server.Close()

	require.NoError(t, updateEndpoint(configPath, server.URL))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := Main(context.Background(), []string{
		"--config", configPath,
		"box", "create", "my-box",
		"--shape", "2c4g",
		"--image", "public.ecr.aws/docker/library/alpine:3.20",
	}, stdout, stderr)
	require.Equal(t, 0, exitCode)
	require.Empty(t, stderr.String())

	var view api.BoxView
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &view))
	require.Equal(t, "my-box", view.BoxID)
	require.Equal(t, "2c4g", view.DesiredShape)
}

func TestMainBoxCreateSupportsImageRefAlias(t *testing.T) {
	configPath := writeLoggedInConfig(t, "http://example.invalid")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/boxes", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)

		var req api.CreateBoxRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, "alias-box", req.BoxID)
		require.Equal(t, "public.ecr.aws/docker/library/alpine:3.20", req.SourceImageRef)

		writeJSONResponse(t, w, http.StatusCreated, api.BoxView{
			BoxID:        "alias-box",
			DesiredShape: defaultBoxShape,
			State:        api.BoxState("ready"),
		})
	}))
	defer server.Close()

	require.NoError(t, updateEndpoint(configPath, server.URL))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := Main(context.Background(), []string{
		"--config", configPath,
		"box", "create", "alias-box",
		"--image-ref", "public.ecr.aws/docker/library/alpine:3.20",
	}, stdout, stderr)
	require.Equal(t, 0, exitCode)
	require.Empty(t, stderr.String())

	var view api.BoxView
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &view))
	require.Equal(t, "alias-box", view.BoxID)
}
