// Package sandbox applies OS-level constraints to agent processes:
// filesystem scope (chroot/namespaces) and network egress policy.
// Three modes are supported: none, filesystem-only, and filesystem+network.
package sandbox
