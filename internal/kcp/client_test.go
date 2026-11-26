package kcp

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func TestNewClientTLSVerificationDefaultsAndOptIn(t *testing.T) {
	path := t.TempDir() + "/config"
	config := `apiVersion: v1
kind: Config
clusters:
- name: kcp
  cluster:
    server: https://kcp.example/clusters/root
    certificate-authority-data: Y2VydGlmaWNhdGU=
users:
- name: admin
  user: {}
contexts:
- name: admin@kcp
  context:
    cluster: kcp
    user: admin
current-context: admin@kcp
`
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}

	secureClient, err := NewClient(path, false)
	if err != nil {
		t.Fatalf("create default client: %v", err)
	}
	if secureClient.config.TLSClientConfig.Insecure {
		t.Fatal("TLS verification is disabled by default")
	}
	if len(secureClient.config.TLSClientConfig.CAData) == 0 {
		t.Fatal("configured CA data was not retained when TLS verification is enabled")
	}

	insecureClient, err := NewClient(path, true)
	if err != nil {
		t.Fatalf("create client with TLS bypass: %v", err)
	}
	if !insecureClient.config.TLSClientConfig.Insecure {
		t.Fatal("TLS verification was not disabled by opt-in")
	}
	if len(insecureClient.config.TLSClientConfig.CAData) != 0 || insecureClient.config.TLSClientConfig.CAFile != "" {
		t.Fatal("configured CA roots were not cleared when TLS verification is skipped")
	}
	if _, err := dynamic.NewForConfig(insecureClient.configForWorkspace("root")); err != nil {
		t.Fatalf("create workspace client with TLS bypass: %v", err)
	}
}

func TestWorkspacePathFromHost(t *testing.T) {
	tests := []struct {
		host string
		want string
	}{
		{host: "https://kcp.example/clusters/root", want: "root"},
		{host: "https://kcp.example/prefix/clusters/root:org:team/", want: "root:org:team"},
	}
	for _, test := range tests {
		t.Run(test.host, func(t *testing.T) {
			got, err := workspacePathFromHost(test.host)
			if err != nil {
				t.Fatalf("workspacePathFromHost() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("workspacePathFromHost() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestWorkspacePathFromHostRequiresKCPClusterPath(t *testing.T) {
	if _, err := workspacePathFromHost("https://kubernetes.example"); err == nil {
		t.Fatal("workspacePathFromHost() expected an error without a /clusters/ path")
	}
}

func TestConfigForWorkspaceReplacesClusterPath(t *testing.T) {
	client := &Client{
		config: &rest.Config{Host: "https://kcp.example/proxy/clusters/root"},
	}
	config := client.configForWorkspace("root:org:team")
	server, err := url.Parse(config.Host)
	if err != nil {
		t.Fatalf("parse generated host: %v", err)
	}
	if got, want := server.Path, "/proxy/clusters/root:org:team"; got != want {
		t.Fatalf("generated path = %q, want %q", got, want)
	}
	if client.config.Host != "https://kcp.example/proxy/clusters/root" {
		t.Fatalf("original config was mutated: %q", client.config.Host)
	}
}

func TestCreateWorkspaceKubeconfigPreservesAuthAndSecuresTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.crt")
	if err := os.WriteFile(caPath, []byte("test CA"), 0600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config")
	config := `apiVersion: v1
kind: Config
clusters:
- name: kcp
  cluster:
    server: https://kcp.example/prefix/clusters/root
    certificate-authority: ca.crt
users:
- name: admin
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1
      command: /usr/local/bin/kcp-auth
      args: ["--profile", "admin"]
      interactiveMode: Never
contexts:
- name: admin@kcp
  context:
    cluster: kcp
    user: admin
current-context: admin@kcp
`
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}

	client, err := NewClient(configPath, false)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	path, cleanup, err := client.CreateWorkspaceKubeconfig("root:org:team")
	if err != nil {
		t.Fatalf("create workspace kubeconfig: %v", err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			t.Errorf("cleanup workspace kubeconfig: %v", err)
		}
	}()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat generated kubeconfig: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("temporary kubeconfig permissions = %04o, want 0600", got)
	}

	generated, err := clientcmd.LoadFromFile(path)
	if err != nil {
		t.Fatalf("load generated kubeconfig: %v", err)
	}
	if generated.CurrentContext != "admin@kcp" {
		t.Fatalf("current context = %q, want admin@kcp", generated.CurrentContext)
	}
	context := generated.Contexts[generated.CurrentContext]
	if context == nil || context.Cluster != "kcp" || context.AuthInfo != "admin" {
		t.Fatalf("generated context = %#v, want the original context and auth info", context)
	}
	cluster := generated.Clusters["kcp"]
	if cluster == nil || cluster.Server != "https://kcp.example/prefix/clusters/root:org:team" {
		t.Fatalf("generated cluster = %#v, want selected workspace endpoint", cluster)
	}
	if cluster.InsecureSkipTLSVerify || cluster.CertificateAuthority != caPath {
		t.Fatalf("generated TLS configuration = %#v, want TLS verification and resolved CA path", cluster)
	}
	auth := generated.AuthInfos["admin"]
	if auth == nil || auth.Exec == nil || auth.Exec.Command != "/usr/local/bin/kcp-auth" ||
		strings.Join(auth.Exec.Args, " ") != "--profile admin" {
		t.Fatalf("generated auth info = %#v, want original exec authentication", auth)
	}
	if got, err := os.ReadFile(configPath); err != nil || !strings.Contains(string(got), "/clusters/root\n") {
		t.Fatalf("original kubeconfig changed: err=%v content=%s", err, got)
	}

	if err := cleanup(); err != nil {
		t.Fatalf("remove generated kubeconfig: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("temporary kubeconfig still exists after cleanup: %v", err)
	}
}

func TestCreateWorkspaceKubeconfigCarriesTLSBypass(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	config := `apiVersion: v1
kind: Config
clusters:
- name: kcp
  cluster:
    server: https://kcp.example/clusters/root
    certificate-authority-data: Y2VydA==
users:
- name: admin
  user: {}
contexts:
- name: admin@kcp
  context:
    cluster: kcp
    user: admin
current-context: admin@kcp
`
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(path, true)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	generatedPath, cleanup, err := client.CreateWorkspaceKubeconfig("root")
	if err != nil {
		t.Fatalf("create workspace kubeconfig: %v", err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			t.Errorf("cleanup workspace kubeconfig: %v", err)
		}
	}()

	generated, err := clientcmd.LoadFromFile(generatedPath)
	if err != nil {
		t.Fatalf("load generated kubeconfig: %v", err)
	}
	cluster := generated.Clusters["kcp"]
	if cluster == nil || !cluster.InsecureSkipTLSVerify {
		t.Fatalf("generated cluster should retain explicit TLS bypass: %#v", cluster)
	}
	if cluster.CertificateAuthority != "" || len(cluster.CertificateAuthorityData) != 0 {
		t.Fatalf("generated TLS bypass retained CA roots: %#v", cluster)
	}
}
