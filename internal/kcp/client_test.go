package kcp

import (
	"net/url"
	"os"
	"testing"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
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
