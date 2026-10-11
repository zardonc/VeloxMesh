//go:build phase29preflight

package app

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/pipeline"
)

func liveFeatureChain(t *testing.T, feature string) liveChain {
	t.Helper()
	if os.Getenv("PHASE29_MODEL") == "" {
		t.Skip("real feature chain opt-in")
	}
	env := liveEnvironment(t)
	repo, dsn, token := liveRepository(t)
	model, provider := env["SANS_PRIMARY_DEFAULT_MODEL"], "sans-primary"
	ctx := context.Background()
	if feature == "fusion" {
		judge := model
		model, provider = "live-fusion", "fusion-ensemble"
		if _, err := repo.Combos().Create(ctx, &controlstate.ComboMutation{ID: model, Name: model, Enabled: true, Strategy: "fusion", Members: []string{judge}, Judge: &judge}); err != nil {
			t.Fatal(err)
		}
	}
	application, _ := liveApplicationWithDatabase(t, env, liveDatabase{dsn: dsn, keyID: token})
	if feature == "buffered" {
		liveEnableResponseRules(t, application, repo)
	}
	if feature != "fusion" {
		if err := repo.Rates().Save(ctx, &controlstate.ProviderModelRate{ProviderID: provider, Model: model, InputCreditRate: liveTestCreditRate, OutputCreditRate: liveTestCreditRate}); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(application.Router)
	t.Cleanup(server.Close)
	var balance int
	if err := repo.DBForTest().QueryRow("SELECT credit_balance FROM api_keys WHERE id = ?", token).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	return liveChain{app: application, repo: repo, token: token, model: model, url: server.URL, initialBalance: balance}
}

func liveEnableResponseRules(t *testing.T, application *App, repo controlstate.Repository) {
	t.Helper()
	ctx := context.Background()
	cfg := &pipeline.SemanticPipelineConfig{Rules: map[pipeline.RuleName]pipeline.RuleConfig{pipeline.RuleFilter: {Enabled: true}}}
	if err := repo.SemanticRules().SaveGlobalDefaults(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	if err := application.ReloadSemanticRules(ctx, repo); err != nil {
		t.Fatal(err)
	}
	global, err := application.RuntimeProviderManager.GetGlobalDefaults(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !pipeline.New(pipeline.DefaultRegistry(), pipeline.ResolveSemanticRuleConfig(global, nil)).HasResponseRulesEnabled() {
		t.Fatal("response rule was not activated")
	}
	t.Log("response rules persisted, published and verified enabled")
}

func liveFeatureStream(t *testing.T, feature string) {
	t.Helper()
	chain := liveFeatureChain(t, feature)
	response := liveHTTP(t, chain, llm.ChatCompletionRequest{Model: chain.model, Stream: true, Messages: []llm.Message{{Role: llm.RoleUser, Content: "Reply with a short greeting."}}})
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	var done, fragments int
	for scanner.Scan() {
		line := scanner.Text()
		liveRejectSSEError(t, line)
		if line == "data: [DONE]" {
			done++
		}
		fragments += liveContentFragments(t, line)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if done != 1 || fragments == 0 {
		t.Fatalf("feature=%s done=%d fragments=%d", feature, done, fragments)
	}
	t.Logf("feature=%s done=%d fragments=%d provider=%s strategy=%s", feature, done, fragments, response.Header.Get("X-Provider"), response.Header.Get("X-Routing-Strategy"))
	if feature == "fusion" {
		liveAssertFusionUsage(t, chain)
		return
	}
	liveAssertSettlement(t, chain, 1)
}

func liveContentFragments(t *testing.T, line string) int {
	t.Helper()
	if !strings.HasPrefix(line, "data: ") || line == "data: [DONE]" {
		return 0
	}
	var chunk llm.ChatCompletionChunkResponse
	if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk); err != nil {
		t.Fatal(err)
	}
	var fragments int
	for _, choice := range chunk.Choices {
		if choice.Delta.Content != "" {
			fragments++
		}
	}
	return fragments
}

func liveAssertFusionUsage(t *testing.T, chain liveChain) {
	t.Helper()
	var count, prompt, completion, total, balance int
	var status string
	if err := chain.repo.DBForTest().QueryRow("SELECT COUNT(*), COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(response_tokens),0), COALESCE(SUM(total_tokens),0), COALESCE(MAX(status),'') FROM usage_records WHERE api_key_id = ?", chain.token).Scan(&count, &prompt, &completion, &total, &status); err != nil {
		t.Fatal(err)
	}
	if err := chain.repo.DBForTest().QueryRow("SELECT credit_balance FROM api_keys WHERE id = ?", chain.token).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if count != 1 || total <= 0 || total != prompt+completion || status != "missing_rate" || balance != chain.initialBalance {
		t.Fatalf("fusion usage rows=%d total_tokens=%d status=%s", count, total, status)
	}
	t.Logf("fusion usage exactly once total_tokens=%d status=missing_rate no_debit=true", total)
}

func TestLiveBufferedStream(t *testing.T) { liveFeatureStream(t, "buffered") }
func TestLiveFusionStream(t *testing.T)   { liveFeatureStream(t, "fusion") }

func TestLiveAuthentication(t *testing.T) {
	chain := newLiveChain(t)
	request, err := http.NewRequest(http.MethodPost, chain.url+"/v1/chat/completions", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d", response.StatusCode)
	}
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		t.Fatal(err)
	}
	if liveUsageCount(t, chain) != 0 || chain.app.HealthStore().Snapshot("sans-primary").TotalSuccesses != 0 {
		t.Fatal("unauthorized request reached settlement or provider")
	}
}
