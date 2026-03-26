package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewClientDisablesRestyWarnings(t *testing.T) {
	client := NewClient(" http://example.com/ ")

	require.Equal(t, "http://example.com", client.http.BaseURL)
	require.True(t, client.http.DisableWarn)
	require.Zero(t, client.http.GetClient().Timeout)
}

func TestClientBoxesReturnsJSONErrorMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/boxes", r.URL.Path)
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]string{
			"error": "invalid state filter",
		}))
	}))
	defer server.Close()

	_, err := NewClient(server.URL).Boxes(context.Background(), Credentials{
		AK: "ak-1",
		SK: "sk-1",
	}, "", "", "broken")
	require.Error(t, err)

	var apiErr *Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	require.Equal(t, "invalid state filter", apiErr.Message)
}

func TestClientDownloadArchiveFallsBackToRawBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/boxes/box-1/files/download", r.URL.Path)
		require.Equal(t, http.MethodGet, r.Method)
		w.WriteHeader(http.StatusBadGateway)
		_, err := w.Write([]byte("gateway overloaded"))
		require.NoError(t, err)
	}))
	defer server.Close()

	_, err := NewClient(server.URL).DownloadArchive(context.Background(), Credentials{
		AK: "ak-1",
		SK: "sk-1",
	}, "box-1", "/work/result.txt")
	require.Error(t, err)

	var apiErr *Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
	require.Equal(t, "gateway overloaded", apiErr.Message)
}
