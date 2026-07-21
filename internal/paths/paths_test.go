package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withEnv 临时设置一组环境变量并恢复原值。
func withEnv(t *testing.T, kv map[string]string, fn func()) {
	t.Helper()
	old := make(map[string]string, len(kv))
	for k, v := range kv {
		old[k] = os.Getenv(k)
		os.Setenv(k, v)
	}
	defer func() {
		for k, v := range old {
			os.Setenv(k, v)
		}
	}()
	fn()
}

func TestConfigDir_XDGExplicit(t *testing.T) {
	withEnv(t, map[string]string{"XDG_CONFIG_HOME": "/tmp/xcfg"}, func() {
		got := ConfigDir()
		want := filepath.Join("/tmp/xcfg", "lindiag")
		if got != want {
			t.Errorf("ConfigDir()=%q want %q", got, want)
		}
	})
}

func TestConfigDir_HomeFallback(t *testing.T) {
	withEnv(t, map[string]string{
		"XDG_CONFIG_HOME": "",
		"HOME":            "/tmp/fakehome",
	}, func() {
		got := ConfigDir()
		want := filepath.Join("/tmp/fakehome", ".config", "lindiag")
		if got != want {
			t.Errorf("ConfigDir()=%q want %q", got, want)
		}
	})
}

func TestDataDir_XDGExplicit(t *testing.T) {
	withEnv(t, map[string]string{"XDG_DATA_HOME": "/tmp/xdta"}, func() {
		got := DataDir()
		want := filepath.Join("/tmp/xdta", "lindiag")
		if got != want {
			t.Errorf("DataDir()=%q want %q", got, want)
		}
	})
}

func TestDataDir_HomeFallback(t *testing.T) {
	withEnv(t, map[string]string{
		"XDG_DATA_HOME": "",
		"HOME":          "/tmp/fakehome",
	}, func() {
		got := DataDir()
		want := filepath.Join("/tmp/fakehome", ".local", "share", "lindiag")
		if got != want {
			t.Errorf("DataDir()=%q want %q", got, want)
		}
	})
}

func TestConfigFile_Extension(t *testing.T) {
	got := ConfigFile()
	if !strings.HasSuffix(got, "config.json") {
		t.Errorf("ConfigFile()=%q want suffix config.json", got)
	}
	if !strings.Contains(got, "lindiag") {
		t.Errorf("ConfigFile()=%q should contain lindiag", got)
	}
}

func TestWhitelistFile(t *testing.T) {
	got := WhitelistFile()
	if !strings.HasSuffix(got, "whitelist.txt") {
		t.Errorf("WhitelistFile()=%q want suffix whitelist.txt", got)
	}
}

func TestHistoryFile_Format(t *testing.T) {
	got := HistoryFile("20240101_120000")
	if !strings.HasSuffix(got, "history_20240101_120000.json") {
		t.Errorf("HistoryFile()=%q want suffix history_20240101_120000.json", got)
	}
	if !strings.Contains(got, "lindiag") {
		t.Errorf("HistoryFile()=%q should contain lindiag", got)
	}
}

func TestReportFile_Format(t *testing.T) {
	got := ReportFile("20240101_120000", "md")
	if !strings.HasSuffix(got, "report_20240101_120000.md") {
		t.Errorf("ReportFile()=%q want suffix report_20240101_120000.md", got)
	}
}

func TestEnsureConfigDir_CreatesPath(t *testing.T) {
	tmp := t.TempDir()
	withEnv(t, map[string]string{"XDG_CONFIG_HOME": tmp}, func() {
		if err := EnsureConfigDir(); err != nil {
			t.Fatalf("EnsureConfigDir: %v", err)
		}
		info, err := os.Stat(filepath.Join(tmp, "lindiag"))
		if err != nil {
			t.Fatalf("expected lindiag dir created: %v", err)
		}
		if !info.IsDir() {
			t.Fatalf("expected dir, got file")
		}
	})
}

func TestEnsureDataDir_Idempotent(t *testing.T) {
	tmp := t.TempDir()
	withEnv(t, map[string]string{"XDG_DATA_HOME": tmp}, func() {
		if err := EnsureDataDir(); err != nil {
			t.Fatalf("first EnsureDataDir: %v", err)
		}
		if err := EnsureDataDir(); err != nil {
			t.Fatalf("second EnsureDataDir: %v", err)
		}
	})
}
