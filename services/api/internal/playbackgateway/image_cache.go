package playbackgateway

import (
	"bytes"
	"container/list"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	logpkg "github.com/konghang/ember/backend/internal/logging"
)

const (
	imageCacheMaxEntries = 256
	imageCacheMaxBytes   = 32 << 20
	imageCacheMaxBody    = 2 << 20
	imageCacheFreshLimit = 5 * time.Minute
	imageCacheRetention  = 10 * time.Minute
	imageCacheMaxFlights = 16
	imageCacheMaxWaiters = 64
)

type cachedImage struct {
	header                            http.Header
	body                              []byte
	storedAt, freshUntil, retainUntil time.Time
	age                               time.Duration
}

type imageCacheEntry struct {
	key   string
	image *cachedImage
}

type imageCacheFlight struct {
	key       string
	done      chan struct{}
	cancel    context.CancelFunc
	waiters   map[*imageCacheWaiter]struct{}
	finished  bool
	handedOff bool
	image     *cachedImage
	raw       *http.Response
	err       error
}

type imageCacheWaiter struct{ ctx context.Context }

type imageCachingTransport struct {
	base                                                  http.RoundTripper
	logger                                                *log.Logger
	now                                                   func() time.Time
	mu                                                    sync.Mutex
	entries                                               map[string]*list.Element
	lru                                                   *list.List
	bytes                                                 int
	flights                                               map[string]*imageCacheFlight
	maxEntries, maxBytes, maxBody, maxFlights, maxWaiters int
}

// newImageCachingTransport caches only exact image responses reached after
// Gateway routing/authentication; the caller's raw transport stays untouched.
func newImageCachingTransport(base http.RoundTripper, logger *log.Logger) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &imageCachingTransport{base: base, logger: logger, now: time.Now,
		entries: make(map[string]*list.Element), lru: list.New(), flights: make(map[string]*imageCacheFlight),
		maxEntries: imageCacheMaxEntries, maxBytes: imageCacheMaxBytes, maxBody: imageCacheMaxBody,
		maxFlights: imageCacheMaxFlights, maxWaiters: imageCacheMaxWaiters}
}

// RoundTrip serves fresh GET/HEAD snapshots or joins one image fetch. Client
// refresh directives, preconditions and unsupported traffic stay transparent.
func (cache *imageCachingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	key, force, eligible := imageCacheRequestKey(request)
	if !eligible || force {
		return cache.base.RoundTrip(request)
	}
	if err := request.Context().Err(); err != nil {
		return nil, err
	}
	now := cache.now()
	cache.mu.Lock()
	previous := cache.lookupLocked(key, now)
	if previous != nil && now.Before(previous.freshUntil) {
		cache.mu.Unlock()
		cache.observe(request, "hit")
		return cachedImageResponse(previous, request, now), nil
	}
	if request.Method == http.MethodHead {
		cache.mu.Unlock()
		return cache.base.RoundTrip(request)
	}
	flight := cache.flights[key]
	owner := flight == nil
	var sharedRequest *http.Request
	if owner && len(cache.flights) >= cache.maxFlights || flight != nil && len(flight.waiters) >= cache.maxWaiters {
		cache.mu.Unlock()
		return cache.base.RoundTrip(request)
	}
	if owner {
		shared, cancel := context.WithCancel(context.WithoutCancel(request.Context()))
		flight = &imageCacheFlight{key: key, done: make(chan struct{}), cancel: cancel, waiters: make(map[*imageCacheWaiter]struct{})}
		cache.flights[key] = flight
		sharedRequest = request.Clone(shared)
	}
	waiter := &imageCacheWaiter{ctx: request.Context()}
	flight.waiters[waiter] = struct{}{}
	cache.mu.Unlock()
	if owner {
		go cache.fetch(sharedRequest, key, previous, flight)
	}
	defer cache.leave(flight, waiter)
	select {
	case <-request.Context().Done():
		return nil, request.Context().Err()
	case <-flight.done:
		return cache.flightResponse(request, flight)
	}
}

// lookupLocked returns immutable retained metadata, dropping expired entries
// and rejecting clock rollback rather than extending a previous lifetime.
func (cache *imageCachingTransport) lookupLocked(key string, now time.Time) *cachedImage {
	element := cache.entries[key]
	if element == nil {
		return nil
	}
	image := element.Value.(imageCacheEntry).image
	if now.Before(image.storedAt) || !now.Before(image.retainUntil) {
		cache.removeLocked(element)
		return nil
	}
	cache.lru.MoveToFront(element)
	return image
}

// putLocked bounds both metadata count and retained body bytes before adding
// an immutable image; access never prolongs freshness or retention.
func (cache *imageCachingTransport) putLocked(key string, image *cachedImage) {
	if old := cache.entries[key]; old != nil {
		cache.removeLocked(old)
	}
	if cache.maxEntries <= 0 || len(image.body) > cache.maxBytes {
		return
	}
	for cache.lru.Len() >= cache.maxEntries || cache.bytes+len(image.body) > cache.maxBytes {
		cache.removeLocked(cache.lru.Back())
	}
	cache.entries[key] = cache.lru.PushFront(imageCacheEntry{key: key, image: image})
	cache.bytes += len(image.body)
}

