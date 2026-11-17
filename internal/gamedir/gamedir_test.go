package gamedir

import (
	"os"
	"path/filepath"
	"testing"
)

func mkFake(t *testing.T) (root, forever string) {
	t.Helper()
	root = filepath.Join(t.TempDir(), "World of Warcraft")
	forever = filepath.Join(root, ForeverDir)
	for _, acct := range []string{"ACCOUNT1", "ACCOUNT2", "ACCOUNT3"} {
		if err := os.MkdirAll(filepath.Join(forever, "WTF", "Account", acct, "SavedVariables"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.MkdirAll(filepath.Join(root, "_retail_", "WTF"), 0o755)
	os.WriteFile(filepath.Join(forever, "WTF", "Account", "ACCOUNT1", "SavedVariables", "GreenFangToolkit.lua"), []byte("x=1"), 0o644)
	os.WriteFile(filepath.Join(forever, "WTF", "Account", "ACCOUNT3", "SavedVariables", "GreenFangToolkit.lua"), []byte("x=1"), 0o644)
	os.WriteFile(filepath.Join(forever, "WTF", "Account", "ACCOUNT2", "SavedVariables", "Other.lua"), []byte("x=1"), 0o644)
	return
}

func TestNormalize(t *testing.T) {
	root, forever := mkFake(t)
	for _, in := range []string{
		root,
		forever,
		filepath.Join(forever, "WTF"),
		filepath.Join(forever, "WTF", "Account"),
		filepath.Join(root, "_retail_"), // registry often points here
	} {
		if got := Normalize(in); got != forever {
			t.Errorf("Normalize(%q) = %q want %q", in, got, forever)
		}
	}
	if got := Normalize(t.TempDir()); got != "" {
		t.Errorf("empty dir: got %q", got)
	}
	if Normalize("") != "" {
		t.Error("empty string")
	}
}

func TestSavedVariablesFiles(t *testing.T) {
	_, forever := mkFake(t)
	files := SavedVariablesFiles(forever)
	if len(files) != 2 {
		t.Fatalf("%v", files)
	}
	if AccountOf(files[0]) != "ACCOUNT1" || AccountOf(files[1]) != "ACCOUNT3" {
		t.Fatalf("%v", files)
	}
}

func TestCommonRoots(t *testing.T) {
	r := CommonRoots([]string{"D"})
	if r[0] != `D:\World of Warcraft` {
		t.Fatal(r[0])
	}
}
