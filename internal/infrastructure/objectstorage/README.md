# infrastructure/objectstorage

Adapters that implement the object-storage seam (`internal/usecase/boundary/objectstorage`)
against a concrete storage service.

## Role

Implements the `objectstorage.Storage` port. `New` in this package selects the implementation;
`s3/` holds the S3-compatible one. Vendor vocabulary (bucket / region / endpoint / SDK types)
stops here — the port above exposes only a key, bytes, and content metadata.

## Directory Structure

|Path|Role|
|---|---|
|`objectstorage.go`|The single place that chooses an implementation. Both the DI graph and the CLI call it, so retargeting the substrate is a one-function edit|
|`s3/`|S3-compatible adapter (AWS SDK v2). `New(Config, TracerFactory)` returns the port; the concrete type stays unexported|

## Layout convention

- **Substrate-agnostic contract** lives at `internal/usecase/boundary/objectstorage` (the seam),
  above this layer — not here. Infrastructure implements that port; it does not own the abstraction.
- **Substrate-specific adapter** lives at `objectstorage/<substrate>/` (e.g. `objectstorage/s3`).
  The package name is the substrate, so the concrete technology stays visible at the import site.

## Port mapping

| seam | S3 |
| --- | --- |
| `Put(ctx, PutObject) (Path, error)` | `PutObject`. `Key` → `Key` (under the configured `Bucket`), `Body` → body + `ContentLength`, `ContentType` → `ContentType`, `CacheControl` → `CacheControl`. An empty `CacheControl` leaves the field unset rather than sending an empty header, so "no caching directive" stays distinguishable from "an empty one" |
| return `Path` | The key that was written, echoed back. The adapter never returns a URL — composing one is the caller's job, because the delivery origin is not a property of the store |
| `List(ctx, ListQuery) (ListResult, error)` | `ListObjectsV2`. `Prefix` → `Prefix`, `Limit` → `MaxKeys`, `Cursor` → `ContinuationToken`; each `Contents` entry becomes `Object{Key, ModifiedAt}` from `Key` + `LastModified`. `NextCursor` carries `NextContinuationToken` **only when `IsTruncated`** — otherwise it is empty, so "there is more" stays distinguishable from "this was the last page". An unset `Prefix` / `Cursor` leaves the field off the request rather than sending an empty string |
| `Delete(ctx, keys) error` | `DeleteObjects` with `Quiet: true`. Each key becomes an `ObjectIdentifier`; an empty `keys` short-circuits without a request. S3 reports per-key failures in the response body rather than as a call error, so a non-empty `Errors` is turned into a failure — silently treating an undeleted key as deleted would make a reclamation job stop reporting the object it failed to remove |

## Error normalization

Every SDK failure is wrapped into `apperror.ErrUnavailable` at each call site (`Put` / `List` /
`Delete`), so upper layers branch on the sentinel and never inspect an AWS error type. This is
deliberately **coarser than the RDB side**: `rdb/pgerror` maps SQLSTATE to distinct sentinels
because callers act differently on a unique-violation than on a deadlock, whereas every failure
of this port means "the store did not accept the operation". `Delete` in particular has no
not-found case to distinguish — S3 treats deleting an absent key as success, which is what makes
a reclamation job safe to re-run. Splitting the mapping is worth doing when an operation is added
whose caller must distinguish not-found from denied.

## Config

`s3.Config` is populated from `OBJECT_STORAGE_*` (see [env/README.md](../../../env/README.md)):

- `Endpoint` — empty means SDK default resolution, i.e. real AWS S3. A non-empty value points at
  a compatible service (locally, the Garage container)
- `UsePathStyle` — must be `true` for Garage / MinIO, `false` for AWS S3
- `Region` is used for request signing even against a non-AWS service

## Observability

`Put` opens an infrastructure-layer span via the injected `TracerFactory`, so a store call appears
in the same trace as the handler and usecase that triggered it. The adapter emits no logs of its
own; failures surface through the normalized error.

