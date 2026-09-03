package playbackgateway

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

const (
	maxLocalMediaRelativePathBytes = 8 * 1024
	maxLocalMediaPathSegmentBytes  = 255
)

var (
	ErrLocalMediaRootInvalid     = errors.New("local media root invalid")
	ErrLocalMediaRootUnsafe      = errors.New("local media root unsafe")
	ErrLocalMediaRootUnavailable = errors.New("local media root unavailable")
	ErrLocalMediaNotFound        = errors.New("local media not found")
	ErrLocalMediaUnsafe          = errors.New("local media path unsafe")
	ErrLocalMediaUnavailable     = errors.New("local media unavailable")
)

// LocalMediaResolver opens one already-mapped relative media path. Returned
// files are pinned descriptors; callers own closing them after the response.
type LocalMediaResolver interface {
	Open(relativePath string) (*os.File, error)
}

// filesystemLocalMediaResolver keeps only the deployment path. Every lookup
// reopens and pins the real root descriptor so a mount replacement cannot turn
// a prior string check into a traversal outside the configured tree.
type filesystemLocalMediaResolver struct {
	root              string
	beforeOpenSegment func(index int)
	afterOpenSegment  func(index int)
}

// newFilesystemLocalMediaResolver validates the optional deployment root
// without making the local feature a Gateway startup dependency. An empty root
// means disabled; invalid or unavailable configured roots return safe sentinels.
func newFilesystemLocalMediaResolver(root string) (*filesystemLocalMediaResolver, error) {
	if root == "" {
		return nil, nil
	}
	if !validLocalMediaRoot(root) {
		return nil, ErrLocalMediaRootInvalid
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, ErrLocalMediaRootUnavailable
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrLocalMediaRootUnsafe
	}
	if !info.IsDir() {
		return nil, ErrLocalMediaRootUnavailable
	}
	fd, err := openDirectoryNoFollow(unix.AT_FDCWD, root)
	if err != nil {
		return nil, classifyLocalRootOpenError(err)
	}
	_ = unix.Close(fd)
	return &filesystemLocalMediaResolver{root: root}, nil
}

// Open walks from a pinned root descriptor with O_NOFOLLOW on every segment.
// It never scans directories, follows links, or opens more than one candidate.
func (resolver *filesystemLocalMediaResolver) Open(relativePath string) (*os.File, error) {
	if resolver == nil || !validLocalMediaRelativePath(relativePath) {
		return nil, ErrLocalMediaUnsafe
	}
	rootFD, err := openDirectoryNoFollow(unix.AT_FDCWD, resolver.root)
	if err != nil {
		return nil, classifyLocalRootOpenError(err)
	}
	currentFD := rootFD
	defer func() {
		if currentFD >= 0 {
			_ = unix.Close(currentFD)
		}
	}()

	segments := strings.Split(relativePath, "/")
	for index, segment := range segments {
		if resolver.beforeOpenSegment != nil {
			resolver.beforeOpenSegment(index)
		}
		last := index == len(segments)-1
		flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
		if !last {
			flags |= unix.O_DIRECTORY
		}
		nextFD, openErr := unix.Openat(currentFD, segment, flags, 0)
		if openErr != nil {
			return nil, classifyLocalPathOpenError(currentFD, segment, openErr)
		}
		var stat unix.Stat_t
		if statErr := unix.Fstat(nextFD, &stat); statErr != nil {
			_ = unix.Close(nextFD)
			return nil, ErrLocalMediaUnavailable
		}
		kind := stat.Mode & unix.S_IFMT
		if (!last && kind != unix.S_IFDIR) || (last && kind != unix.S_IFREG) {
			_ = unix.Close(nextFD)
			return nil, ErrLocalMediaUnavailable
		}
		if resolver.afterOpenSegment != nil {
			resolver.afterOpenSegment(index)
		}
		_ = unix.Close(currentFD)
		currentFD = nextFD
	}

	file := os.NewFile(uintptr(currentFD), filepath.Base(relativePath))
	if file == nil {
		return nil, ErrLocalMediaUnavailable
	}
	currentFD = -1
	return file, nil
}

func validLocalMediaRoot(root string) bool {
	if !utf8.ValidString(root) || !filepath.IsAbs(root) || filepath.Clean(root) != root ||
		root == string(filepath.Separator) || strings.ContainsAny(root, "\\\x00\r\n") {
		return false
	}
	for _, segment := range strings.Split(strings.TrimPrefix(root, string(filepath.Separator)), string(filepath.Separator)) {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func validLocalMediaRelativePath(relativePath string) bool {
	if relativePath == "" || len(relativePath) > maxLocalMediaRelativePathBytes || !utf8.ValidString(relativePath) ||
		strings.HasPrefix(relativePath, "/") || strings.ContainsAny(relativePath, "\\\x00\r\n") {
		return false
	}
	for _, segment := range strings.Split(relativePath, "/") {
		if segment == "" || segment == "." || segment == ".." || len(segment) > maxLocalMediaPathSegmentBytes {
			return false
		}
	}
	return true
}

func openDirectoryNoFollow(parentFD int, path string) (int, error) {
	if parentFD == unix.AT_FDCWD {
		return unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	}
	return unix.Openat(parentFD, path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
}

func classifyLocalRootOpenError(err error) error {
	if errors.Is(err, unix.ELOOP) {
		return ErrLocalMediaRootUnsafe
	}
	return ErrLocalMediaRootUnavailable
}

func classifyLocalPathOpenError(parentFD int, segment string, err error) error {
	if errors.Is(err, unix.ENOENT) {
		return ErrLocalMediaNotFound
	}
	if errors.Is(err, unix.ELOOP) || pathSegmentIsSymlink(parentFD, segment) {
		return ErrLocalMediaUnsafe
	}
	return ErrLocalMediaUnavailable
}

func pathSegmentIsSymlink(parentFD int, segment string) bool {
	var stat unix.Stat_t
	if err := unix.Fstatat(parentFD, segment, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return false
	}
	return stat.Mode&unix.S_IFMT == unix.S_IFLNK
}
