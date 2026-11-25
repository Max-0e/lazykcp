package kcp

import (
	"net/url"
	"testing"

	"k8s.io/client-go/rest"
)

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
