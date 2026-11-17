# DevOps review

Judge whether this change can be built, deployed, operated and rolled back safely. The desk runs this profile when
infrastructure or delivery files change (container files, CI, Kubernetes or Helm, Terraform or OpenTofu, compose files,
configuration management).

## What to check

- **Reproducibility**: images, base images, actions and dependencies pinned (a digest or an exact version, not
  `latest`); builds that don't depend on the machine.
- **Secrets**: never in images, manifests, CI files or logs; injected at run time from the platform's secret store;
  least privilege for every token and service account the change adds.
- **Workloads**: resource requests and limits, health and readiness checks, graceful shutdown, replicas and
  disruption budgets where the service needs them, non-root users, read-only file systems where possible.
- **Configuration**: every new setting has a safe default and is documented; environment-specific values stay out of
  the image.
- **Data and migrations**: backward compatible with the running version (expand, then contract), reversible or with a
  stated recovery path, safe to run twice.
- **Networking**: only the ports and egress the service needs; TLS where traffic leaves the cluster or host.
- **Observability**: logs without secrets or personal data, metrics or alerts for new failure modes.
- **CI/CD**: no step that runs untrusted code with secrets; protected variables stay out of jobs anyone can trigger.
- **Infrastructure as code**: state handled safely, no destructive change without a plan step, provider and module
  versions pinned.

## Severity

Rate by deployment impact × likelihood: `critical` (outage, data loss or exposed secret on deploy), `high` (likely
incident or a security gap in production), `medium` (fragile: breaks under specific conditions), `low` (hygiene),
`info` (an observation). Cite `file:line` for every finding and give the concrete fix.
