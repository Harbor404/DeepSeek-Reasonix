package serve

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/session/control"
)

func TestServeSwitchEffortNoopsSameLevelBeforeBuild(t *testing.T) {
	writeServeEffortSelectionConfig(t, "high")
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{
		Sink:       bc,
		ModelRef:   "alternate/shared-chat",
		Effort:     "high",
		SessionDir: testenv.TempDir(t),
	})
	defer ctrl.Close()
	server := New(ctrl, bc, config.ServeConfig{})
	built := 0
	server.buildController = func(_ context.Context, ref string) (*control.Controller, error) {
		built++
		return control.New(control.Options{
			Sink:       bc,
			ModelRef:   ref,
			Effort:     "low",
			SessionDir: testenv.TempDir(t),
		}), nil
	}

	if err := server.switchEffort(context.Background(), "high"); err != nil {
		t.Fatalf("same-level switchEffort: %v", err)
	}
	if built != 0 {
		t.Fatalf("same-level switch rebuilt controller %d times", built)
	}
	if err := server.switchEffort(context.Background(), "ultra"); err == nil {
		t.Fatal("unsupported effort was accepted")
	}
	if built != 0 {
		t.Fatalf("invalid effort reached builder %d times", built)
	}
}

func TestServeModelAndEffortNoopWhileRunning(t *testing.T) {
	writeServeEffortSelectionConfig(t, "high")
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{
		Runner:     blockingRunner{},
		Sink:       bc,
		ModelRef:   "alternate/shared-chat",
		Effort:     "high",
		SessionDir: testenv.TempDir(t),
	})
	defer ctrl.Close()
	server := New(ctrl, bc, config.ServeConfig{})
	built := 0
	server.buildController = func(context.Context, string) (*control.Controller, error) {
		built++
		return control.New(control.Options{Sink: bc}), nil
	}
	srv := httptest.NewServer(operatorHandler(server))
	defer srv.Close()

	ctrl.SubmitHTTP("keep running")
	waitRunning(t, ctrl)
	for _, tc := range []struct {
		path string
		body string
	}{
		{path: "/model", body: `{"ref":"alternate/shared-chat"}`},
		{path: "/effort", body: `{"effort":"high"}`},
	} {
		resp, err := http.Post(srv.URL+tc.path, "application/json", strings.NewReader(tc.body))
		if err != nil {
			t.Fatalf("POST %s: %v", tc.path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("POST %s while running = %d, want 204", tc.path, resp.StatusCode)
		}
	}
	if built != 0 {
		t.Fatalf("same-selection switch while running rebuilt controller %d times", built)
	}
	ctrl.Cancel()
	waitNotRunning(t, ctrl)
}

func TestServeSubmitNoopsSameModelAndEffortBeforeBuild(t *testing.T) {
	writeServeEffortSelectionConfig(t, "high")
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{
		Sink:       bc,
		ModelRef:   "alternate/shared-chat",
		Effort:     "high",
		SessionDir: testenv.TempDir(t),
	})
	defer ctrl.Close()
	server := New(ctrl, bc, config.ServeConfig{})
	built := 0
	server.buildController = func(context.Context, string) (*control.Controller, error) {
		built++
		return control.New(control.Options{Sink: bc}), nil
	}
	srv := httptest.NewServer(operatorHandler(server))
	defer srv.Close()

	for _, input := range []string{"/model alternate/shared-chat", "/effort high"} {
		body := fmt.Sprintf(`{"input":%q}`, input)
		resp, err := http.Post(srv.URL+"/submit", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatalf("POST /submit %q: %v", input, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("POST /submit %q = %d, want 204", input, resp.StatusCode)
		}
	}
	if built != 0 {
		t.Fatalf("same-selection CLI switches rebuilt controller %d times", built)
	}
}

func writeServeEffortSelectionConfig(t *testing.T, effort string) {
	t.Helper()
	home := testenv.TempDir(t)
	t.Setenv("REASONIX_HOME", home)
	cfgPath := config.UserConfigPath()
	if cfgPath == "" {
		t.Fatal("user config path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`default_model = "alternate/shared-chat"

[[providers]]
name = "alternate"
kind = "openai"
base_url = "http://127.0.0.1:1/v1"
models = ["shared-chat"]
default = "shared-chat"
supported_efforts = ["low", "high"]
effort = %q
`, effort)
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
