# Retained artifacts

The existing PostgreSQL artifact repository owns assistant/tool/browser artifacts.
`SaveMedia` adds bounded private `communication_media` records in the same store
with additive migration 091. It generates a fresh 128-bit artifact identity and
stores name, MIME, SHA-256 and base64 bytes as `private_media_v1`. No filesystem
path, public URL, secret or channel token is model-selected.

`ResolveMediaTx` requires a current artifact/source/export policy callback and
checks owning-session equality, privacy redaction and data integrity. A missing
guard denies access. PNG/JPEG files are decoded for dimensions and MIME; SVG and
arbitrary image formats are rejected. V1 supports at most 10 MiB per artifact,
with image dimension sum at most 10000 and aspect ratio at most 20. Documents
support plain text, PDF and generic binary uploads; they are not executed.

Channel manifests pin the digest and metadata and resolve bytes again before
sending. The existing Catalog privacy lifecycle clears artifact content JSON;
redacted data cannot be read. Migration rollback refuses retained media history,
including redacted records, instead of deleting it. Default startup is unchanged.
