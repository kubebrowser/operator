package browser_api

import (
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{
	Group:    "subresource.kubebrowser.io",
	Version:  "v1alpha1",
	Resource: "browsers",
}

const (
	devEnvironment = "development"
)

func namespacedResourceUrl(gvr schema.GroupVersionResource, subpath string) string {
	return fmt.Sprintf("/apis/%s/%s/namespaces/{namespace}/%s/{name}/%s", gvr.Group, gvr.Version, gvr.Resource, subpath)
}

func groupVersionBasePath(gvr schema.GroupVersionResource) string {
	return fmt.Sprintf("/apis/%s/%s", gvr.Group, gvr.Version)
}

func browserServerUrl(name string, namespace string) string {
	env := os.Getenv("ENVIRONMENT")
	if env == devEnvironment {
		return "http://localhost:3000/action"
	}

	return fmt.Sprintf("http://%s.%s.svc.cluster.local:3000/action", name, namespace)
}

func browserVncUrl(name string, namespace string) string {
	env := os.Getenv("ENVIRONMENT")
	if env == devEnvironment {
		return "localhost:15900"
	}

	return fmt.Sprintf("%s.%s.svc.cluster.local:5900", name, namespace)
}
