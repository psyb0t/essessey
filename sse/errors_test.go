package sse

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nonFlusher implements http.ResponseWriter and deliberately NOT http.Flusher.
// httptest.ResponseRecorder cannot stand in here: it has a Flush method, so
// embedding it would satisfy the very interface this test needs absent.
type nonFlusher struct{}

func (nonFlusher) Header() http.Header         { return http.Header{} }
func (nonFlusher) Write(b []byte) (int, error) { return len(b), nil }
func (nonFlusher) WriteHeader(int)             {}

// The sentinel is only worth exporting if it survives the wrap: a caller
// matching with errors.Is must not have to know what context was added.
func TestNewHTTPSink_NotAFlusherSentinelSurvivesTheWrap(t *testing.T) {
	t.Parallel()

	_, err := NewHTTPSink(nonFlusher{})

	require.ErrorIs(t, err, ErrNotAFlusher)
	assert.Contains(t, err.Error(), "new http sink",
		"the wrap must add call-site context, not replace the sentinel")
}
