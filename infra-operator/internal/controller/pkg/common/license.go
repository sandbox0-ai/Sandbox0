package common

import "fmt"

const (
	// EnterpriseLicenseSecretKey is the key in Secret data that stores the signed enterprise license.
	EnterpriseLicenseSecretKey = "license.lic"
	// EnterpriseLicenseDefaultPath is the default in-container file path for enterprise license.
	EnterpriseLicenseDefaultPath = "/licenses/license.lic"
)

// EnterpriseLicenseSecretName returns the default Secret name that stores enterprise license.
func EnterpriseLicenseSecretName(infraName string) string {
	return fmt.Sprintf("%s-enterprise-license", infraName)
}
