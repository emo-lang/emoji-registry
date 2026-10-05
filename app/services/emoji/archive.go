package emoji

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"
)

// MaxArchiveContentsSize bounds the total uncompressed size of a package.
const MaxArchiveContentsSize = 10 << 20 // 10 MB

// MetadataFileName is the server-generated metadata file injected into the
// stored archive. Clients ignore it when unpacking, so the content digest of
// an unpacked archive still matches the lockfile checksum.
const MetadataFileName = "EMO-METADATA.json"

// ReadmeFileName is the one non-source file authors may ship: a root-level,
// case-sensitive README.md. It never feeds the content digest.
const ReadmeFileName = "README.md"

// Result is the outcome of processing an uploaded .emoji archive.
type Result struct {
	Manifest      *Manifest
	Digest        string
	ArchiveSHA256 string
	Archive       []byte
	// Files holds the .emo sources only (no EMO-METADATA.json).
	Files map[string][]byte
	// Readme holds the root README.md content, nil when absent.
	Readme []byte
}

// Unpack decodes a .emoji archive (gzip tar) into a path → content map.
// Directories are skipped; anything that is not a regular file, or any path
// that is absolute or escapes the archive root, is rejected.
func Unpack(data []byte) (map[string][]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("invalid archive: not gzip: %w", err)
	}
	defer gz.Close()

	files := map[string][]byte{}
	tr := tar.NewReader(gz)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("invalid archive: %w", err)
		}

		if header.Typeflag == tar.TypeDir {
			continue
		}
		if header.Typeflag != tar.TypeReg {
			return nil, fmt.Errorf("invalid archive: %s is not a regular file", header.Name)
		}

		name := path.Clean(strings.TrimPrefix(header.Name, "./"))
		if name == "." || name == "" {
			continue
		}
		if path.IsAbs(name) || strings.HasPrefix(name, "/") {
			return nil, fmt.Errorf("invalid archive: absolute path %q", header.Name)
		}
		if name == ".." || strings.HasPrefix(name, "../") {
			return nil, fmt.Errorf("invalid archive: path %q escapes the archive root", header.Name)
		}

		content, err := io.ReadAll(io.LimitReader(tr, MaxArchiveContentsSize+1))
		if err != nil {
			return nil, fmt.Errorf("invalid archive: reading %s: %w", name, err)
		}
		files[name] = content
	}

	return files, nil
}

// Validate enforces the archive content rules: package.emo must be present,
// every file must be an .emo source (the only exceptions are a root-level
// EMO-METADATA.json and a root-level README.md), and the total size must stay
// within the limit.
func Validate(files map[string][]byte) error {
	if _, ok := files["package.emo"]; !ok {
		return fmt.Errorf("archive must contain package.emo")
	}

	var total int64
	for name, content := range files {
		if !strings.HasSuffix(name, ".emo") {
			if name == MetadataFileName || name == ReadmeFileName {
				total += int64(len(content))
				continue
			}
			return fmt.Errorf("file %q is not allowed: only .emo sources (plus a root README.md) are permitted", name)
		}
		total += int64(len(content))
	}

	if total > MaxArchiveContentsSize {
		return fmt.Errorf("archive contents exceed the %d MB limit", MaxArchiveContentsSize>>20)
	}

	return nil
}

// BuildArchive packs files into a deterministic .emoji archive: paths sorted,
// tar metadata zeroed, gzip stream without a timestamp. The same input map
// always yields the same bytes.
func BuildArchive(files map[string][]byte) ([]byte, error) {
	paths := make([]string, 0, len(files))
	for name := range files {
		paths = append(paths, name)
	}
	sort.Strings(paths)

	var buf bytes.Buffer

	gz, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	tw := tar.NewWriter(gz)

	for _, name := range paths {
		content := files[name]

		header := &tar.Header{
			Name:    name,
			Mode:    0644,
			Size:    int64(len(content)),
			ModTime: time.Time{},
		}
		if err := tw.WriteHeader(header); err != nil {
			return nil, fmt.Errorf("build archive: %w", err)
		}
		if _, err := tw.Write(content); err != nil {
			return nil, fmt.Errorf("build archive: %w", err)
		}
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("build archive: %w", err)
	}
	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("build archive: %w", err)
	}

	return buf.Bytes(), nil
}

// ProcessUpload runs the full server-side publish pipeline over a raw .emoji
// archive: unpack, validate, parse the manifest, compute the content digest,
// generate EMO-METADATA.json and repack into the canonical stored archive.
func ProcessUpload(data []byte) (*Result, error) {
	files, err := Unpack(data)
	if err != nil {
		return nil, err
	}

	if err := Validate(files); err != nil {
		return nil, err
	}

	sources := map[string][]byte{}
	for name, content := range files {
		if strings.HasSuffix(name, ".emo") {
			sources[name] = content
		}
	}

	manifest, err := ParseManifest(files["package.emo"])
	if err != nil {
		return nil, fmt.Errorf("invalid package.emo: %w", err)
	}

	digest := Digest(sources)

	readme := files[ReadmeFileName]

	metadata, err := buildMetadata(sources, readme, digest)
	if err != nil {
		return nil, err
	}

	withMetadata := make(map[string][]byte, len(sources)+2)
	for name, content := range sources {
		withMetadata[name] = content
	}
	if readme != nil {
		withMetadata[ReadmeFileName] = readme
	}
	withMetadata[MetadataFileName] = metadata

	archive, err := BuildArchive(withMetadata)
	if err != nil {
		return nil, err
	}

	return &Result{
		Manifest:      manifest,
		Digest:        digest,
		ArchiveSHA256: SHA256Hex(archive),
		Archive:       archive,
		Files:         sources,
		Readme:        readme,
	}, nil
}

type metadataFile struct {
	Description string            `json:"description"`
	Digest      string            `json:"digest"`
	Files       map[string]string `json:"files"`
}

func buildMetadata(sources map[string][]byte, readme []byte, digest string) ([]byte, error) {
	perFile := make(map[string]string, len(sources)+1)
	for name, content := range sources {
		perFile[name] = SHA256Hex(content)
	}
	if readme != nil {
		perFile[ReadmeFileName] = SHA256Hex(readme)
	}

	metadata := metadataFile{
		Description: "",
		Digest:      digest,
		Files:       perFile,
	}

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("generate %s: %w", MetadataFileName, err)
	}

	return append(data, '\n'), nil
}
