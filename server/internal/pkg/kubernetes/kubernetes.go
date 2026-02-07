package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	badgerKV "b0k3ts/internal/pkg/badger"

	"github.com/dgraph-io/badger/v4"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// --- Constants / Types ---

const (
	kubeconfigKeyPrefix = "kubeconfig-"

	defaultFieldManager = "b0k3ts"

	// Query params to choose auth mode:
	// - mode=incluster
	// - mode=kubeconfig&config=<name>
	queryMode   = "mode"
	queryConfig = "config"

	modeInCluster  = "incluster"
	modeKubeconfig = "kubeconfig"
)

var (
	// Common OBC GVR for NooBaa / OBC:
	// apiVersion: objectbucket.io/v1alpha1
	// kind: ObjectBucketClaim
	obcGVR = schema.GroupVersionResource{
		Group:    "objectbucket.io",
		Version:  "v1alpha1",
		Resource: "objectbucketclaims",
	}

	// Very small allowlist to keep keys clean and safe.
	kubeconfigNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,62}$`)
)

type SecretCredsResponse struct {
	AWSAccessKeyID     string `json:"AWS_ACCESS_KEY_ID"`
	AWSSecretAccessKey string `json:"AWS_SECRET_ACCESS_KEY"`
}

type BucketListRequest struct {
	BucketName string `json:"bucket_name"`
	OBC        string `json:"obc"`
}

// --- Public: Route registration ---

// RegisterRoutes mounts Kubernetes-related APIs under the provided router group.
// Recommended mount point: /api/v1/kubernetes
func RegisterRoutes(rg *gin.RouterGroup, db *badger.DB) {
	h := &handler{db: db}

	// Kubeconfigs (named)
	rg.POST("/kubeconfigs/:name", h.UploadKubeconfig)
	rg.GET("/kubeconfigs", h.ListKubeconfigs)
	rg.DELETE("/kubeconfigs/:name", h.DeleteKubeconfig)

	// ObjectBucketClaim APIs
	rg.GET("/obc/:namespace", h.ListObjectBucketClaims)
	rg.POST("/obc/:namespace/apply", h.ApplyObjectBucketClaim)
	rg.DELETE("/obc/:namespace/:name", h.DeleteObjectBucketClaim)

	// Secret extraction for bucket credentials
	rg.GET("/obc/:namespace/:bucket/secret", h.GetObjectBucketClaimSecretCreds)
}

type handler struct {
	db *badger.DB
}

// --- Kubeconfig store (Badger) ---

func kubeconfigKey(name string) string {
	return kubeconfigKeyPrefix + name
}

func ValidateKubeconfigName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("kubeconfig name is required")
	}
	if !kubeconfigNameRe.MatchString(name) {
		return fmt.Errorf("invalid kubeconfig name %q (allowed: letters, digits, dot, underscore, dash; max 63 chars)", name)
	}
	return nil
}

func SaveKubeconfig(db *badger.DB, name string, kubeconfigBytes []byte) error {
	if db == nil {
		return errors.New("badger db is nil")
	}
	if err := ValidateKubeconfigName(name); err != nil {
		return err
	}
	if len(kubeconfigBytes) == 0 {
		return errors.New("kubeconfig content is empty")
	}

	// Validate kubeconfig parses (best effort sanity check)
	if _, err := clientcmd.Load(kubeconfigBytes); err != nil {
		return fmt.Errorf("invalid kubeconfig: %w", err)
	}

	if err := badgerKV.PutKV(db, kubeconfigKey(name), kubeconfigBytes); err != nil {
		return err
	}

	slog.Info("kubeconfig saved", "name", name, "bytes", len(kubeconfigBytes))
	return nil
}

func GetKubeconfig(db *badger.DB, name string) ([]byte, error) {
	if db == nil {
		return nil, errors.New("badger db is nil")
	}
	if err := ValidateKubeconfigName(name); err != nil {
		return nil, err
	}
	return badgerKV.PullKV(db, kubeconfigKey(name))
}

func DeleteKubeconfig(db *badger.DB, name string) error {
	if db == nil {
		return errors.New("badger db is nil")
	}
	if err := ValidateKubeconfigName(name); err != nil {
		return err
	}
	return badgerKV.DeleteKV(db, kubeconfigKey(name))
}

func ListKubeconfigNames(db *badger.DB) ([]string, error) {
	if db == nil {
		return nil, errors.New("badger db is nil")
	}

	var names []string
	err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		opts.Prefix = []byte(kubeconfigKeyPrefix)

		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte(kubeconfigKeyPrefix)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := string(it.Item().Key())
			name := strings.TrimPrefix(key, kubeconfigKeyPrefix)
			if name != "" {
				names = append(names, name)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(names)
	return names, nil
}

// --- REST configs & clients (client-go) ---

func BuildRestConfigInCluster() (*rest.Config, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	applyDefaults(cfg)
	return cfg, nil
}

func BuildRestConfigFromBadger(ctx context.Context, db *badger.DB, name string) (*rest.Config, error) {
	_ = ctx // reserved for future (e.g., external decrypt, audit)
	kc, err := GetKubeconfig(db, name)
	if err != nil {
		return nil, err
	}
	cfg, err := clientcmd.RESTConfigFromKubeConfig(kc)
	if err != nil {
		return nil, err
	}
	applyDefaults(cfg)
	return cfg, nil
}

func applyDefaults(cfg *rest.Config) {
	// Reasonable, conservative defaults.
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.QPS == 0 {
		cfg.QPS = 10
	}
	if cfg.Burst == 0 {
		cfg.Burst = 20
	}
	cfg.UserAgent = "b0k3ts/1.0 (dynamic-client)"
}

func NewDynamicClient(cfg *rest.Config) (dynamic.Interface, error) {
	if cfg == nil {
		return nil, errors.New("rest config is nil")
	}
	return dynamic.NewForConfig(cfg)
}

func NewCoreClient(cfg *rest.Config) (*kubernetes.Clientset, error) {
	if cfg == nil {
		return nil, errors.New("rest config is nil")
	}
	return kubernetes.NewForConfig(cfg)
}

// --- Helpers: choose auth mode per request ---

func (h *handler) buildClientsFromRequest(c *gin.Context) (dynamic.Interface, *kubernetes.Clientset, error) {
	mode := strings.TrimSpace(strings.ToLower(c.Query(queryMode)))
	if mode == "" {
		// Default to in-cluster if unspecified (sane for server deployments).
		mode = modeInCluster
	}

	var (
		cfg *rest.Config
		err error
	)

	switch mode {
	case modeInCluster:
		cfg, err = BuildRestConfigInCluster()
	case modeKubeconfig:
		name := strings.TrimSpace(c.Query(queryConfig))
		if name == "" {
			return nil, nil, fmt.Errorf("missing query parameter %q for mode=%q", queryConfig, modeKubeconfig)
		}
		cfg, err = BuildRestConfigFromBadger(c.Request.Context(), h.db, name)
	default:
		return nil, nil, fmt.Errorf("unsupported mode %q (use %q or %q)", mode, modeInCluster, modeKubeconfig)
	}
	if err != nil {
		return nil, nil, err
	}

	dyn, err := NewDynamicClient(cfg)
	if err != nil {
		return nil, nil, err
	}

	core, err := NewCoreClient(cfg)
	if err != nil {
		return nil, nil, err
	}

	return dyn, core, nil
}

// --- OBC operations (dynamic client) ---

func ListObjectBucketClaims(ctx context.Context, dyn dynamic.Interface, namespace string) (*unstructured.UnstructuredList, error) {
	if dyn == nil {
		return nil, errors.New("dynamic client is nil")
	}
	if strings.TrimSpace(namespace) == "" {
		return nil, errors.New("namespace is required")
	}

	return dyn.Resource(obcGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
}

func ApplyObjectBucketClaim(ctx context.Context, dyn dynamic.Interface, namespace string, manifestYAMLorJSON []byte, fieldManager string) (*unstructured.Unstructured, error) {
	if dyn == nil {
		return nil, errors.New("dynamic client is nil")
	}
	if strings.TrimSpace(namespace) == "" {
		return nil, errors.New("namespace is required")
	}
	if len(manifestYAMLorJSON) == 0 {
		return nil, errors.New("manifest body is empty")
	}
	if strings.TrimSpace(fieldManager) == "" {
		fieldManager = defaultFieldManager
	}

	// Convert YAML to JSON if needed, then decode into Unstructured.
	jsonBytes, err := yaml.ToJSON(manifestYAMLorJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to convert manifest to json: %w", err)
	}

	var obj unstructured.Unstructured
	if err := obj.UnmarshalJSON(jsonBytes); err != nil {
		return nil, fmt.Errorf("failed to parse manifest as unstructured json: %w", err)
	}

	// Enforce OBC kind/apiversion if omitted; leave as-is if provided.
	if obj.GetAPIVersion() == "" {
		obj.SetAPIVersion("objectbucket.io/v1alpha1")
	}
	if obj.GetKind() == "" {
		obj.SetKind("ObjectBucketClaim")
	}

	// Apply to target namespace (namespaced resource).
	if obj.GetNamespace() == "" {
		obj.SetNamespace(namespace)
	} else if obj.GetNamespace() != namespace {
		return nil, fmt.Errorf("manifest namespace %q does not match request namespace %q", obj.GetNamespace(), namespace)
	}

	name := obj.GetName()
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("manifest metadata.name is required for apply")
	}

	// Server-Side Apply: send the object as an "apply" patch.
	applyJSON, err := obj.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal unstructured object: %w", err)
	}

	force := true
	return dyn.Resource(obcGVR).Namespace(namespace).Patch(
		ctx,
		name,
		types.ApplyPatchType,
		applyJSON,
		metav1.PatchOptions{
			FieldManager: fieldManager,
			Force:        &force,
		},
	)
}

func DeleteObjectBucketClaim(ctx context.Context, dyn dynamic.Interface, namespace, name string) error {
	if dyn == nil {
		return errors.New("dynamic client is nil")
	}
	if strings.TrimSpace(namespace) == "" {
		return errors.New("namespace is required")
	}
	if strings.TrimSpace(name) == "" {
		return errors.New("name is required")
	}

	propagation := metav1.DeletePropagationBackground
	return dyn.Resource(obcGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
}

// --- Secret extraction (typed client) ---

func ExtractAWSCredsFromSecret(sec *corev1.Secret) (SecretCredsResponse, error) {
	if sec == nil {
		return SecretCredsResponse{}, errors.New("secret is nil")
	}

	ak := strings.TrimSpace(string(sec.Data["AWS_ACCESS_KEY_ID"]))
	sk := strings.TrimSpace(string(sec.Data["AWS_SECRET_ACCESS_KEY"]))

	if ak == "" || sk == "" {
		return SecretCredsResponse{}, errors.New("secret is missing AWS_ACCESS_KEY_ID or AWS_SECRET_ACCESS_KEY")
	}

	return SecretCredsResponse{
		AWSAccessKeyID:     ak,
		AWSSecretAccessKey: sk,
	}, nil
}

func GetBucketSecretCreds(ctx context.Context, core *kubernetes.Clientset, namespace, bucketName string) (SecretCredsResponse, error) {
	if core == nil {
		return SecretCredsResponse{}, errors.New("core client is nil")
	}
	if strings.TrimSpace(namespace) == "" {
		return SecretCredsResponse{}, errors.New("namespace is required")
	}
	if strings.TrimSpace(bucketName) == "" {
		return SecretCredsResponse{}, errors.New("bucket name is required")
	}

	sec, err := core.CoreV1().Secrets(namespace).Get(ctx, bucketName, metav1.GetOptions{})
	if err != nil {
		return SecretCredsResponse{}, err
	}

	return ExtractAWSCredsFromSecret(sec)
}

// --- Gin handlers ---

// UploadKubeconfig stores a kubeconfig in Badger under a *named* key.
// Supports either:
// - multipart/form-data with field "file"
// - raw body (application/yaml, text/yaml, application/octet-stream, etc.)
func (h *handler) UploadKubeconfig(c *gin.Context) {
	name := strings.TrimSpace(c.Param("name"))
	if err := ValidateKubeconfigName(name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Small guardrail: kubeconfigs are typically tiny. Keep limit sane.
	// If you need bigger, raise it.
	const maxBytes = 1 << 20 // 1 MiB

	var data []byte
	if fh, err := c.FormFile("file"); err == nil && fh != nil {
		if fh.Size > maxBytes {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "kubeconfig file too large"})
			return
		}
		f, err := fh.Open()
		if err != nil {
			slog.Error("failed to open uploaded kubeconfig", "err", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to open uploaded file"})
			return
		}
		defer f.Close()

		data, err = io.ReadAll(io.LimitReader(f, maxBytes+1))
		if err != nil {
			slog.Error("failed to read uploaded kubeconfig", "err", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read uploaded file"})
			return
		}
	} else {
		// raw body
		var err error
		data, err = io.ReadAll(io.LimitReader(c.Request.Body, maxBytes+1))
		if err != nil {
			slog.Error("failed to read kubeconfig body", "err", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
			return
		}
	}

	if len(data) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kubeconfig is empty (send multipart 'file' or raw body)"})
		return
	}
	if len(data) > maxBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "kubeconfig too large"})
		return
	}

	if err := SaveKubeconfig(h.db, name, data); err != nil {
		slog.Error("failed to save kubeconfig", "name", name, "err", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "kubeconfig saved", "name": name})
}

func (h *handler) ListKubeconfigs(c *gin.Context) {
	names, err := ListKubeconfigNames(h.db)
	if err != nil {
		slog.Error("failed to list kubeconfigs", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list kubeconfigs"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": names})
}

func (h *handler) DeleteKubeconfig(c *gin.Context) {
	name := strings.TrimSpace(c.Param("name"))
	if err := ValidateKubeconfigName(name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := DeleteKubeconfig(h.db, name); err != nil {
		slog.Error("failed to delete kubeconfig", "name", name, "err", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "kubeconfig deleted", "name": name})
}

func (h *handler) ListObjectBucketClaims(c *gin.Context) {
	namespace := strings.TrimSpace(c.Param("namespace"))
	dyn, _, err := h.buildClientsFromRequest(c)
	if err != nil {
		slog.Error("failed to build kubernetes clients", "err", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	list, err := ListObjectBucketClaims(ctx, dyn, namespace)
	if err != nil {
		slog.Error("failed to list OBC", "namespace", namespace, "err", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var bucketNames []BucketListRequest

	for _, item := range list.Items {

		innerObject, ok := item.Object["spec"].(map[string]interface{})
		if ok {

			if bucketName, ok := innerObject["bucketName"].(string); ok {
				bucketNames = append(bucketNames, BucketListRequest{
					OBC:        item.GetName(),
					BucketName: bucketName,
				})

			} else {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid OBC format"})
				return
			}

		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid OBC format"})
			return
		}

	}

	// Return as-is (unstructured list is already JSON-friendly).
	c.JSON(http.StatusOK, bucketNames)
}

func (h *handler) ApplyObjectBucketClaim(c *gin.Context) {
	namespace := strings.TrimSpace(c.Param("namespace"))

	dyn, _, err := h.buildClientsFromRequest(c)
	if err != nil {
		slog.Error("failed to build kubernetes clients", "err", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 2<<20)) // 2 MiB
	if err != nil {
		slog.Error("failed to read apply body", "err", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()

	obj, err := ApplyObjectBucketClaim(ctx, dyn, namespace, body, defaultFieldManager)
	if err != nil {
		slog.Error("failed to apply OBC", "namespace", namespace, "err", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, obj.Object)
}

func (h *handler) DeleteObjectBucketClaim(c *gin.Context) {
	namespace := strings.TrimSpace(c.Param("namespace"))
	name := strings.TrimSpace(c.Param("name"))

	dyn, _, err := h.buildClientsFromRequest(c)
	if err != nil {
		slog.Error("failed to build kubernetes clients", "err", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	if err := DeleteObjectBucketClaim(ctx, dyn, namespace, name); err != nil {
		slog.Error("failed to delete OBC", "namespace", namespace, "name", name, "err", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted", "namespace": namespace, "name": name})
}

func (h *handler) GetObjectBucketClaimSecretCreds(c *gin.Context) {
	namespace := strings.TrimSpace(c.Param("namespace"))
	bucket := strings.TrimSpace(c.Param("bucket"))

	_, core, err := h.buildClientsFromRequest(c)
	if err != nil {
		slog.Error("failed to build kubernetes clients", "err", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()

	creds, err := GetBucketSecretCreds(ctx, core, namespace, bucket)
	if err != nil {
		// Avoid leaking internals; return error string for now (you can map k8s StatusReason if you want).
		slog.Error("failed to get bucket secret creds", "namespace", namespace, "bucket", bucket, "err", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Return only the two fields requested.
	c.JSON(http.StatusOK, creds)
}

// --- Optional helper: pretty-print unstructured (not used, but handy) ---

func ToPrettyJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}
