package kcp

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const workspaceGVR = "tenancy.kcp.io/v1alpha1/workspaces"

type Resource struct {
	Group      string
	Version    string
	Name       string
	Kind       string
	Namespaced bool
}

func (r Resource) GVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: r.Group, Version: r.Version, Resource: r.Name}
}

type Client struct {
	config                *rest.Config
	rootPath              string
	kubeconfigPath        string
	insecureSkipTLSVerify bool
}

func NewClient(kubeconfigPath string, insecureSkipTLSVerify bool) (*Client, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	config.TLSClientConfig.Insecure = insecureSkipTLSVerify
	if insecureSkipTLSVerify {
		config.TLSClientConfig.CAFile = ""
		config.TLSClientConfig.CAData = nil
	}
	rootPath, err := workspacePathFromHost(config.Host)
	if err != nil {
		return nil, err
	}
	return &Client{
		config:                config,
		rootPath:              rootPath,
		kubeconfigPath:        kubeconfigPath,
		insecureSkipTLSVerify: insecureSkipTLSVerify,
	}, nil
}

func (c *Client) RootPath() string {
	return c.rootPath
}

func (c *Client) CreateWorkspaceKubeconfig(workspacePath string) (string, func() error, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if c.kubeconfigPath != "" {
		rules.ExplicitPath = c.kubeconfigPath
	}
	config, err := rules.Load()
	if err != nil {
		return "", nil, fmt.Errorf("reload kubeconfig for terminal: %w", err)
	}
	config = config.DeepCopy()
	if err := clientcmdapi.MinifyConfig(config); err != nil {
		return "", nil, fmt.Errorf("minify kubeconfig for terminal: %w", err)
	}

	context := config.Contexts[config.CurrentContext]
	if context == nil {
		return "", nil, fmt.Errorf("kubeconfig context %q is missing", config.CurrentContext)
	}
	cluster := config.Clusters[context.Cluster]
	if cluster == nil {
		return "", nil, fmt.Errorf("kubeconfig cluster %q is missing", context.Cluster)
	}
	cluster.Server = c.configForWorkspace(workspacePath).Host
	cluster.InsecureSkipTLSVerify = c.insecureSkipTLSVerify
	if c.insecureSkipTLSVerify {
		cluster.CertificateAuthority = ""
		cluster.CertificateAuthorityData = nil
	}

	file, err := os.CreateTemp("", "lazykcp-kubeconfig-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temporary kubeconfig for terminal: %w", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", nil, fmt.Errorf("close temporary kubeconfig: %w", err)
	}
	if err := clientcmd.WriteToFile(*config, path); err != nil {
		_ = os.Remove(path)
		return "", nil, fmt.Errorf("write temporary kubeconfig: %w", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		_ = os.Remove(path)
		return "", nil, fmt.Errorf("secure temporary kubeconfig: %w", err)
	}
	return path, func() error {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove temporary kubeconfig: %w", err)
		}
		return nil
	}, nil
}

func (c *Client) ListWorkspaces(ctx context.Context, parentPath string) ([]*unstructured.Unstructured, error) {
	client, err := dynamic.NewForConfig(c.configForWorkspace(parentPath))
	if err != nil {
		return nil, fmt.Errorf("create workspace client: %w", err)
	}
	list, err := client.Resource(parseGVR(workspaceGVR)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list child workspaces in %s: %w", parentPath, err)
	}
	sort.Slice(list.Items, func(i, j int) bool {
		return list.Items[i].GetName() < list.Items[j].GetName()
	})
	result := make([]*unstructured.Unstructured, len(list.Items))
	for i := range list.Items {
		result[i] = &list.Items[i]
	}
	return result, nil
}

func (c *Client) DiscoverResources(ctx context.Context, workspacePath string) ([]Resource, error) {
	client, err := discovery.NewDiscoveryClientForConfig(c.configForWorkspace(workspacePath))
	if err != nil {
		return nil, fmt.Errorf("create discovery client: %w", err)
	}
	lists, err := client.ServerPreferredResources()
	if err != nil {
		return nil, fmt.Errorf("discover resources in %s: %w", workspacePath, err)
	}

	var resources []Resource
	for _, list := range lists {
		gv, err := schema.ParseGroupVersion(list.GroupVersion)
		if err != nil {
			return nil, fmt.Errorf("parse discovered API group version %q: %w", list.GroupVersion, err)
		}
		for _, apiResource := range list.APIResources {
			if strings.Contains(apiResource.Name, "/") || apiResource.Verbs == nil || !contains(apiResource.Verbs, "list") {
				continue
			}
			resources = append(resources, Resource{
				Group:      gv.Group,
				Version:    gv.Version,
				Name:       apiResource.Name,
				Kind:       apiResource.Kind,
				Namespaced: apiResource.Namespaced,
			})
		}
	}
	sort.Slice(resources, func(i, j int) bool {
		left := resources[i].Group + "/" + resources[i].Name
		right := resources[j].Group + "/" + resources[j].Name
		return left < right
	})
	return resources, nil
}

func (c *Client) ListInstances(ctx context.Context, workspacePath string, resource Resource) ([]unstructured.Unstructured, error) {
	client, err := dynamic.NewForConfig(c.configForWorkspace(workspacePath))
	if err != nil {
		return nil, fmt.Errorf("create resource client: %w", err)
	}
	resourceClient := client.Resource(resource.GVR())
	var list *unstructured.UnstructuredList
	if resource.Namespaced {
		list, err = resourceClient.Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	} else {
		list, err = resourceClient.List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("list %s in %s: %w", resource.Name, workspacePath, err)
	}
	sort.Slice(list.Items, func(i, j int) bool {
		left := list.Items[i].GetNamespace() + "/" + list.Items[i].GetName()
		right := list.Items[j].GetNamespace() + "/" + list.Items[j].GetName()
		return left < right
	})
	return list.Items, nil
}

func (c *Client) configForWorkspace(workspacePath string) *rest.Config {
	config := rest.CopyConfig(c.config)
	server, _ := url.Parse(config.Host)
	base := server.Path
	if idx := strings.Index(base, "/clusters/"); idx >= 0 {
		base = base[:idx]
	}
	server.Path = strings.TrimRight(base, "/") + "/clusters/" + workspacePath
	config.Host = server.String()
	return config
}

func workspacePathFromHost(host string) (string, error) {
	server, err := url.Parse(host)
	if err != nil {
		return "", fmt.Errorf("parse kubeconfig server URL: %w", err)
	}
	idx := strings.Index(server.Path, "/clusters/")
	if idx < 0 {
		return "", fmt.Errorf("kubeconfig server URL %q must include a kcp /clusters/<workspace-path> endpoint", host)
	}
	path := strings.Trim(strings.TrimPrefix(server.Path[idx:], "/clusters/"), "/")
	if path == "" {
		return "", fmt.Errorf("kubeconfig server URL %q has an empty kcp workspace path", host)
	}
	return path, nil
}

func parseGVR(value string) schema.GroupVersionResource {
	parts := strings.Split(value, "/")
	if len(parts) != 3 {
		return schema.GroupVersionResource{}
	}
	return schema.GroupVersionResource{Group: parts[0], Version: parts[1], Resource: parts[2]}
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
