# infrastructure/queue

Adapters that implement the worker seam (`internal/usecase/boundary/worker`) against a
concrete message broker.

## Layout convention

- **Broker-agnostic contract** lives at `internal/usecase/boundary/worker` (the seam), above
  this layer — not here. Infrastructure implements those ports; it does not own the abstraction.
- **Broker-specific adapter** lives at `queue/<broker>/` (e.g. `queue/sqs`). The package name is
  the broker, so the concrete technology stays visible at the import site.
- **Code shared across brokers** lives directly under `queue/`. Shared code is extracted only
  when two or more adapters duplicate a concrete implementation detail, so a helper is hoisted
  from observed duplication rather than designed up front.

## SQS is one worked example, not the target

The seam is broker-agnostic, but an abstraction shipped with nothing behind it proves nothing — the
port is only credible once something real is wired through it. So one broker is implemented
concretely, and SQS is that one. It is the **reference**, not the assumption: retargeting means
adding a sibling package under `queue/`, with nothing above this layer changing.

To keep that claim honest rather than aspirational, here is the local container each major
provider's queue is developed against, so the equivalent loop can be stood up on day one after
retargeting.

|Provider|Service|Local container|License|Published by|
|---|---|---|---|---|
|AWS|SQS|`softwaremill/elasticmq-native`|Apache-2.0|SoftwareMill|
|Azure|Queue Storage|`mcr.microsoft.com/azure-storage/azurite`|MIT|Microsoft|
|GCP|Pub/Sub|`gcr.io/google.com/cloudsdktool/google-cloud-cli:emulators`|Google Cloud SDK terms|Google|

Selection follows the same rule as every other dependency here — one replaceable job per
component ([ADR-0081 (library-selection-policy)](../../../docs/adr/0081-library-selection-policy.md)) — so a single-purpose
emulator is preferred over a suite that emulates a whole cloud. Notes per choice:

- **ElasticMQ** emulates SQS and nothing else, and its native image is small enough to start per
  test run. A multi-service AWS emulator would cover more surface, but it buys that with a
  many-jobs-one-container dependency and a commercially gated feature line
- **Azurite** is Microsoft's own emulator and covers Blob *and* Queue, so Azure needs one
  container for both seams. For Service Bus semantics (topics / subscriptions / sessions) there
  is a separate first-party emulator, but it ships under a proprietary EULA and pulls SQL Server
  alongside it — reach for it only when Queue Storage genuinely does not fit
- **Pub/Sub** has no standalone first-party image; the emulator is a component of the Google
  Cloud CLI, published by Google under the `:emulators` tag. It is heavier than the other two.
  Third-party slim wrappers exist and are correspondingly less accountable

## Test Strategy

The substrate is a message broker, not a database, so the infrastructure layer's real-DB strategy does
not apply. `queue/` itself holds no Go code; these viewpoints govern the adapters under it, and an
adapter that needs its own is expected to declare one in its package README rather than widen this.

- **The broker is a generated mock of the SDK's API client, and the container above is not a test
  dependency.** `make go-test` starts nothing: the emulator table exists for `make serve` and for manual
  verification of behaviour an SDK mock cannot represent (real visibility timeouts, redrive). Naming a
  queue URL in a test is a fixture string, never a connection.
- **The SDK call is asserted, not just the outcome.** An adapter's job is to translate a boundary call
  into one SDK request, so the input handed to the mock is inspected — queue URL, body, and the message
  attributes carrying the boundary's headers. Asserting only the returned error would let that
  translation drift unnoticed.
- **Error normalisation is pinned per broker error class**, because that mapping is the whole reason the
  adapter exists as a layer: a retryable broker fault, a permanent rejection, and a cancelled context
  must each reach the worker seam as the matching `apperror` sentinel, asserted with `errors.Is` rather
  than by inspecting the SDK error type. `nil` staying `nil` is its own case — a normaliser that
  manufactures an error from a success is silent and total.
- **Sensitive headers are pinned against normalisation gaps**, so header matching cannot be defeated by
  case or surrounding whitespace.
