package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLoadExternalPlugin(t *testing.T) {
	repoRoot := findRepoRoot(t)

	soDir := t.TempDir()
	soPath := filepath.Join(soDir, "hello.so")
	cmd := exec.Command("go", "build", "-buildmode=plugin", "-o", soPath, "./examples/plugins/hello")
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("编译示例插件失败: %v\n%s", err, out)
	}

	dir := t.TempDir()
	cfg := `{"PluginsDir": "` + soDir + `"}`
	if err := os.WriteFile(filepath.Join(dir, "ASF.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	asf, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := asf.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = asf.Stop() }()

	found := false
	for _, p := range asf.Plugins.Plugins() {
		if p.Name() == "HelloPlugin" {
			found = true
		}
	}
	if !found {
		t.Fatal("未加载 HelloPlugin")
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("找不到仓库根目录")
		}
		dir = parent
	}
}
