package playbackgateway

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestFilesystemLocalMediaResolverOpensExactRegularFileAndHardLink(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "Movies", "Fixture")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(directory, "fixture.mkv")
	if err := os.WriteFile(original, []byte("fixture-video"), 0o600); err != nil {
		t.Fatal(err)
	}
	hardLink := filepath.Join(directory, "fixture-hardlink.mkv")
	if err := os.Link(original, hardLink); err != nil {
		t.Fatal(err)
	}

	resolver, err := newFilesystemLocalMediaResolver(root)
	if err != nil {
		t.Fatalf("newFilesystemLocalMediaResolver() error = %v", err)
	}
	for _, relativePath := range []string{"Movies/Fixture/fixture.mkv", "Movies/Fixture/fixture-hardlink.mkv"} {
		t.Run(relativePath, func(t *testing.T) {
			file, err := resolver.Open(relativePath)
			if err != nil {
				t.Fatalf("Open(%q) error = %v", relativePath, err)
			}
			defer file.Close()
			body, err := io.ReadAll(file)
			if err != nil || string(body) != "fixture-video" {
				t.Fatalf("ReadAll() = %q, %v", body, err)
			}
		})
	}
}

func TestFilesystemLocalMediaResolverClassifiesMissUnsafeAndUnavailable(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "safe"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "safe", "video.mkv"), []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.mkv"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked-directory")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.mkv"), filepath.Join(root, "linked-file.mkv")); err != nil {
		t.Fatal(err)
	}

	resolver, err := newFilesystemLocalMediaResolver(root)
	if err != nil {
		t.Fatalf("newFilesystemLocalMediaResolver() error = %v", err)
	}
	tests := []struct {
		name         string
		relativePath string
		want         error
	}{
		{name: "missing", relativePath: "safe/missing.mkv", want: ErrLocalMediaNotFound},
		{name: "directory", relativePath: "safe", want: ErrLocalMediaUnavailable},
		{name: "parent traversal", relativePath: "../secret.mkv", want: ErrLocalMediaUnsafe},
		{name: "absolute", relativePath: "/safe/video.mkv", want: ErrLocalMediaUnsafe},
		{name: "backslash ambiguity", relativePath: `safe\video.mkv`, want: ErrLocalMediaUnsafe},
		{name: "linked directory", relativePath: "linked-directory/secret.mkv", want: ErrLocalMediaUnsafe},
		{name: "linked file", relativePath: "linked-file.mkv", want: ErrLocalMediaUnsafe},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := resolver.Open(test.relativePath)
			if file != nil || !errors.Is(err, test.want) {
				t.Fatalf("Open(%q) = (%v, %v), want nil, %v", test.relativePath, file, err, test.want)
			}
		})
	}
}

func TestFilesystemLocalMediaResolverPinsRootDescriptorAgainstReplacement(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "media")
	outside := filepath.Join(parent, "outside")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "fixture.mkv"), []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "fixture.mkv"), []byte("outside-secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	resolver, err := newFilesystemLocalMediaResolver(root)
	if err != nil {
		t.Fatalf("newFilesystemLocalMediaResolver() error = %v", err)
	}
	replaced := false
	resolver.beforeOpenSegment = func(index int) {
		if index != 0 || replaced {
			return
		}
		replaced = true
		oldRoot := root + "-old"
		if err := os.Rename(root, oldRoot); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, root); err != nil {
			t.Fatal(err)
		}
	}

	file, err := resolver.Open("fixture.mkv")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer file.Close()
	body, err := io.ReadAll(file)
	if err != nil || string(body) != "inside" {
		t.Fatalf("ReadAll() = %q, %v; root replacement escaped descriptor", body, err)
	}
}

func TestFilesystemLocalMediaResolverPinsIntermediateDescriptorAgainstReplacement(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "Season")
	outside := t.TempDir()
	if err := os.Mkdir(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inside, "fixture.mkv"), []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "fixture.mkv"), []byte("outside-secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	resolver, err := newFilesystemLocalMediaResolver(root)
	if err != nil {
		t.Fatalf("newFilesystemLocalMediaResolver() error = %v", err)
	}
	replaced := false
	resolver.afterOpenSegment = func(index int) {
		if index != 0 || replaced {
			return
		}
		replaced = true
		if err := os.Rename(inside, inside+"-old"); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, inside); err != nil {
			t.Fatal(err)
		}
	}

	file, err := resolver.Open("Season/fixture.mkv")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer file.Close()
	body, err := io.ReadAll(file)
	if err != nil || string(body) != "inside" {
		t.Fatalf("ReadAll() = %q, %v; intermediate replacement escaped descriptor", body, err)
	}
}

func TestFilesystemLocalMediaResolverValidatesOptionalRoot(t *testing.T) {
	if resolver, err := newFilesystemLocalMediaResolver(""); resolver != nil || err != nil {
		t.Fatalf("empty root = (%v, %v), want disabled", resolver, err)
	}
	for _, root := range []string{"relative/path", "/", "/tmp/../tmp", " /tmp"} {
		t.Run(root, func(t *testing.T) {
			if resolver, err := newFilesystemLocalMediaResolver(root); resolver != nil || !errors.Is(err, ErrLocalMediaRootInvalid) {
				t.Fatalf("root %q = (%v, %v), want ErrLocalMediaRootInvalid", root, resolver, err)
			}
		})
	}

	missing := filepath.Join(t.TempDir(), "missing")
	if resolver, err := newFilesystemLocalMediaResolver(missing); resolver != nil || !errors.Is(err, ErrLocalMediaRootUnavailable) {
		t.Fatalf("missing root = (%v, %v), want ErrLocalMediaRootUnavailable", resolver, err)
	}

	target := t.TempDir()
	linkedRoot := filepath.Join(t.TempDir(), "linked-root")
	if err := os.Symlink(target, linkedRoot); err != nil {
		t.Fatal(err)
	}
	if resolver, err := newFilesystemLocalMediaResolver(linkedRoot); resolver != nil || !errors.Is(err, ErrLocalMediaRootUnsafe) {
		t.Fatalf("linked root = (%v, %v), want ErrLocalMediaRootUnsafe", resolver, err)
	}
}
