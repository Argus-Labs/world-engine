package cluster

import (
	"os"
	"path/filepath"
	"testing"
)

const kubeconfig = `apiVersion: v1
kind: Config
clusters:
- name: a
  cluster: {server: https://a.example:6443}
- name: b
  cluster: {server: https://b.example:6443}
contexts:
- name: ctx-a
  context: {cluster: a, user: u}
- name: ctx-b
  context: {cluster: b, user: u}
current-context: ctx-a
users:
- name: u
  user: {token: t}
`

func TestLoadKubeconfigUsesExplicitFileAndContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(path, []byte(kubeconfig), 0o600); err != nil {
		t.Fatal(err)
	}
	rc, name, err := loadKubeconfig(Config{Kubeconfig: path, Context: "ctx-b"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "ctx-b" || rc.Host != "https://b.example:6443" {
		t.Fatalf("name=%s host=%s", name, rc.Host)
	}
	_, name, err = loadKubeconfig(Config{Kubeconfig: path})
	if err != nil || name != "ctx-a" {
		t.Fatalf("default context: %s %v", name, err)
	}
}

func TestLoadKubeconfigNoCurrentContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(path, []byte("apiVersion: v1\nkind: Config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadKubeconfig(Config{Kubeconfig: path}); err == nil {
		t.Fatal("expected an error without a current context")
	}
}
