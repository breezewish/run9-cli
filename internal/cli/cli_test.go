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
	"github.com/breezewish/run9-cli/internal/buildinfo"
	"github.com/breezewish/run9-cli/internal/config"
	"github.com/stretchr/testify/require"
)

func TestMainVersionFlagPrintsEmbeddedVersion(t *testing.T) {
	previousVersion := buildinfo.Version
	buildinfo.Version = "1.2.3"
	t.Cleanup(func() {
		buildinfo.Version = previousVersion
	})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := Main(context.Background(), []string{"--version"}, stdout, stderr)
	require.Equal(t, 0, exitCode)
	require.Equal(t, "run9 version 1.2.3\n", stdout.String())
	require.Empty(t, stderr.String())
}

func TestMainVersionCommandPrintsEmbeddedVersion(t *testing.T) {
	previousVersion := buildinfo.Version
	buildinfo.Version = "2.0.0-beta.1"
	t.Cleanup(func() {
		buildinfo.Version = previousVersion
	})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := Main(context.Background(), []string{"version"}, stdout, stderr)
	require.Equal(t, 0, exitCode)
	require.Equal(t, "run9 version 2.0.0-beta.1\n", stdout.String())
	require.Empty(t, stderr.String())
}

func TestMainCompletionZshOutputsScript(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := Main(context.Background(), []string{"completion", "zsh"}, stdout, stderr)
	require.Equal(t, 0, exitCode)
	require.Empty(t, stderr.String())
	require.Contains(t, stdout.String(), "#compdef run9")
}

func TestMainBoxExecHelp(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := Main(context.Background(), []string{"box", "exec", "--help"}, stdout, stderr)
	require.Equal(t, 0, exitCode)
	require.Empty(t, stderr.String())
	require.Contains(t, stdout.String(), "run9 box exec <box-id>")
	require.Contains(t, stdout.String(), "Stream one remote exec through portal-api")
}

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

func TestMainBoxExecSendsDeadlineWorkdirAndEnvOverrides(t *testing.T) {
	configPath := writeLoggedInConfig(t, "http://example.invalid")

	var req api.ExecBoxRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/boxes/my-box/execs/stream", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))

		w.Header().Set("Content-Type", "application/x-ndjson")
		encoder := json.NewEncoder(w)
		require.NoError(t, encoder.Encode(api.ExecStreamEvent{Type: "exit", ExitCode: 0}))
	}))
	defer server.Close()

	require.NoError(t, updateEndpoint(configPath, server.URL))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	before := time.Now()
	exitCode := Main(context.Background(), []string{
		"--config", configPath,
		"box", "exec", "my-box",
		"--deadline", "2m",
		"--user", "root",
		"--workdir", "/workspace",
		"-e", "HELLO=world",
		"-e", "LANG=C",
		"/bin/true",
	}, stdout, stderr)
	require.Equal(t, 0, exitCode)
	require.Empty(t, stdout.String())
	require.Empty(t, stderr.String())
	require.Equal(t, []string{"/bin/true"}, req.Command)
	require.Equal(t, "root", req.User)
	require.Equal(t, "/workspace", req.Workdir)
	require.Equal(t, map[string]string{
		"HELLO": "world",
		"LANG":  "C",
	}, req.EnvOverrides)
	require.WithinDuration(t, before.Add(2*time.Minute), req.DeadlineAt, 10*time.Second)
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

func TestMainBoxCopyRejectsTwoBoxPaths(t *testing.T) {
	configPath := writeLoggedInConfig(t, "http://example.invalid")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := Main(context.Background(), []string{
		"--config", configPath,
		"box", "cp", "box-1:/work/a.txt", "box-2:/work/b.txt",
	}, stdout, stderr)
	require.Equal(t, 1, exitCode)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), "exactly one side of box cp must be")
}

func TestMainWhoAmIReturnsNotLoggedInForIncompleteConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "cli.toml")
	require.NoError(t, config.Save(configPath, config.File{
		Endpoint: "http://example.invalid",
		AK:       "ak-1",
	}))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := Main(context.Background(), []string{
		"--config", configPath,
		"auth", "whoami",
	}, stdout, stderr)
	require.Equal(t, 1, exitCode)
	require.Empty(t, stdout.String())
	require.Equal(t, "not logged in\n", stderr.String())
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

func TestMainBoxListPrintsJSONErrorMessage(t *testing.T) {
	configPath := writeLoggedInConfig(t, "http://example.invalid")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/boxes", r.URL.Path)
		writeJSONResponse(t, w, http.StatusBadRequest, map[string]string{
			"error": "invalid state filter",
		})
	}))
	defer server.Close()

	require.NoError(t, updateEndpoint(configPath, server.URL))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := Main(context.Background(), []string{
		"--config", configPath,
		"box", "ls",
		"--state", "broken",
	}, stdout, stderr)
	require.Equal(t, 1, exitCode)
	require.Empty(t, stdout.String())
	require.Equal(t, "invalid state filter\n", stderr.String())
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

func TestMainBoxCreateAllowsGeneratedBoxIDWhenPositionalIsOmitted(t *testing.T) {
	configPath := writeLoggedInConfig(t, "http://example.invalid")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/boxes", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)

		var req api.CreateBoxRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Empty(t, req.BoxID)
		require.Equal(t, defaultBoxShape, req.DesiredShape)
		require.Equal(t, defaultBoxImageRef, req.SourceImageRef)

		writeJSONResponse(t, w, http.StatusCreated, api.BoxView{
			BoxID:        "calm-forest",
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
		"box", "create",
	}, stdout, stderr)
	require.Equal(t, 0, exitCode)
	require.Empty(t, stderr.String())

	var view api.BoxView
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &view))
	require.Equal(t, "calm-forest", view.BoxID)
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

