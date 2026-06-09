package kubernetes

import (
	"context"
	"path/filepath"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

const (
	testKey  = "test_config.json"
	testNS   = "default"
	testName = "test"
)

var (
	objectMeta = metav1.ObjectMeta{
		Name:      testName,
		Namespace: testNS,
		Labels: map[string]string{
			"app": "test",
		},
	}
)

func TestSource(t *testing.T) {
	home := homedir.HomeDir()
	s := New(
		WithNamespace(testNS),
		WithLabelSelector(""),
		WithKubeConfig(filepath.Join(home, ".kube", "config")),
	)
	data, err := s.Load(context.Background(), testNS+"/"+testName+"/"+testKey)
	if err != nil {
		t.Error(err)
	}
	t.Log(string(data))
}

func TestConfig(t *testing.T) {
	restConfig, err := rest.InClusterConfig()
	home := homedir.HomeDir()

	options := []Option{
		WithNamespace(testNS),
		WithLabelSelector("app=test"),
	}

	if err != nil {
		kubeconfig := filepath.Join(home, ".kube", "config")
		restConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			t.Fatal(err)
		}
		options = append(options, WithKubeConfig(kubeconfig))
	}
	clientSet, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		t.Fatal(err)
	}

	clientSetConfigMaps := clientSet.CoreV1().ConfigMaps(testNS)

	source := New(options...)
	if _, err = clientSetConfigMaps.Create(context.Background(), &v1.ConfigMap{
		ObjectMeta: objectMeta,
		Data: map[string]string{
			testKey: "test config",
		},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err = clientSetConfigMaps.Delete(context.Background(), testName, metav1.DeleteOptions{}); err != nil {
			t.Error(err)
		}
	}()

	key := testNS + "/" + testName + "/" + testKey
	data, err := source.Load(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "test config" {
		t.Fatalf("config error: got %q, want %q", string(data), "test config")
	}
}