// removeLocked removes one LRU member and its exact byte contribution.
func (cache *imageCachingTransport) removeLocked(element *list.Element) {
	if element == nil {
		return
	}
	entry := element.Value.(imageCacheEntry)
	delete(cache.entries, entry.key)
	cache.bytes -= len(entry.image.body)
	cache.lru.Remove(element)
}

// leave cancels work only when its final independent waiter leaves. A raw
// streaming response instead owns cancellation until its body closes.
func (cache *imageCachingTransport) leave(flight *imageCacheFlight, waiter *imageCacheWaiter) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	delete(flight.waiters, waiter)
	if len(flight.waiters) != 0 || flight.handedOff {
		return
	}
	flight.cancel()
	if cache.flights[flight.key] == flight {
		delete(cache.flights, flight.key)
	}
	if flight.finished && flight.raw != nil && flight.raw.Body != nil {
		_ = flight.raw.Body.Close()
		flight.raw = nil
	}
}

// flightResponse copies successful immutable data, but hands an uncacheable
// response to one waiter only; all others perform their own transparent call.
func (cache *imageCachingTransport) flightResponse(request *http.Request, flight *imageCacheFlight) (*http.Response, error) {
	if err := request.Context().Err(); err != nil {
		return nil, err
	}
	cache.mu.Lock()
	image, err := flight.image, flight.err
	if image != nil || err != nil {
		cache.mu.Unlock()
		if err != nil {
			cache.observe(request, "error")
			return nil, err
		}
		cache.observe(request, "filled")
		return cachedImageResponse(image, request, cache.now()), nil
	}
	response := flight.raw
	if response != nil && !flight.handedOff {
		flight.raw, flight.handedOff = nil, true
		if response.Body == nil {
			response.Body = http.NoBody
		}
		stop := context.AfterFunc(request.Context(), flight.cancel)
		response.Body = &imageCacheBody{ReadCloser: response.Body, close: func() { stop(); flight.cancel() }}
		response.Request = request
		cache.mu.Unlock()
		cache.observe(request, "bypass")
		return response, nil
	}
	cache.mu.Unlock()
	return cache.base.RoundTrip(request)
}

// fetch publishes a result only while at least one waiter still needs it.
// Errors and noncacheable bodies never become shared cache entries.
func (cache *imageCachingTransport) fetch(request *http.Request, key string, previous *cachedImage, flight *imageCacheFlight) {
	var image *cachedImage
	var raw *http.Response
	var err error
	defer func() {
		if recover() != nil {
			image, err = nil, errors.New("image cache fetch failed")
			if raw != nil && raw.Body != nil {
				_ = raw.Body.Close()
			}
			raw = nil
		}
		cache.completeFlight(request, key, previous, flight, image, raw, err)
	}()
	image, raw, err = cache.fetchImage(request, previous)
}

// completeFlight removes only its own in-flight generation and publishes a
// bounded success before waking waiters, including recovered worker failures.
func (cache *imageCachingTransport) completeFlight(request *http.Request, key string, previous *cachedImage, flight *imageCacheFlight, image *cachedImage, raw *http.Response, err error) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	live := false
	for waiter := range flight.waiters {
		live = live || waiter.ctx.Err() == nil
	}
	if request.Context().Err() != nil || !live {
		if raw != nil && raw.Body != nil {
			_ = raw.Body.Close()
		}
		image, raw, err = nil, nil, request.Context().Err()
		if err == nil {
			err = context.Canceled
		}
	}
	if image != nil {
		cache.putLocked(key, image)
	} else if previous != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		if element := cache.entries[key]; element != nil && element.Value.(imageCacheEntry).image == previous {
			cache.removeLocked(element)
		}
	}
	flight.image, flight.raw, flight.err, flight.finished = image, raw, err, true
	if cache.flights[key] == flight {
		delete(cache.flights, key)
	}
	close(flight.done)
	if raw == nil {
		flight.cancel()
	}
}

