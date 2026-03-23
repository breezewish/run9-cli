package archive

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildUploadArchiveDirectoryRequiresTrailingSlash(t *testing.T) {
	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "hello.txt"), []byte("hello"), 0o644))

	_, err := BuildUploadArchive(sourceDir, "/work/project")
	require.EqualError(t, err, "directory upload target must end with /")
}

func TestBuildUploadArchiveDirectoryPreservesRootDirectory(t *testing.T) {
	sourceDir := t.TempDir()
	sourceBase := filepath.Base(sourceDir)
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "hello.txt"), []byte("hello"), 0o644))

	upload, err := BuildUploadArchive(sourceDir, "/work/")
	require.NoError(t, err)
	defer func() {
		_ = upload.File.Close()
		_ = os.Remove(upload.File.Name())
	}()
	require.Equal(t, "/work", upload.BoxAbsPath)

	tr := tar.NewReader(upload.File)
	hdr, err := tr.Next()
	require.NoError(t, err)
	require.Equal(t, sourceBase+"/", hdr.Name)

	hdr, err = tr.Next()
	require.NoError(t, err)
	require.Equal(t, sourceBase+"/hello.txt", hdr.Name)
	body, err := io.ReadAll(tr)
	require.NoError(t, err)
	require.Equal(t, "hello", string(body))
}

func TestExtractDownloadArchiveWritesSingleFileToExactDestination(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name:     "ignored.txt",
		Mode:     0o644,
		Size:     5,
		Typeflag: tar.TypeReg,
	}))
	_, err := tw.Write([]byte("hello"))
	require.NoError(t, err)
	require.NoError(t, tw.Close())

	destination := filepath.Join(t.TempDir(), "download.txt")
	require.NoError(t, ExtractDownloadArchive(bytes.NewReader(buf.Bytes()), destination))

	data, err := os.ReadFile(destination)
	require.NoError(t, err)
	require.Equal(t, "hello", string(data))
}