## Wired by default — unlike the SQS adapter

This adapter **is** in the default DI graph, so `aws-sdk-go-v2/service/s3` and the SDK core are
linked into the shipped binary. [`queue/sqs`](../queue/sqs/README.md) is wired only from the
removable sample set, so `service/sqs` leaves the binary once the sample is removed
([ADR-0053 (broker-sdk-isolation-measured-as-coupling)](../../../docs/adr/0053-broker-sdk-isolation-measured-as-coupling.md)).

The asymmetry is intentional: a worker has no broker until one is chosen, whereas the
object-storage port is exercised out of the box and needs a working implementation
to be more than a declaration. A deployment that stores nothing can drop `objectStorageModule()` from
`InfrastructureModule()`.

## Test strategy

- **Unit tests run against `gofakes3` in-process** — no container, so `make go-test` needs nothing
  running. The fake is started per test and speaks enough of the S3 API to exercise the adapter
- **The Garage container is for `make serve`**, not for tests. Anything asserted about real
  delivery (public read, cache headers) is verified by hand against that container
- `gofakes3` does not persist every header it receives (`Cache-Control` among them), so assertions
  about headers the adapter *sends* inspect the outgoing request rather than the stored object

## S3 is one worked example, not the target

The seam is substrate-agnostic, but an abstraction shipped with nothing behind it proves nothing — the
port is only credible once something real is wired through it. So one substrate is implemented
concretely, and the S3 API is that one. It is the **reference**, not the assumption: the S3 API is
the closest thing this space has to a lingua franca, which is why the local container is Garage
rather than AWS itself.

To keep that claim honest rather than aspirational, here is the local container each major
provider's object storage is developed against, so the equivalent loop can be stood up on day one
after retargeting.

|Provider|Service|Local container|License|Published by|
|---|---|---|---|---|
|AWS (and any S3-compatible)|S3|`dxflrs/garage`|AGPL-3.0|Deuxfleurs (non-profit)|
|Azure|Blob Storage|`mcr.microsoft.com/azure-storage/azurite`|MIT|Microsoft|
|GCP|Cloud Storage|`fsouza/fake-gcs-server`|BSD-2-Clause|fsouza (individual maintainer)|

Selection follows the same rule as every other dependency here — one replaceable job per component
([ADR-0081 (library-selection-policy)](../../../docs/adr/0081-library-selection-policy.md)) — so a single-purpose emulator is
preferred over a suite that emulates a whole cloud. Notes per choice:

- **Garage** speaks the S3 API and nothing else, and stays small enough to run per checkout. Its
  AGPL-3.0 terms attach to distributing a modified Garage; running the published image as a
  development dependency places no obligation on the application that talks to it
- **Azurite** is Microsoft's own emulator and covers Blob *and* Queue, so Azure needs one container
  for both this seam and the worker seam
- **fake-gcs-server** is the de-facto Cloud Storage emulator and the only one with a real user base,
  but it is the one entry here maintained by an individual rather than an organization, and its
  tagged releases are slower than the others. Weigh that before depending on it in CI

Anything S3-compatible (MinIO, Ceph RGW, Cloudflare R2, a managed S3) needs no adapter change at
all — only `ENDPOINT_OBJECT_STORAGE` and credentials. A non-S3 substrate needs a sibling package
under `objectstorage/`, with nothing above this layer changing.

## Credentials

Credentials are resolved by [`infrastructure/awsclient`](../awsclient/README.md), which the SQS
adapter shares. Leaving `OBJECT_STORAGE_ACCESS_KEY_ID` / `OBJECT_STORAGE_SECRET_ACCESS_KEY` empty
hands resolution to the SDK's default chain, so a deployment on an IAM role needs no code change;
setting both overrides the chain, which is what the local Garage credentials do. Either way the
resolution is attempted at startup, so an unusable configuration fails there rather than on the
first upload.
