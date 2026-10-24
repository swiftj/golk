package testutil

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// Recorded is one request the fake server received.
type Recorded struct {
	Method   string
	Path     string
	RawQuery string
	Header   http.Header
	Body     []byte
}

// Reply is one scripted response. If Handler is set it runs instead, after
// the request is recorded.
type Reply struct {
	Status  int
	Header  map[string]string
	Body    string
	Handler http.HandlerFunc
}

// JSON returns a reply with a JSON body.
func JSON(status int, body string) Reply {
	return Reply{Status: status, Header: map[string]string{"Content-Type": "application/json"}, Body: body}
}

// APIError returns a reply in the API's error body shape.
func APIError(status int, code, requestID string) Reply {
	r := JSON(status, `{"message":"scripted `+code+`","code":"`+code+`","requestId":"`+requestID+`"}`)
	r.Header["X-Request-Id"] = requestID
	return r
}

// FakeServer serves scripted replies in order and records every request.
// A request with no reply left fails the test and gets a 599.
type FakeServer struct {
	*httptest.Server
	t       testing.TB
	mu      sync.Mutex
	replies []Reply
	reqs    []Recorded
}

// NewFakeServer starts a server that answers with replies in order. It is
// closed when the test ends.
func NewFakeServer(t testing.TB, replies ...Reply) *FakeServer {
	t.Helper()
	s := &FakeServer{t: t, replies: replies}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// Enqueue appends replies.
func (s *FakeServer) Enqueue(replies ...Reply) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.replies = append(s.replies, replies...)
}

// Requests returns the recorded requests in arrival order.
func (s *FakeServer) Requests() []Recorded {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Recorded(nil), s.reqs...)
}

func (s *FakeServer) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.reqs = append(s.reqs, Recorded{
		Method: r.Method, Path: r.URL.EscapedPath(), RawQuery: r.URL.RawQuery,
		Header: r.Header.Clone(), Body: body,
	})
	var reply Reply
	ok := len(s.replies) > 0
	if ok {
		reply, s.replies = s.replies[0], s.replies[1:]
	}
	s.mu.Unlock()

	if !ok {
		s.t.Errorf("fake server: unexpected request %s %s", r.Method, r.URL)
		w.WriteHeader(599)
		return
	}
	if reply.Handler != nil {
		reply.Handler(w, r)
		return
	}
	for k, v := range reply.Header {
		w.Header().Set(k, v)
	}
	w.WriteHeader(reply.Status)
	_, _ = io.WriteString(w, reply.Body)
}
