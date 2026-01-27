package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sandbox0-ai/infra/manager/pkg/apis/sandbox0/v1alpha1"
	"github.com/sandbox0-ai/infra/pkg/naming"
	"go.uber.org/zap"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecs.UniversalDeserializer()
)

type Server struct {
	port     int
	certPath string
	keyPath  string
	logger   *zap.Logger
	server   *http.Server
}

func NewServer(port int, certPath, keyPath string, logger *zap.Logger) *Server {
	return &Server{
		port:     port,
		certPath: certPath,
		keyPath:  keyPath,
		logger:   logger,
	}
}

func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/validate-sandboxtemplate", s.handleValidate)

	addr := fmt.Sprintf(":%d", s.port)
	s.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	s.logger.Info("Starting webhook server", zap.String("addr", addr))

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("Webhook server shutdown failed", zap.Error(err))
		}
	}()

	if err := s.server.ListenAndServeTLS(s.certPath, s.keyPath); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		if data, err := io.ReadAll(r.Body); err == nil {
			body = data
		}
	}
	if len(body) == 0 {
		s.logger.Error("Empty body")
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		s.logger.Error("Invalid Content-Type", zap.String("contentType", contentType))
		http.Error(w, "invalid Content-Type, expect application/json", http.StatusUnsupportedMediaType)
		return
	}

	var admissionResponse *admissionv1.AdmissionResponse
	ar := admissionv1.AdmissionReview{}
	if _, _, err := deserializer.Decode(body, nil, &ar); err != nil {
		s.logger.Error("Can't decode body", zap.Error(err))
		admissionResponse = &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	} else {
		if ar.Request != nil {
			admissionResponse = s.validate(&ar)
		} else {
			admissionResponse = &admissionv1.AdmissionResponse{
				Result: &metav1.Status{
					Message: "AdmissionReview Request is nil",
				},
			}
		}
	}

	admissionReview := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AdmissionReview",
			APIVersion: "admission.k8s.io/v1",
		},
	}
	if admissionResponse != nil {
		admissionReview.Response = admissionResponse
		if ar.Request != nil {
			admissionReview.Response.UID = ar.Request.UID
		}
	}

	resp, err := json.Marshal(admissionReview)
	if err != nil {
		s.logger.Error("Can't encode response", zap.Error(err))
		http.Error(w, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
		return
	}

	if _, err := w.Write(resp); err != nil {
		s.logger.Error("Can't write response", zap.Error(err))
	}
}

func (s *Server) validate(ar *admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	req := ar.Request
	var template v1alpha1.SandboxTemplate

	// If it's a delete operation, we might not have the object in the request (unless OldObject is used),
	// but validation usually runs on Create/Update.
	// CheckTemplate logic validates the content of the template.

	if req.Operation == admissionv1.Create || req.Operation == admissionv1.Update {
		if err := json.Unmarshal(req.Object.Raw, &template); err != nil {
			s.logger.Error("Could not unmarshal raw object", zap.Error(err))
			return &admissionv1.AdmissionResponse{
				Result: &metav1.Status{
					Message: err.Error(),
				},
			}
		}

		clusterID := naming.ClusterIDOrDefault(template.Spec.ClusterId)
		if err := naming.CheckTemplateName(clusterID, template.Name); err != nil {
			s.logger.Warn("Template validation failed",
				zap.String("namespace", req.Namespace),
				zap.String("name", req.Name),
				zap.Error(err),
			)
			return &admissionv1.AdmissionResponse{
				Allowed: false,
				Result: &metav1.Status{
					Message: fmt.Sprintf("Validation failed: %v", err),
					Reason:  metav1.StatusReasonInvalid,
					Code:    http.StatusForbidden,
				},
			}
		}
	}

	return &admissionv1.AdmissionResponse{
		Allowed: true,
	}
}
