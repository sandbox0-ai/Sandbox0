package service

import (
	"sort"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
)

// SandboxIndex keeps an in-memory index of sandbox IDs by namespace.
// All methods are safe for concurrent use.
type SandboxIndex struct {
	mu          sync.RWMutex
	byNamespace map[string]map[string]struct{}
	bySandboxID map[string]string
}

// NewSandboxIndex creates a new SandboxIndex instance.
func NewSandboxIndex() *SandboxIndex {
	return &SandboxIndex{
		byNamespace: make(map[string]map[string]struct{}),
		bySandboxID: make(map[string]string),
	}
}

// ResourceEventHandler returns handlers to keep the index in sync.
func (s *SandboxIndex) ResourceEventHandler() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    s.handleAdd,
		UpdateFunc: s.handleUpdate,
		DeleteFunc: s.handleDelete,
	}
}

// GetNamespace returns the namespace of a sandbox ID if present.
func (s *SandboxIndex) GetNamespace(sandboxID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	namespace, ok := s.bySandboxID[sandboxID]
	return namespace, ok
}

// ListSandboxIDs returns sandbox IDs in the given namespace.
func (s *SandboxIndex) ListSandboxIDs(namespace string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	set := s.byNamespace[namespace]
	if len(set) == 0 {
		return nil
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (s *SandboxIndex) handleAdd(obj any) {
	if pod := extractPod(obj); pod != nil {
		s.upsertPod(pod)
	}
}

func (s *SandboxIndex) handleUpdate(oldObj, newObj any) {
	oldPod := extractPod(oldObj)
	newPod := extractPod(newObj)
	s.refreshPodIndex(oldPod, newPod)
}

func (s *SandboxIndex) handleDelete(obj any) {
	if pod := extractPod(obj); pod != nil {
		s.deletePod(pod)
	}
}

func (s *SandboxIndex) refreshPodIndex(oldPod, newPod *corev1.Pod) {
	if oldPod != nil {
		oldID := sandboxIDFromPod(oldPod)
		if oldID != "" {
			newID := ""
			newNamespace := ""
			if newPod != nil {
				newID = sandboxIDFromPod(newPod)
				newNamespace = newPod.Namespace
			}
			if newID != oldID || oldPod.Namespace != newNamespace {
				s.removeBySandboxID(oldID)
			}
		}
	}
	if newPod != nil {
		s.upsertPod(newPod)
	}
}

func (s *SandboxIndex) upsertPod(pod *corev1.Pod) {
	sandboxID := sandboxIDFromPod(pod)
	if sandboxID == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if oldNamespace, ok := s.bySandboxID[sandboxID]; ok && oldNamespace != pod.Namespace {
		s.removeBySandboxIDLocked(sandboxID, oldNamespace)
	}

	set, ok := s.byNamespace[pod.Namespace]
	if !ok {
		set = make(map[string]struct{})
		s.byNamespace[pod.Namespace] = set
	}
	set[sandboxID] = struct{}{}
	s.bySandboxID[sandboxID] = pod.Namespace
}

func (s *SandboxIndex) deletePod(pod *corev1.Pod) {
	sandboxID := sandboxIDFromPod(pod)
	if sandboxID == "" {
		return
	}
	s.removeBySandboxID(sandboxID)
}

func (s *SandboxIndex) removeBySandboxID(sandboxID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	namespace, ok := s.bySandboxID[sandboxID]
	if !ok {
		return
	}
	s.removeBySandboxIDLocked(sandboxID, namespace)
}

func (s *SandboxIndex) removeBySandboxIDLocked(sandboxID, namespace string) {
	delete(s.bySandboxID, sandboxID)
	if set, ok := s.byNamespace[namespace]; ok {
		delete(set, sandboxID)
		if len(set) == 0 {
			delete(s.byNamespace, namespace)
		}
	}
}

func sandboxIDFromPod(pod *corev1.Pod) string {
	if pod == nil {
		return ""
	}
	return pod.Name
}

func extractPod(obj any) *corev1.Pod {
	switch pod := obj.(type) {
	case *corev1.Pod:
		return pod
	case cache.DeletedFinalStateUnknown:
		if pod, ok := pod.Obj.(*corev1.Pod); ok {
			return pod
		}
	case *cache.DeletedFinalStateUnknown:
		if pod, ok := pod.Obj.(*corev1.Pod); ok {
			return pod
		}
	}
	return nil
}
