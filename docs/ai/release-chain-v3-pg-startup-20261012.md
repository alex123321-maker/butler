# Closed prospective chain PostgreSQL storage

This independently prepared prototype is retained in the separate local branch
`codex/scenario-release-chain-v3-pg-storage`, final documentation commit
`f45c980735693edc43d36d57a8a69679c34b1d3f`. The issue33 PR retains this operational
evidence only; its unestablished500-call prerequisite means the speculative
runner is not integrated or enabled. This storage failure is distinct from the
actual V3 encoding failure and does not establish its cause.

The future500→540 chain remains closed: its V3 PASS bridge is empty and the
actual V3 failed at266. This change authorizes no native start,provider import,
TLS,model load,old-window reuse or production deployment.

The f3fad3d prepare failed before tests at
`/srv/storage/butler/agents/scenario-release-chain-v3-high-20261012/reports/chain-high-prepare-20261012T162053Z-2302997`.
Build/vet had completed; no tests.jsonl existed. The independent same-image,
same-limit,network-none PostgreSQL probe was preserved at
`/srv/storage/butler/agents/scenario-release-chain-v3-high-20261012/reports/pg-startup-diagnosis-20261012T163443Z-2353137`.
Its safe state is exit1,OOMfalse,readyfalse; fixed startup classification is
INITDB_NO_SPACE. The private startup log contains No-space/initdb evidence and
no shared-memory failure. Raw logs are not exported here. Log SHA256:
`ff64553ed89336ed67e5bb827ca82bf6dd15cc7381b0a161b5acdaa19bf27b49`;
state SHA256:
`8c12de5ab1a73a42fc2a10fa80880351c5ff0e0d1fd6c9b7c93b7fa7b8b5ab0a`.

DockerRootDir is `/var/lib/docker` on the system filesystem. At diagnosis it
had about29MiB free/100% used,while `/srv/storage` had about44GiB free/89% used;
inodes were available on both. The image declares an anonymous data volume,
and the prepare runner supplied no explicit data bind. Therefore its initdb
writes landed on the full system filesystem. This is an observed startup
storage failure,not a model,credential,PG query,timeout or OOM diagnosis.

The runner now binds `$proof/postgres` under the isolated storage-backed proof
root to `/var/lib/postgresql/data`,as the working shadow runner does. Every
prepare still has its own private database. Images,PG settings,512MiB RAM,
CPU/pid limits,readiness count,3-minute test fence and native gates stay unchanged.
No global prune,unrelated delete,data move or old-report rewrite was performed.

The bounded same-image/settings/network-none follow-up proof is
`/srv/storage/butler/agents/scenario-release-chain-v3-high-20261012/reports/pg-startup-storage-20261012T164227Z-2379377`.
With only the data bind changed,it was ready,SELECT1 succeeded,and after2s it
remained running/exit0/OOMfalse with no No-space marker. Model/auth calls=0.
The original failures remain separate. This does not by itself complete the
32 named owning-PG tests or approve any native chain.

The subsequent closed prepare used exact executable commit
`a520eb165bcf75e01d72eac62df58714c5696f6e`,snapshot `20261012-234353-789`:
`/srv/storage/butler/agents/scenario-release-chain-v3-high-20261012/reports/chain-high-prepare-20261012T164432Z-2385813`.
All32 named owning-PG checks /12 top-level passed,0 failures/skips,race/vet,
elapsed14.46s. The build/vet log is empty. Events SHA256:
`fb586e927108a94315dfeaf0d8668fc8b2fdbe0c0a1816294f72928b15a3c4fb`;
source-manifest SHA256:
`38282eb60b674e5d95e55a8e7b1eaa685f34a6a08464ce740fd43fec008591ec`.
PG-only fixed state SHA256:
`a49c8a8760af854041776093cc015c81bd9389ae4b9dc4cc5544667e935f141f`;
private startup-log SHA256:
`6acd9c29c62fc1b3bf5f4f2ebd5a171c23b31d70831a7b2b553d5824895f3080`.
This section was appended after testing with no further executable change.
The report explicitly retains execution_enabled=false and prospective baseline500;
no V3 PASS bridge or model authority has been created. Native/auth/TLS/load=0.
