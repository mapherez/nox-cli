package maintenance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/mapherez/nox-cli/internal/install"
	"github.com/mapherez/nox-cli/internal/releases"
)

type diskSource struct{ candidate string }

func (s diskSource) Latest(context.Context) (releases.Release, error) {
	return releases.Release{Version: "v1.1.0"}, nil
}
func (s diskSource) Download(_ context.Context, _ string, stage string) (string, error) {
	data, err := os.ReadFile(s.candidate)
	if err != nil {
		return "", err
	}
	name := "nox"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(stage, name)
	return path, os.WriteFile(path, data, 0700)
}
func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		_, _ = os.Stdout.WriteString("nox v1.1.0 (commit test, built test)\n")
		os.Exit(0)
	}
	if len(os.Args) == 3 && os.Args[1] == "--complete-update" {
		source, _ := os.Executable()
		root := filepath.Dir(filepath.Dir(source))
		if install.FinishUpdate(context.Background(), root, source, os.Args[2], "v1.1.0") != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	if len(os.Args) == 4 && os.Args[1] == "--test-managed-update" {
		manager := &Manager{Root: os.Args[2], Version: "v1.0.0", Source: diskSource{candidate: os.Args[3]}}
		if _, err := manager.Update(context.Background(), "v1.1.0"); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func TestManagedExecutableUpdateAndFailedPreparation(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "replacement", true: "bad-candidate"}[failure], func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "user with spaces", "nox-cli")
			if err := os.MkdirAll(root, 0700); err != nil {
				t.Fatal(err)
			}
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			payload, err := os.ReadFile(executable)
			if err != nil {
				t.Fatal(err)
			}
			name := "nox"
			if runtime.GOOS == "windows" {
				name += ".exe"
			}
			target := filepath.Join(root, name)
			if err := os.WriteFile(target, payload, 0700); err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(payload)
			record := map[string]any{"schema": 1, "product": "nox-cli", "binary": name, "sha256": hex.EncodeToString(sum[:]), "version": "v1.0.0", "pathHooks": []any{}}
			raw, _ := json.Marshal(record)
			_ = os.WriteFile(filepath.Join(root, "install-state.json"), raw, 0600)
			_ = os.WriteFile(filepath.Join(root, "config.json"), []byte("preserve settings"), 0600)
			candidate := filepath.Join(t.TempDir(), name)
			next := append(append([]byte{}, payload...), []byte("new-release")...)
			if failure {
				next = []byte("not-an-executable")
			}
			_ = os.WriteFile(candidate, next, 0700)
			child := exec.Command(target, "--test-managed-update", root, candidate)
			output, err := child.CombinedOutput()
			if failure {
				if err == nil {
					t.Fatal("bad payload activated")
				}
				got, _ := os.ReadFile(target)
				if string(got) != string(payload) {
					t.Fatal("failed preparation changed executable")
				}
			} else {
				if err != nil {
					t.Fatalf("update: %v %s", err, output)
				}
				deadline := time.Now().Add(30 * time.Second)
				for {
					current, _ := os.ReadFile(filepath.Join(root, "install-state.json"))
					var state struct {
						Version string `json:"version"`
					}
					_ = json.Unmarshal(current, &state)
					_, stageErr := os.Stat(filepath.Join(root, "install"))
					if state.Version == "v1.1.0" && os.IsNotExist(stageErr) {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("update/helper did not finish cleanup")
					}
					time.Sleep(100 * time.Millisecond)
				}
				got, _ := os.ReadFile(target)
				if string(got) != string(next) {
					t.Fatal("wrong replacement payload")
				}
			}
			if _, err := os.Stat(filepath.Join(root, "install")); !os.IsNotExist(err) {
				t.Fatal("update left staging")
			}
			settings, _ := os.ReadFile(filepath.Join(root, "config.json"))
			if string(settings) != "preserve settings" {
				t.Fatal("configuration changed")
			}
		})
	}
}
