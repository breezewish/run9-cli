package archive

import (
	"archive/tar"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// UploadArchive describes one prepared local tar archive and the target box path.
type UploadArchive struct {
	BoxAbsPath string
	File       *os.File
}

// BuildUploadArchive packages one local file or directory into a tar stream.
func BuildUploadArchive(localSource string, rawBoxTarget string) (UploadArchive, error) {
	sourcePath, err := filepath.Abs(strings.TrimSpace(localSource))
	if err != nil {
		return UploadArchive{}, err
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return UploadArchive{}, err
	}

	boxTarget, entryName, err := planUploadTarget(sourcePath, info, rawBoxTarget)
	if err != nil {
		return UploadArchive{}, err
	}

	file, err := os.CreateTemp("", "run9-upload-*.tar")
	if err != nil {
		return UploadArchive{}, err
	}
	if err := writeArchive(file, sourcePath, info, entryName); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return UploadArchive{}, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return UploadArchive{}, err
	}
	return UploadArchive{
		BoxAbsPath: boxTarget,
		File:       file,
	}, nil
}

// ExtractDownloadArchive extracts one downloaded tar archive onto the local filesystem.
func ExtractDownloadArchive(source io.Reader, rawLocalTarget string) error {
	localTarget := filepath.Clean(strings.TrimSpace(rawLocalTarget))
	if localTarget == "" || localTarget == "." {
		return errors.New("local destination is required")
	}

	stageDir, err := os.MkdirTemp("", "run9-download-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stageDir)

	if err := untarInto(stageDir, source); err != nil {
		return err
	}

	singleFilePath, hasSingleFile, err := findSingleRegularFile(stageDir)
	if err != nil {
		return err
	}

	info, err := os.Stat(localTarget)
	if err == nil {
		if info.IsDir() {
			return mergeStageIntoDir(stageDir, localTarget)
		}
		if !hasSingleFile {
			return errors.New("download archive is not a single file, local destination must be a directory")
		}
		return replacePath(singleFilePath, localTarget)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if hasSingleFile && !strings.HasSuffix(strings.TrimSpace(rawLocalTarget), string(os.PathSeparator)) {
		return replacePath(singleFilePath, localTarget)
	}

	if err := os.MkdirAll(localTarget, 0o755); err != nil {
		return err
	}
	return mergeStageIntoDir(stageDir, localTarget)
}

func planUploadTarget(sourcePath string, info os.FileInfo, rawBoxTarget string) (boxAbsPath string, entryName string, err error) {
	boxTarget := strings.TrimSpace(rawBoxTarget)
	if boxTarget == "" {
		return "", "", errors.New("box destination is required")
	}
	boxTargetClean := path.Clean(boxTarget)
	if !path.IsAbs(boxTargetClean) {
		return "", "", errors.New("box path must be absolute")
	}

	destIsDirHint := strings.HasSuffix(boxTarget, "/") || boxTargetClean == "/"
	sourceBase := filepath.Base(sourcePath)
	if sourceBase == "." || sourceBase == string(filepath.Separator) || sourceBase == "" {
		sourceBase = "archive"
	}

	if info.IsDir() {
		if !destIsDirHint {
			return "", "", errors.New("directory upload target must end with /")
		}
		return boxTargetClean, sourceBase, nil
	}
	if destIsDirHint {
		return boxTargetClean, sourceBase, nil
	}
	return path.Dir(boxTargetClean), path.Base(boxTargetClean), nil
}

func writeArchive(file *os.File, sourcePath string, sourceInfo os.FileInfo, rootEntryName string) error {
	tw := tar.NewWriter(file)
	defer tw.Close()

	if sourceInfo.IsDir() {
		return filepath.WalkDir(sourcePath, func(currentPath string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return errors.New("symlink upload is not supported")
			}

			relPath, err := filepath.Rel(sourcePath, currentPath)
			if err != nil {
				return err
			}
			name := rootEntryName
			if relPath != "." {
				name = path.Join(rootEntryName, filepath.ToSlash(relPath))
			}
			return writeEntry(tw, currentPath, info, name)
		})
	}

	return writeEntry(tw, sourcePath, sourceInfo, rootEntryName)
}

func writeEntry(tw *tar.Writer, sourcePath string, info os.FileInfo, archiveName string) error {
	switch {
	case info.IsDir():
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = strings.TrimSuffix(archiveName, "/") + "/"
		return tw.WriteHeader(header)
	case info.Mode().IsRegular():
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = archiveName
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		file, err := os.Open(sourcePath)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(tw, file)
		return err
	default:
		return errors.New("only regular files and directories are supported")
	}
}

func untarInto(root string, source io.Reader) error {
	tr := tar.NewReader(source)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		targetPath, err := archivePath(root, hdr.Name)
		if err != nil {
			return err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, os.FileMode(hdr.Mode)&0o777); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return err
			}
			if _, err := io.Copy(file, tr); err != nil {
				_ = file.Close()
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
		default:
			return errors.New("download archive contains unsupported entry type")
		}
	}
}

func archivePath(root string, rawName string) (string, error) {
	cleanName := path.Clean(strings.TrimPrefix(strings.TrimSpace(rawName), "/"))
	if cleanName == "." || cleanName == "" || strings.HasPrefix(cleanName, "../") || cleanName == ".." {
		return "", errors.New("download archive contains invalid path")
	}

	targetPath := filepath.Join(root, filepath.FromSlash(cleanName))
	rel, err := filepath.Rel(root, targetPath)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("download archive escapes local destination")
	}
	return targetPath, nil
}

func findSingleRegularFile(root string) (string, bool, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", false, err
	}
	if len(entries) != 1 {
		return "", false, nil
	}

	path := filepath.Join(root, entries[0].Name())
	info, err := os.Stat(path)
	if err != nil {
		return "", false, err
	}
	if !info.Mode().IsRegular() {
		return "", false, nil
	}
	return path, true, nil
}

func mergeStageIntoDir(stageDir string, localDir string) error {
	entries, err := os.ReadDir(stageDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		sourcePath := filepath.Join(stageDir, entry.Name())
		targetPath := filepath.Join(localDir, entry.Name())
		if err := replacePath(sourcePath, targetPath); err != nil {
			return err
		}
	}
	return nil
}

func replacePath(sourcePath string, targetPath string) error {
	info, err := os.Lstat(sourcePath)
	if err != nil {
		return err
	}

	if info.IsDir() {
		if err := os.RemoveAll(targetPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.MkdirAll(targetPath, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(sourcePath)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := replacePath(filepath.Join(sourcePath, entry.Name()), filepath.Join(targetPath, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(targetPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	targetFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(targetFile, sourceFile); err != nil {
		_ = targetFile.Close()
		return err
	}
	return targetFile.Close()
}