func TestMainBoxCreateRejectsMismatchedImageAliases(t *testing.T) {
	configPath := writeLoggedInConfig(t, "http://example.invalid")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := Main(context.Background(), []string{
		"--config", configPath,
		"box", "create", "alias-box",
		"--image", "public.ecr.aws/docker/library/alpine:3.20",
		"--image-ref", "public.ecr.aws/docker/library/busybox:1.36",
	}, stdout, stderr)
	require.Equal(t, 1, exitCode)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), "--image and --image-ref must match")
}

func TestMainBoxCreateSendsDescriptionAndLabels(t *testing.T) {
	configPath := writeLoggedInConfig(t, "http://example.invalid")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/boxes", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)

		var req api.CreateBoxRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, "named-box", req.BoxID)
		require.Equal(t, "Demo Box", req.Description)
		require.Equal(t, "snap-1", req.SourceSnapID)
		require.Equal(t, map[string]string{
			"env":  "prod",
			"team": "portal",
		}, req.Labels)

		writeJSONResponse(t, w, http.StatusCreated, api.BoxView{
			BoxID:        "named-box",
			DesiredShape: defaultBoxShape,
			Description:  "Demo Box",
			Labels: map[string]string{
				"env":  "prod",
				"team": "portal",
			},
			State: api.BoxState("ready"),
		})
	}))
	defer server.Close()

	require.NoError(t, updateEndpoint(configPath, server.URL))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := Main(context.Background(), []string{
		"--config", configPath,
		"box", "create", "named-box",
		"--snap", "snap-1",
		"--description", "Demo Box",
		"--label", "team=portal",
		"--label", "env=prod",
	}, stdout, stderr)
	require.Equal(t, 0, exitCode)
	require.Empty(t, stderr.String())

	var view api.BoxView
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &view))
	require.Equal(t, "named-box", view.BoxID)
	require.Equal(t, "Demo Box", view.Description)
	require.Equal(t, "portal", view.Labels["team"])
	require.Equal(t, "prod", view.Labels["env"])
}

func TestMainSnapListUsesAttachedFilter(t *testing.T) {
	configPath := writeLoggedInConfig(t, "http://example.invalid")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/snaps", r.URL.Path)
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "true", r.URL.Query().Get("attached"))
		writeJSONResponse(t, w, http.StatusOK, []api.SnapView{{
			SnapID:   "snap-1",
			State:    api.SnapState("available"),
			Attached: true,
		}})
	}))
	defer server.Close()

	require.NoError(t, updateEndpoint(configPath, server.URL))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := Main(context.Background(), []string{
		"--config", configPath,
		"snap", "ls",
		"--attached", "true",
	}, stdout, stderr)
	require.Equal(t, 0, exitCode)
	require.Empty(t, stderr.String())

	var snaps []api.SnapView
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &snaps))
	require.Len(t, snaps, 1)
	require.Equal(t, "snap-1", snaps[0].SnapID)
	require.True(t, snaps[0].Attached)
}

func TestMainSnapListRejectsInvalidAttachedFilter(t *testing.T) {
	configPath := writeLoggedInConfig(t, "http://example.invalid")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := Main(context.Background(), []string{
		"--config", configPath,
		"snap", "ls",
		"--attached", "maybe",
	}, stdout, stderr)
	require.Equal(t, 1, exitCode)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), "--attached must be true or false")
}

func TestMainBoxExecPrintsStreamJSONErrorMessage(t *testing.T) {
	configPath := writeLoggedInConfig(t, "http://example.invalid")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/boxes/box-1/execs/stream", r.URL.Path)
		writeJSONResponse(t, w, http.StatusConflict, map[string]string{
			"error": "box is stopped",
		})
	}))
	defer server.Close()

	require.NoError(t, updateEndpoint(configPath, server.URL))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := Main(context.Background(), []string{
		"--config", configPath,
		"box", "exec", "box-1", "/bin/true",
	}, stdout, stderr)
	require.Equal(t, 1, exitCode)
	require.Empty(t, stdout.String())
	require.Equal(t, "box is stopped\n", stderr.String())
}

func TestStreamExecReturnsCancelledReason(t *testing.T) {
	var body bytes.Buffer
	require.NoError(t, json.NewEncoder(&body).Encode(api.ExecStreamEvent{
		Type:         "cancelled",
		CancelReason: "box is stopping",
	}))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := streamExec(stdout, stderr, bytes.NewReader(body.Bytes()))
	require.Equal(t, 1, exitCode)
	require.Empty(t, stdout.String())
	require.Equal(t, "box is stopping\n", stderr.String())
}

func TestStreamExecReturnsFailureReason(t *testing.T) {
	var body bytes.Buffer
	require.NoError(t, json.NewEncoder(&body).Encode(api.ExecStreamEvent{
		Type:          "error",
		FailureReason: "runtime unavailable",
	}))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := streamExec(stdout, stderr, bytes.NewReader(body.Bytes()))
	require.Equal(t, 1, exitCode)
	require.Empty(t, stdout.String())
	require.Equal(t, "runtime unavailable\n", stderr.String())
}

func TestStreamExecRejectsUnexpectedEOF(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := streamExec(stdout, stderr, bytes.NewReader(nil))
	require.Equal(t, 1, exitCode)
	require.Empty(t, stdout.String())
	require.Equal(t, "exec stream ended unexpectedly\n", stderr.String())
}
