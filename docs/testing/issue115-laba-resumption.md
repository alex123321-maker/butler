# Issue115: isolated laba resumption, 2026-10-28

The owner's instruction "Хорошо приступай и работай на ssh laba" supersedes the
PC-only location restriction for subsequent issue115 work. Earlier closed PC
studies, identities, permissions, costs and verdicts remain unchanged. This
resumption first runs zero-native controls; it does not deploy working Butler.

At entry the server's /srv/storage had about101MiB free and its root filesystem
about708MiB. No shared cache, archive, model or application data was deleted.
The writable /srv/gitea filesystem had136GiB available. Only a new private
directory /srv/gitea/butler-acceptance/issue115-20261028-v1 is used there; Gitea
configuration/data and existing Butler containers are not modified.

The complete raw source snapshot is producer
a58aa96936c01ee3f9de6d6601e02f8df9058653: 6282 files /335187485 bytes. The
transferred tar SHA256 is
d5cce46f897feb6d74bb30e8ad95be6aa5ff75c1be67ca2a9027f660ca367497.
Every extracted file passed its retained SHA256 inventory; source is mounted
read-only. Local packaging/transfer is not server execution evidence by itself.

Build/test image is the already present Go1.27.2 bookworm image
sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61.
Its own cache/temp/proof directories reside on the free filesystem; the existing
module cache is read-only. Build containers have no network, read-only root,
UID1000, two CPUs,3GiB RAM/no extra swap,256 PIDs and no Linux capabilities.
No real credential, provider configuration or live application data is mounted.

Separate PostgreSQL17.11 uses the already present image
sha256:ac08538c6f8b9904c33c8224c5e5706dbe760aca29db1d096972b4052c22a75d,
new private data,512MiB RAM and one CPU; fsync is on. Its internal Docker network
has explicit subnet10.249.115.0/28 and no host port. The default network pool was
exhausted; this new range was checked against existing routes/networks. Initial
network/logging setup failures occurred before any test or model execution and
are retained separately. No existing network was removed.

The all-listeners readiness unit passed under race detection (1.048s). The
fresh five-artifact build and full57-stage scripted collector completed,
retaining private proof through BUTLER_EXPERIENCE_PROCESS_PROOF. Readiness and
compilation alone are not acceptance; the completed scripted results appear
below. No model comparison is admitted by this document; a future server run
requires its own current one-use recipe/domain/source/artifact/preseal binding,
unchanged historical accounting and the original finite limits.

## Completed zero-native results

All five fresh binaries were built. The full scripted collector passed all57
subcases, including the formerly failing control033. The23 other app tests and
seven source/lease checks passed with zero skips or race reports. Inventory
reconciliation proved that all31 originally required test names actually ran:
24 belong to app, six to orchestrator and one to memory/catalog. The earlier
statement that all seven omitted names belonged to orchestrator was imprecise;
the catalog test is now routed to its actual package.

The new recipe-derived read comparison passed all10 subcases: narrow grant
denied, authorized complete grant read both lessons and completed, recipient
dispatches unchanged. The first operator invocation rejected the network-DNS
database before its integration ran; its failure stays preserved. A separate
PostgreSQL namespace provided the existing required127.0.0.1:55451 endpoint.
The unchanged operator then passed seven tests/38 subcases, including actual
synthetic PostgreSQL recovery/CAS. No guard was relaxed. Vet passed for app,
Input, Core, transport and memory/catalog. Both new test databases were stopped
after joining workers; their data and private proof are retained.

Every source file still matches the original complete6282-file inventory.
Commands, explicit failures, per-log hashes and count receipts are in
experiments/memory115/2026-10-28/laba-zero-native-v1. Raw logs/proof remain in the
private server directory; no credential fixture or synthetic private history is
published. This establishes current scripted integration, not the sole cause of
the old failure or a useful model-quality improvement.

Native entry is still unbound. The old operator literally invokes a WSL chroot
and owns a PC-specific cgroup/source/auth handback contract. It cannot simply be
launched through SSH or have its guards bypassed. A separately fixed server host
adapter and current one-use day/recipe/preseal must preserve those guarantees
before the single new Gemini comparison. All prior known/unknown expense and
the old failed57-stage denominator remain unchanged.
