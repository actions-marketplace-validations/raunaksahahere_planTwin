# ADR-0006: Go toolchain runs in disposable containers

**Context.** Go is not installed on the development host. Installing a toolchain from a download runs
downloaded code outside a reviewed container.

**Decision.** All Go commands (build, test, vet, lint) run in the official `golang:1.24` image, using `docker run --rm`
with the repository mounted. Images are pinned by tag in the Makefile and the CI workflow.

**Consequences.** Slower first run (image pull). Integration tests reach Docker through the host socket
when run this way; the socket mount is documented as a development-only shortcut.
