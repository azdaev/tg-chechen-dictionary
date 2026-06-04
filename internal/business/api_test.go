package business

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirupsen/logrus"
)

func stubDoshamAPI(t *testing.T, status int, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DOSHAM_API_URL", srv.URL)
}

func TestFetchTranslations_EmptyAnswerIsNotNil(t *testing.T) {
	stubDoshamAPI(t, http.StatusOK, `{"data":{"find":[]}}`)
	b := &Business{log: logrus.New()}

	got := b.fetchTranslationsWithFallback("яблоками")
	if got == nil {
		t.Fatal("real empty answer must be non-nil so it gets negative-cached")
	}
	if len(got) != 0 {
		t.Fatalf("expected no pairs, got %d", len(got))
	}
}

func TestFetchTranslations_HTTPErrorIsNil(t *testing.T) {
	stubDoshamAPI(t, http.StatusInternalServerError, `{"data":{"find":[]}}`)
	b := &Business{log: logrus.New()}

	if got := b.fetchTranslationsWithFallback("яблоками"); got != nil {
		t.Fatalf("HTTP failure must return nil so it is not cached, got %#v", got)
	}
}

func TestFetchTranslations_GraphQLErrorIsNil(t *testing.T) {
	stubDoshamAPI(t, http.StatusOK, `{"errors":[{"message":"internal error"}],"data":null}`)
	b := &Business{log: logrus.New()}

	if got := b.fetchTranslationsWithFallback("яблоками"); got != nil {
		t.Fatalf("GraphQL error must return nil so it is not cached, got %#v", got)
	}
}
