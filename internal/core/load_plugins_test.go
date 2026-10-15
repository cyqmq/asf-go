package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"testing"
)

func TestLoadExternalPlugin(t *testing.T) {
	repoRoot := findRepoRoot(t)

	soDir := t.TempDir()
	soPath := filepath.Join(soDir, "hello.so")
	// Go plugin 要求与宿主运行时一致：测试二进制以 -race 构建时，插件也必须用 -race 构建。
	args := []string{"build"}
	if raceEnabled(t) {
		args = append(args, "-race")
	}
	args = append(args, "-buildmode=plugin", "-o", soPath, "./examples/plugins/hello")
	cmd := exec.Command("go", args...)
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

// raceEnabled 返回当前测试二进制是否以 -race 构建。
func raceEnabled(t *testing.T) bool {
	t.Helper()
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return false
	}
	for _, s := range bi.Settings {
		if s.Key == "-race" && s.Value == "true" {
			return true
		}
	}
	return false
}
