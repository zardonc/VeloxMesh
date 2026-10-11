//go:build phase29preflight

package app

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestLiveEmbeddingPrefixIsolation(t *testing.T) {
	env := liveEnvironment(t)
	repo, dsn, keyID := liveRepository(t)
	database := liveDatabase{dsn: dsn, keyID: keyID}
	var scopes []string
	for index, prefix := range []string{"", "search_query: ", ""} {
		t.Setenv("PHASE29_INPUT_PREFIX", prefix)
		application, _ := liveApplicationWithDatabase(t, env, database)
		recorder := installLiveTiming(t, application, time.Now())
		server := httptest.NewServer(application.Router)
		chain := liveChain{app: application, repo: repo, url: server.URL, token: keyID,
			model: env["SANS_PRIMARY_DEFAULT_MODEL"], timing: recorder}
		scopes = append(scopes, liveEmbeddingLifecycle(t, chain, index == 2))
		server.Close()
		application.Close()
		recorder.dump(t)
	}
	if scopes[0] == scopes[1] || scopes[0] != scopes[2] || liveUsageCount(t, liveChain{repo: repo, token: keyID}) != 2 {
		t.Fatal("input prefix did not isolate/restore vectors and billing")
	}
	shipLogJSON(t, map[string]any{"type": "input_policy_isolation", "old_scope_preserved": true,
		"different_prefix_isolated": true, "restored_old_hit": true})
}
