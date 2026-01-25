package predicates

// TemplateReady returns true when a template has the expected replicas.
func TemplateReady(readyReplicas, desiredReplicas int32) bool {
	if desiredReplicas == 0 {
		return false
	}
	return readyReplicas >= desiredReplicas
}
