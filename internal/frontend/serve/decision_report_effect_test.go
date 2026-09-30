package serve

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/assembly/boot"
	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/contract/provider"
)

func TestEffectDecisionHTTPUsesBootStoreWithoutInference(t *testing.T) {
	dir := testenv.TempDir(t)
	t.Setenv("REASONIX_HOME", dir)
	const kind = "decision-http-boot-probe"
	probe := &traceScript{}
	provider.Register(kind, func(provider.Config) (provider.Provider, error) { return probe, nil })
	configuration := `default_model = "test-model"
[agent]
system_prompt = "BASE"
[[providers]]
name = "test-model"
kind = "` + kind + `"
model = "x"
`
	if err := os.WriteFile(filepath.Join(dir, "reasonix.toml"), []byte(configuration), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := (config.Roots{}).ApproveWorkspacePrograms(dir); err != nil {
		t.Fatal(err)
	}
	sessions := filepath.Join(dir, "sessions")
	digest := sha256.Sum256([]byte(filepath.Clean(dir)))
	path := filepath.Join(sessions, "decisions", hex.EncodeToString(digest[:]), "snapshots.sqlite")
	// The public comparison tool is the producer; no private store state is assigned.
	id := decisionFixtureSnapshot(t, path)
	t.Chdir(dir)
	bc := NewBroadcaster()
	ctrl, err := boot.Build(context.Background(), boot.Options{Home: dir, WorkspaceRoot: dir, SessionDir: sessions, Sink: bc})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctrl.Close)
	srv := httptest.NewServer(operatorHandler(New(ctrl, bc, config.ServeConfig{})))
	t.Cleanup(srv.Close)
	raw, _ := json.Marshal(map[string]any{"snapshot_id": id, "candidate_json": "WRONG ANSWER"})
	status, out := postDecision(t, srv, string(raw))
	if status != 200 || out["status"] != "computed_fallback" || out["snapshot_id"] != id {
		t.Fatalf("boot report unavailable: %d %v", status, out)
	}
	probe.mu.Lock()
	defer probe.mu.Unlock()
	if probe.round != 0 {
		t.Fatal("report made a provider inference call")
	}
}
