package cases

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sandbox0-ai/infra/pkg/framework"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func registerApiNetworkPolicySuite(env *framework.ScenarioEnv) {
	Describe("API network policy mode", Ordered, func() {
		var (
			session   *apiSession
			cleanup   func()
			sandboxID string
		)

		BeforeAll(func() {
			Expect(env).NotTo(BeNil())

			var err error
			session, cleanup, err = newAPISession(env, false)
			Expect(err).NotTo(HaveOccurred())

			password, err := framework.GetSecretValue(env.TestCtx.Context, env.Config.Kubeconfig, env.Infra.Namespace, "admin-password", "password")
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() error {
				return session.login(env.TestCtx.Context, "admin@localhost", password)
			}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

			Eventually(func() error {
				templates, err := session.listTemplates(env.TestCtx.Context)
				if err != nil {
					return err
				}
				if len(templates) == 0 {
					return fmt.Errorf("no templates found")
				}
				return nil
			}).WithTimeout(3 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

			resp, err := session.claimSandbox(env.TestCtx.Context, "default")
			Expect(err).NotTo(HaveOccurred())
			sandboxID = resp.SandboxID
		})

		AfterAll(func() {
			if session != nil {
				_ = session.deleteSandbox(env.TestCtx.Context, sandboxID)
			}
			if cleanup != nil {
				cleanup()
			}
		})

		Context("template lifecycle", func() {
			It("creates, updates, and deletes templates", func() {
				templates, err := session.listTemplates(env.TestCtx.Context)
				Expect(err).NotTo(HaveOccurred())
				Expect(templates).NotTo(BeEmpty())

				base := templates[0]
				name := fmt.Sprintf("e2e-network-%d", time.Now().UnixNano())
				newTemplate := cloneTemplateForCreate(base, name)

				created, err := session.createTemplate(env.TestCtx.Context, newTemplate)
				Expect(err).NotTo(HaveOccurred())
				Expect(created).NotTo(BeNil())
				Expect(created.Name).To(Equal(name))

				updated := *created
				updated.Spec.Description = "e2e update"
				updated.Spec.Pool.MaxIdle = updated.Spec.Pool.MaxIdle + 1
				if updated.Spec.Pool.MaxIdle < updated.Spec.Pool.MinIdle {
					updated.Spec.Pool.MaxIdle = updated.Spec.Pool.MinIdle + 1
				}

				updatedResp, err := session.updateTemplate(env.TestCtx.Context, name, updated)
				Expect(err).NotTo(HaveOccurred())
				Expect(updatedResp).NotTo(BeNil())
				Expect(updatedResp.Spec.Description).To(Equal("e2e update"))

				err = session.deleteTemplate(env.TestCtx.Context, name)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("sandbox lifecycle", func() {
			It("fetches status and refreshes sandboxes", func() {
				Expect(sandboxID).NotTo(BeEmpty())

				status, _, err := session.doAPIRequest(env.TestCtx.Context, http.MethodGet, "/sandboxes/"+sandboxID, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(status).To(Equal(http.StatusOK))

				status, _, err = session.doAPIRequest(env.TestCtx.Context, http.MethodGet, "/sandboxes/"+sandboxID+"/status", nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(status).To(Equal(http.StatusOK))

				status, _, err = session.doAPIRequest(env.TestCtx.Context, http.MethodPost, "/sandboxes/"+sandboxID+"/refresh", nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(status).To(Equal(http.StatusOK))
			})
		})

		Context("filesystem and process capabilities", func() {
			It("performs file operations and process management", func() {
				Expect(sandboxID).NotTo(BeEmpty())
				dirPath := fmt.Sprintf("tmp/e2e-network-%d", time.Now().UnixNano())
				filePath := dirPath + "/hello.txt"
				content := []byte("hello network")

				status, _, err := session.doRawAPIRequest(env.TestCtx.Context, http.MethodPost, "/sandboxes/"+sandboxID+"/files/"+dirPath+"?mkdir=true&recursive=true", nil, "")
				Expect(err).NotTo(HaveOccurred())
				Expect(status).To(Equal(http.StatusCreated))

				status, _, err = session.doRawAPIRequest(env.TestCtx.Context, http.MethodPost, "/sandboxes/"+sandboxID+"/files/"+filePath, content, "text/plain")
				Expect(err).NotTo(HaveOccurred())
				Expect(status).To(Equal(http.StatusOK))

				status, body, err := session.doRawAPIRequest(env.TestCtx.Context, http.MethodGet, "/sandboxes/"+sandboxID+"/files/"+filePath, nil, "")
				Expect(err).NotTo(HaveOccurred())
				Expect(status).To(Equal(http.StatusOK))
				Expect(string(body)).To(Equal(string(content)))

				status, body, err = session.doRawAPIRequest(env.TestCtx.Context, http.MethodGet, "/sandboxes/"+sandboxID+"/files/"+dirPath+"?list=true", nil, "")
				Expect(err).NotTo(HaveOccurred())
				Expect(status).To(Equal(http.StatusOK))

				var listResp struct {
					Entries []map[string]any `json:"entries"`
				}
				Expect(json.Unmarshal(body, &listResp)).To(Succeed())
				Expect(listResp.Entries).NotTo(BeEmpty())

				ctxReq := map[string]any{
					"type":    "cmd",
					"command": []string{"/bin/sh", "-c", "sleep 30"},
				}
				status, body, err = session.doAPIRequest(env.TestCtx.Context, http.MethodPost, "/sandboxes/"+sandboxID+"/contexts", ctxReq)
				Expect(err).NotTo(HaveOccurred())
				Expect(status).To(Equal(http.StatusCreated))

				var ctxResp struct {
					ID string `json:"id"`
				}
				Expect(json.Unmarshal(body, &ctxResp)).To(Succeed())
				Expect(ctxResp.ID).NotTo(BeEmpty())

				status, _, err = session.doAPIRequest(env.TestCtx.Context, http.MethodGet, "/sandboxes/"+sandboxID+"/contexts", nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(status).To(Equal(http.StatusOK))

				status, _, err = session.doAPIRequest(env.TestCtx.Context, http.MethodDelete, "/sandboxes/"+sandboxID+"/contexts/"+ctxResp.ID, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(status).To(Equal(http.StatusOK))
			})
		})

		Context("network policies", func() {
			It("retrieves network and bandwidth policies", func() {
				Expect(sandboxID).NotTo(BeEmpty())

				status, _, err := session.doAPIRequest(env.TestCtx.Context, http.MethodGet, "/sandboxes/"+sandboxID+"/network", nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(status).To(Equal(http.StatusOK))

				status, _, err = session.doAPIRequest(env.TestCtx.Context, http.MethodGet, "/sandboxes/"+sandboxID+"/bandwidth", nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(status).To(Equal(http.StatusOK))
			})
		})

		Context("missing services", func() {
			It("returns errors for storage APIs", func() {
				status, body, err := session.doAPIRequest(env.TestCtx.Context, http.MethodGet, "/sandboxvolumes", nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(status).To(Equal(http.StatusServiceUnavailable), string(body))
			})
		})
	})
}
