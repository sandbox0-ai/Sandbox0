package manager

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sandbox0-ai/infra/manager/pkg/apis/sandbox0/v1alpha1"
	clientset "github.com/sandbox0-ai/infra/manager/pkg/generated/clientset/versioned"
	clientsetfake "github.com/sandbox0-ai/infra/manager/pkg/generated/clientset/versioned/fake"
	managerhttp "github.com/sandbox0-ai/infra/manager/pkg/http"
	"github.com/sandbox0-ai/infra/manager/pkg/service"
	"github.com/sandbox0-ai/infra/pkg/internalauth"
	"github.com/sandbox0-ai/infra/tests/integration/internal/utils"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

type managerTestEnv struct {
	server *httptest.Server
	token  string
}

func TestManagerIntegration_TemplateLifecycle(t *testing.T) {
	env := newManagerTestEnv(t)

	template := v1alpha1.SandboxTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name: "basic-template",
		},
		Spec: v1alpha1.SandboxTemplateSpec{
			MainContainer: v1alpha1.ContainerSpec{
				Image:     "sandbox0ai/infra:latest",
				Resources: v1alpha1.ResourceQuota{},
			},
			Pool: v1alpha1.PoolStrategy{
				MinIdle:   0,
				MaxIdle:   1,
				AutoScale: false,
			},
		},
	}

	resp, body := doRequest(t, env.server.Client(), http.MethodPost, env.server.URL+"/api/v1/templates", env.token, template)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected create status %d, got %d: %s", http.StatusCreated, resp.StatusCode, string(body))
	}

	err := utils.WaitUntil(context.Background(), 2*time.Second, 100*time.Millisecond, func(ctx context.Context) (bool, error) {
		resp, body := doRequest(t, env.server.Client(), http.MethodGet, env.server.URL+"/api/v1/templates", env.token, nil)
		if resp.StatusCode != http.StatusOK {
			return false, nil
		}
		var payload struct {
			Templates []v1alpha1.SandboxTemplate `json:"templates"`
			Count     int                        `json:"count"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return false, err
		}
		if payload.Count != 1 {
			return false, nil
		}
		if len(payload.Templates) != 1 || payload.Templates[0].Name != template.Name {
			return false, nil
		}
		return true, nil
	})
	utils.RequireNoError(t, err, "waiting for template to appear in list")
}

func TestManagerIntegration_TemplatesRequireAuth(t *testing.T) {
	env := newManagerTestEnv(t)

	resp, _ := doRequest(t, env.server.Client(), http.MethodGet, env.server.URL+"/api/v1/templates", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", resp.StatusCode)
	}
}

func newManagerTestEnv(t *testing.T) *managerTestEnv {
	t.Helper()

	k8sClient := k8sfake.NewSimpleClientset()
	crdClient := clientsetfake.NewSimpleClientset()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	err := os.WriteFile(configPath, []byte("template_namespace: sandbox0\n"), 0o600)
	utils.RequireNoError(t, err, "write manager config")
	t.Setenv("CONFIG_PATH", configPath)

	podIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	nodeIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	podLister := corelisters.NewPodLister(podIndexer)
	nodeLister := corelisters.NewNodeLister(nodeIndexer)

	templateLister := &testTemplateLister{
		client:    crdClient,
		namespace: "sandbox0",
	}
	logger := zap.NewNop()

	sandboxService := service.NewSandboxService(
		k8sClient,
		podLister,
		templateLister,
		nil,
		nil,
		nil,
		nil,
		service.SandboxServiceConfig{},
		logger,
	)

	templateService := service.NewTemplateService(crdClient, templateLister, logger)
	clusterService := service.NewClusterService(
		k8sClient,
		podLister,
		nodeLister,
		templateLister,
		logger,
	)

	privateKey, publicKey, err := createInternalKeys()
	utils.RequireNoError(t, err, "create internal keys")

	gen := internalauth.NewGenerator(internalauth.DefaultGeneratorConfig("internal-gateway", privateKey))
	token, err := gen.Generate("manager", "team-1", "user-1", internalauth.GenerateOptions{})
	utils.RequireNoError(t, err, "generate internal token")

	cfg := internalauth.DefaultValidatorConfig("manager", publicKey)
	cfg.AllowedCallers = []string{"internal-gateway"}
	validator := internalauth.NewValidator(cfg)

	server := managerhttp.NewServer(
		sandboxService,
		templateService,
		clusterService,
		validator,
		logger,
		0,
	)

	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	return &managerTestEnv{
		server: httpServer,
		token:  token,
	}
}

func createInternalKeys() (internalauth.PrivateKeyType, internalauth.PublicKeyType, error) {
	privatePEM, publicPEM, err := internalauth.GenerateEd25519KeyPair()
	if err != nil {
		return nil, nil, err
	}
	privateKey, err := internalauth.LoadEd25519PrivateKey(privatePEM)
	if err != nil {
		return nil, nil, err
	}
	publicKey, err := internalauth.LoadEd25519PublicKey(publicPEM)
	if err != nil {
		return nil, nil, err
	}
	return privateKey, publicKey, nil
}

type testTemplateLister struct {
	client    clientset.Interface
	namespace string
}

func (t *testTemplateLister) List() ([]*v1alpha1.SandboxTemplate, error) {
	list, err := t.client.Sandbox0V1alpha1().SandboxTemplates(t.namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	templates := make([]*v1alpha1.SandboxTemplate, 0, len(list.Items))
	for i := range list.Items {
		templates = append(templates, &list.Items[i])
	}
	return templates, nil
}

func (t *testTemplateLister) Get(namespace, name string) (*v1alpha1.SandboxTemplate, error) {
	template, err := t.client.Sandbox0V1alpha1().SandboxTemplates(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, errors.NewNotFound(v1alpha1.Resource("sandboxtemplate"), name)
		}
		return nil, err
	}
	return template, nil
}

func doRequest(t *testing.T, client *http.Client, method, url, token string, body any) (*http.Response, []byte) {
	t.Helper()

	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		utils.RequireNoError(t, err, "marshal request body")
		payload = bytes.NewReader(encoded)
	}

	req, err := http.NewRequest(method, url, payload)
	utils.RequireNoError(t, err, "create request")
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Internal-Token", token)
	}

	resp, err := client.Do(req)
	utils.RequireNoError(t, err, "send request")

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	utils.RequireNoError(t, err, "read response")

	return resp, respBody
}