// fetchImage revalidates a retained representation with its own validators.
// A 304 is never forwarded to an unconditional downstream request.
func (cache *imageCachingTransport) fetchImage(request *http.Request, previous *cachedImage) (*cachedImage, *http.Response, error) {
	started := cache.now()
	upstream := request.Clone(request.Context())
	if previous != nil {
		if tag := previous.header.Get("ETag"); tag != "" {
			upstream.Header.Set("If-None-Match", tag)
		} else if modified := previous.header.Get("Last-Modified"); modified != "" {
			upstream.Header.Set("If-Modified-Since", modified)
		}
	}
	response, err := cache.base.RoundTrip(upstream)
	if err != nil {
		return nil, response, err
	}
	if response == nil {
		return nil, nil, errors.New("image upstream response missing")
	}
	if response.StatusCode == http.StatusNotModified && previous != nil {
		if response.Body != nil {
			_ = response.Body.Close()
		}
		for _, field := range []string{"Content-Encoding", "Content-Type"} {
			if value := response.Header.Get(field); value != "" && value != previous.header.Get(field) {
				return cache.readImageResponse(request)
			}
		}
		header := previous.header.Clone()
		for key, values := range response.Header {
			if http.CanonicalHeaderKey(key) == "Content-Length" {
				continue
			}
			header[key] = append([]string(nil), values...)
		}
		if response.Header.Get("Age") == "" {
			header.Del("Age")
		}
		merged := &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(bytes.NewReader(previous.body)), ContentLength: int64(len(previous.body))}
		if image, ok := imageFromResponse(merged, previous.body, cache.now(), started); ok {
			return image, nil, nil
		}
		// The 304 still validated these bytes. Preserve changed policy and
		// cookie headers for one caller, without caching or sharing the result.
		return nil, merged, nil
	}
	return cache.bufferImageResponse(response, started)
}

// readImageResponse retries without internally added validators when upstream
// changed the response policy during a conditional validation.
func (cache *imageCachingTransport) readImageResponse(request *http.Request) (*cachedImage, *http.Response, error) {
	started := cache.now()
	response, err := cache.base.RoundTrip(request)
	if err != nil {
		return nil, response, err
	}
	if response == nil {
		return nil, nil, errors.New("image upstream response missing")
	}
	return cache.bufferImageResponse(response, started)
}

// bufferImageResponse reads at most one bounded image plus an overflow byte.
// On overflow/read failure the consumed prefix and original error are replayed.
func (cache *imageCachingTransport) bufferImageResponse(response *http.Response, started time.Time) (*cachedImage, *http.Response, error) {
	if !imageResponseEligible(response) || response.Body == nil || response.ContentLength > int64(cache.maxBody) {
		return nil, response, nil
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, int64(cache.maxBody)+1))
	if err != nil || len(body) > cache.maxBody {
		parts := []io.Reader{bytes.NewReader(body)}
		if err != nil {
			parts = append(parts, &imageCacheReadError{err: err})
		}
		parts = append(parts, response.Body)
		response.Body = &imageReplayBody{Reader: io.MultiReader(parts...), closer: response.Body}
		return nil, response, nil
	}
	_ = response.Body.Close()
	if response.ContentLength >= 0 && int64(len(body)) != response.ContentLength {
		response.Body = &imageReplayBody{Reader: io.MultiReader(bytes.NewReader(body), &imageCacheReadError{err: io.ErrUnexpectedEOF})}
		return nil, response, nil
	}
	image, ok := imageFromResponse(response, body, cache.now(), started)
	if ok {
		return image, nil, nil
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	return nil, response, nil
}

// cachedImageResponse clones response metadata and readers for each caller;
// HEAD carries the GET representation length without returning image bytes.
func cachedImageResponse(image *cachedImage, request *http.Request, now time.Time) *http.Response {
	header := image.header.Clone()
	age := image.age
	if elapsed := now.Sub(image.storedAt); elapsed > 0 {
		if elapsed > time.Duration(1<<63-1)-age {
			age = time.Duration(1<<63 - 1)
		} else {
			age += elapsed
		}
	}
	if age < 0 {
		age = 0
	}
	header.Set("Age", strconv.FormatInt(int64(age/time.Second), 10))
	length := int64(len(image.body))
	header.Set("Content-Length", strconv.FormatInt(length, 10))
	reader := io.Reader(bytes.NewReader(image.body))
	if request.Method == http.MethodHead {
		reader = bytes.NewReader(nil)
	}
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: header, Body: io.NopCloser(reader), ContentLength: length, Request: request}
}

// observe reports fixed cache outcomes without recording URL, headers, tokens
// or image bytes; it follows the process's existing dynamic Debug level.
func (cache *imageCachingTransport) observe(request *http.Request, outcome string) {
	if cache.logger != nil && logpkg.DebugEnabled() {
		cache.logger.Printf("[PlaybackGateway] level=debug code=image_cache_result requestId=%s outcome=%s", diagnosticRequestID(request.Context()), outcome)
	}
}

type imageCacheBody struct {
	io.ReadCloser
	close func()
	once  sync.Once
}

// Close hands cancellation back from a transparent raw response exactly once.
func (body *imageCacheBody) Close() error {
	body.once.Do(body.close)
	if body.ReadCloser == nil {
		return nil
	}
	return body.ReadCloser.Close()
}

type imageReplayBody struct {
	io.Reader
	closer io.Closer
}

// Close preserves the underlying upstream body's cleanup after prefix replay.
func (body *imageReplayBody) Close() error {
	if body.closer != nil {
		return body.closer.Close()
	}
	return nil
}

type imageCacheReadError struct{ err error }

// Read replays one original body failure after its already consumed prefix.
func (reader *imageCacheReadError) Read([]byte) (int, error) {
	if reader.err == nil {
		return 0, io.EOF
	}
	err := reader.err
	reader.err = nil
	return 0, err
}
