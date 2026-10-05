package emoji

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
	"time"
)

func TestDigestMatchesReferenceConstruction(t *testing.T) {
	files := map[string][]byte{
		"b.emo": []byte("second"),
		"a.emo": []byte("first"),
	}

	// sha256("a.emo\x00first\x00b.emo\x00second\x00"), computed with
	// `printf ... | shasum -a 256`.
	const want = "a530354d1082c1dd90e11ee82c378bf98aa051b55f4c061416c9cf39b1d1c55e"

	if got := Digest(files); got != want {
		t.Fatalf("Digest = %q, want %q", got, want)
	}
}

func TestBuildArchiveIsDeterministic(t *testing.T) {
	files := map[string][]byte{
		"package.emo": []byte("package { name = \"a/b\" version = \"1.0.0\" }"),
		"src/x.emo":   []byte("let x = 1"),
	}

	first, err := BuildArchive(files)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	time.Sleep(time.Millisecond)

	second, err := BuildArchive(files)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if !bytes.Equal(first, second) {
		t.Fatalf("BuildArchive is not deterministic")
	}
}

func TestUnpackBuildArchiveRoundTrip(t *testing.T) {
	files := map[string][]byte{
		"package.emo": []byte("package { name = \"a/b\" version = \"1.0.0\" }"),
		"a.emo":       []byte("aaa"),
		"sub/b.emo":   []byte("bbb"),
	}

	archive, err := BuildArchive(files)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	back, err := Unpack(archive)
	if err != nil {
		t.Fatalf("unpack: %v", err)
	}

	if len(back) != len(files) {
		t.Fatalf("round trip lost files: %#v", back)
	}
	for name, content := range files {
		if !bytes.Equal(back[name], content) {
			t.Fatalf("round trip mismatch for %s", name)
		}
	}
}

func TestUnpackRejectsUnsafeEntries(t *testing.T) {
	build := func(name string, typeflag byte) []byte {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)

		header := &tar.Header{Name: name, Mode: 0644, Typeflag: typeflag, Size: 1}
		if typeflag == tar.TypeSymlink {
			header.Linkname = "/etc/passwd"
			header.Size = 0
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if header.Size > 0 {
			_, _ = tw.Write([]byte("x"))
		}
		_ = tw.Close()
		_ = gz.Close()
		return buf.Bytes()
	}

	for name, archive := range map[string][]byte{
		"path traversal": build("../evil.emo", tar.TypeReg),
		"absolute path":  build("/abs/evil.emo", tar.TypeReg),
		"symlink":        build("link.emo", tar.TypeSymlink),
	} {
		if _, err := Unpack(archive); err == nil {
			t.Fatalf("expected %s archive to be rejected", name)
		}
	}
}

func TestValidate(t *testing.T) {
	if err := Validate(map[string][]byte{"a.emo": []byte("x")}); err == nil {
		t.Fatalf("expected missing package.emo to be rejected")
	}

	if err := Validate(map[string][]byte{
		"package.emo": []byte("x"),
		"README.md":   []byte("x"),
	}); err == nil {
		t.Fatalf("expected non-.emo file to be rejected")
	}

	if err := Validate(map[string][]byte{
		"package.emo":       []byte("x"),
		"EMO-METADATA.json": []byte("{}"),
	}); err != nil {
		t.Fatalf("root EMO-METADATA.json should be allowed: %v", err)
	}

	if err := Validate(map[string][]byte{
		"package.emo":           []byte("x"),
		"sub/EMO-METADATA.json": []byte("{}"),
	}); err == nil {
		t.Fatalf("expected nested EMO-METADATA.json to be rejected")
	}

	oversized := map[string][]byte{
		"package.emo": []byte("x"),
		"big.emo":     make([]byte, MaxArchiveContentsSize),
	}
	if err := Validate(oversized); err == nil {
		t.Fatalf("expected oversized archive to be rejected")
	}
}

func TestProcessUpload(t *testing.T) {
	manifest := "package {\n  name = \"acme/tools\"\n  version = \"0.1.0\"\n  deps { json = \"2.3.1\" }\n}\n"
	files := map[string][]byte{
		"package.emo": []byte(manifest),
		"tools.emo":   []byte("let answer = 42"),
	}

	archive, err := BuildArchive(files)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	result, err := ProcessUpload(archive)
	if err != nil {
		t.Fatalf("process: %v", err)
	}

	if result.Manifest.Name != "acme/tools" || result.Manifest.Version != "0.1.0" {
		t.Fatalf("unexpected manifest: %+v", result.Manifest)
	}
	if result.Manifest.Deps["json"] != "2.3.1" {
		t.Fatalf("unexpected deps: %#v", result.Manifest.Deps)
	}
	if result.Digest != Digest(files) {
		t.Fatalf("digest mismatch")
	}
	if result.ArchiveSHA256 != SHA256Hex(result.Archive) {
		t.Fatalf("archive sha mismatch")
	}
	if _, ok := result.Files[MetadataFileName]; ok {
		t.Fatalf("Files must not contain %s", MetadataFileName)
	}

	// The stored archive carries the metadata file, and stripping it
	// reproduces the original sources — digest stability for clients.
	stored, err := Unpack(result.Archive)
	if err != nil {
		t.Fatalf("unpack stored archive: %v", err)
	}
	metadata, ok := stored[MetadataFileName]
	if !ok {
		t.Fatalf("stored archive is missing %s", MetadataFileName)
	}
	if !strings.Contains(string(metadata), result.Digest) {
		t.Fatalf("metadata does not embed the digest")
	}

	delete(stored, MetadataFileName)
	if Digest(stored) != result.Digest {
		t.Fatalf("digest of unpacked archive must match the published digest")
	}
}

func TestProcessUploadRejectsInvalidManifest(t *testing.T) {
	archive, err := BuildArchive(map[string][]byte{
		"package.emo": []byte("package { name = \"BAD\" version = \"1.0.0\" }"),
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if _, err := ProcessUpload(archive); err == nil {
		t.Fatalf("expected invalid manifest to be rejected")
	}
}
