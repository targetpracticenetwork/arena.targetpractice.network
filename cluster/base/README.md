# base/

This directory contains core Kubernetes manifests for the ArenaService cluster, including:
- Cluster-wide RBAC
- CustomResourceDefinitions (CRDs)
- Baseline namespaces required for control, VPN, and arena areas

Apply these manifests first when bootstrapping a new cluster. 