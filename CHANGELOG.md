# Changelog

## [0.11.0](https://github.com/andymwolf/agentium/compare/v0.10.0...v0.11.0) (2026-02-13)


### Features

* add structured phase I/O to Langfuse span traces ([#521](https://github.com/andymwolf/agentium/issues/521)) ([c228c76](https://github.com/andymwolf/agentium/commit/c228c7605479e20fa053d1029843bd2d8939bacc))
* add system/skills prompts to Langfuse generation traces ([#520](https://github.com/andymwolf/agentium/issues/520)) ([166b01d](https://github.com/andymwolf/agentium/commit/166b01dd44a12bde026b9e9f7f03e6eb2e10e07d))


### Bug Fixes

* consistent return tuple for handleVerifyPhase and add phaseLoopContext ownership docs ([#518](https://github.com/andymwolf/agentium/issues/518)) ([d8d10cf](https://github.com/andymwolf/agentium/commit/d8d10cf108267bd73ef29c05894b6eab911d33ee))

## [0.10.0](https://github.com/andymwolf/agentium/compare/v0.9.0...v0.10.0) (2026-02-13)


### Features

* add Langfuse generation inputs (prompt text) ([8e922e5](https://github.com/andymwolf/agentium/commit/8e922e5de110b71040bc41a34e16776abd809570)), closes [#503](https://github.com/andymwolf/agentium/issues/503)
* add Langfuse generation inputs and outputs ([7a0c508](https://github.com/andymwolf/agentium/commit/7a0c508c1c3d9ab60c99c926145519cc363b1959))
* add Langfuse generation outputs (response text) ([1fb5f3c](https://github.com/andymwolf/agentium/commit/1fb5f3cb85ccccd0fa5a607146d0e25eae627acb)), closes [#503](https://github.com/andymwolf/agentium/issues/503)
* add start/end times to Langfuse generation events ([27c06e9](https://github.com/andymwolf/agentium/commit/27c06e91096d2d58f5cb7229545dd27e597aa6aa))
* add start/end times to Langfuse generation events ([220c40c](https://github.com/andymwolf/agentium/commit/220c40c81e52434f3e80fe96dab6e1536d0f1bf8))
* phase-based comment routing for all roles ([#509](https://github.com/andymwolf/agentium/issues/509)) ([fdd5c8e](https://github.com/andymwolf/agentium/commit/fdd5c8e97dfb09fa080909ff08cb1b09daceca7d)), closes [#508](https://github.com/andymwolf/agentium/issues/508)


### Bug Fixes

* always initialize memory store, fix feedback delivery, add branch sync ([#513](https://github.com/andymwolf/agentium/issues/513)) ([c3651cb](https://github.com/andymwolf/agentium/commit/c3651cbbc080e92209b420364fc0c9021596021a))
* respect defaults.max_duration and defaults.agent from config file ([#506](https://github.com/andymwolf/agentium/issues/506)) ([b9081a7](https://github.com/andymwolf/agentium/commit/b9081a719bf615445b289bbf98b39500e51cb58c))

## [0.9.0](https://github.com/andymwolf/agentium/compare/v0.8.1...v0.9.0) (2026-02-12)


### Features

* phase-scoped context clearing and iteration tracking ([#498](https://github.com/andymwolf/agentium/issues/498)) ([eac3a64](https://github.com/andymwolf/agentium/commit/eac3a6419aa2b86820b0396df092a476d1d1ca38))


### Bug Fixes

* add Langfuse base_url to example config for US region ([#497](https://github.com/andymwolf/agentium/issues/497)) ([f137252](https://github.com/andymwolf/agentium/commit/f137252af651b393c2e4743384507f796b7ff1aa))
* add Langfuse response parsing and diagnostic logging ([#495](https://github.com/andymwolf/agentium/issues/495)) ([9799f0a](https://github.com/andymwolf/agentium/commit/9799f0a91dc14830affdced6aff46680751fa446))
* correct controller Dockerfile Go version and remove aider image build ([#502](https://github.com/andymwolf/agentium/issues/502)) ([8519a18](https://github.com/andymwolf/agentium/commit/8519a1831de896e185fa1995db6a828efc3a3247))
* parse token usage from top-level result event fields ([#501](https://github.com/andymwolf/agentium/issues/501)) ([c177e68](https://github.com/andymwolf/agentium/commit/c177e68318418c2db8f383f7c1313eaf66387ada))
* simplify BigQuery schema and add Cloud Logging error visibility ([#500](https://github.com/andymwolf/agentium/issues/500)) ([0596eaf](https://github.com/andymwolf/agentium/commit/0596eaf084f1061216b0f1738e282e1e3bf45d05))

## [0.8.1](https://github.com/andymwolf/agentium/compare/v0.8.0...v0.8.1) (2026-02-12)


### Bug Fixes

* redesign continuation prompt for phase iteration 2+ ([#492](https://github.com/andymwolf/agentium/issues/492)) ([6580735](https://github.com/andymwolf/agentium/commit/6580735fab55393012f6a7cd8d4c435fb122d0eb)), closes [#491](https://github.com/andymwolf/agentium/issues/491)
* resolve CodeQL scanning alerts for clear-text-logging and url-sanitization ([768a105](https://github.com/andymwolf/agentium/commit/768a105950e809043fd4f079690fdb673b6afdb5)), closes [#488](https://github.com/andymwolf/agentium/issues/488)

## [0.8.0](https://github.com/andymwolf/agentium/compare/v0.7.1...v0.8.0) (2026-02-12)


### Features

* check GitHub native blockedBy relationships before processing issues ([#478](https://github.com/andymwolf/agentium/issues/478)) ([97cdb3e](https://github.com/andymwolf/agentium/commit/97cdb3e18a45e6b72eb3433c78126a013d496ee9)), closes [#477](https://github.com/andymwolf/agentium/issues/477)


### Bug Fixes

* add phase-specific handoff re-emit instructions to continuation feedback ([3aa017a](https://github.com/andymwolf/agentium/commit/3aa017af5f62cf49bf224376c3d9a43c0dfe4dc4)), closes [#485](https://github.com/andymwolf/agentium/issues/485)
* continuation mode workers not re-emitting AGENTIUM_HANDOFF after feedback ([a3a14f7](https://github.com/andymwolf/agentium/commit/a3a14f7797a330f15fab35a5bc1f0d41d2ad0546))
* harden reviewer against 'plan not on disk' confusion ([#474](https://github.com/andymwolf/agentium/issues/474)) ([c669de5](https://github.com/andymwolf/agentium/commit/c669de505c0a5d88a320a9e84de688c567b65542)), closes [#473](https://github.com/andymwolf/agentium/issues/473)
* systematic logging for Langfuse diagnostics and gcloud secret parsing ([#476](https://github.com/andymwolf/agentium/issues/476)) ([b7a18a5](https://github.com/andymwolf/agentium/commit/b7a18a5446904071fbdb5183688e6bfad516240b))

## [0.7.1](https://github.com/andymwolf/agentium/compare/v0.7.0...v0.7.1) (2026-02-12)


### Bug Fixes

* correct gofmt alignment in fallback_test.go ([#471](https://github.com/andymwolf/agentium/issues/471)) ([dc1a87a](https://github.com/andymwolf/agentium/commit/dc1a87a0bf5986f4bd130c0d90c81d00ae1ea4e4))

## [0.7.0](https://github.com/andymwolf/agentium/compare/v0.6.0...v0.7.0) (2026-02-12)


### Features

* accept full step config via API ([#457](https://github.com/andymwolf/agentium/issues/457)) ([4bb5aa1](https://github.com/andymwolf/agentium/commit/4bb5aa1d0421d16c5fb9178d9dd20e7357528ef1))
* add --container-reuse CLI flag ([#462](https://github.com/andymwolf/agentium/issues/462)) ([bf3689c](https://github.com/andymwolf/agentium/commit/bf3689c63e8efce884b859d1e7b0a7398295d775))
* API-based sub-issue detection, queue ordering fix, tracker→parent rename ([#452](https://github.com/andymwolf/agentium/issues/452)) ([cf49e5d](https://github.com/andymwolf/agentium/commit/cf49e5db761ffb38117b782a51bc5bbf952f60a5))
* fetch issue comments for agent context ([#448](https://github.com/andymwolf/agentium/issues/448)) ([95950d5](https://github.com/andymwolf/agentium/commit/95950d5b9a7b30b62365530e2a0db8d662f961f0))
* load Langfuse keys from GCP Secret Manager ([#458](https://github.com/andymwolf/agentium/issues/458)) ([a23fef3](https://github.com/andymwolf/agentium/commit/a23fef325b31e5cb408f7e77cffae9e32f6bae83))
* long-lived phase containers to reduce container churn and token waste ([#460](https://github.com/andymwolf/agentium/issues/460)) ([110cac3](https://github.com/andymwolf/agentium/commit/110cac3e584420143d534e89d983400bf7021565))
* remove session-level max iterations; improve run output ([0f2739d](https://github.com/andymwolf/agentium/commit/0f2739d8aa7c588874dad86c85c8c37da33383a4))


### Bug Fixes

* add Langfuse Secret Manager config and diagnostic logging ([15810f9](https://github.com/andymwolf/agentium/commit/15810f93ee0af83a1bd825c0a70803fc3aaa690b))
* add Langfuse Secret Manager config and diagnostic logging ([e43febe](https://github.com/andymwolf/agentium/commit/e43febe4aef9bd8e6f8bed63d4ee2e427ea706ed)), closes [#467](https://github.com/andymwolf/agentium/issues/467)
* correct BigQuery table partitioning and schema for Cloud Logging ([#464](https://github.com/andymwolf/agentium/issues/464)) ([0091d52](https://github.com/andymwolf/agentium/commit/0091d528b71bea791c2a92d51f7590077668dbd2)), closes [#463](https://github.com/andymwolf/agentium/issues/463)
* make buildReviewPrompt phase-aware so PLAN phase skips code review ([#466](https://github.com/andymwolf/agentium/issues/466)) ([79b1185](https://github.com/andymwolf/agentium/commit/79b1185b9e9dba82b3ef8cdef23b6a773c9652f5)), closes [#465](https://github.com/andymwolf/agentium/issues/465)
* prepend container entrypoint in pooled docker exec ([#470](https://github.com/andymwolf/agentium/issues/470)) ([ac25675](https://github.com/andymwolf/agentium/commit/ac256755b10bd45a7c8825423c0db674d0a57cc2))
* prevent branch contamination and PR mismatch between tasks ([#454](https://github.com/andymwolf/agentium/issues/454)) ([e76f799](https://github.com/andymwolf/agentium/commit/e76f799f2631ec9e601ada91d9d2434ab70ca730)), closes [#453](https://github.com/andymwolf/agentium/issues/453)

## [0.6.0](https://github.com/andymwolf/agentium/compare/v0.5.0...v0.6.0) (2026-02-11)


### Features

* accept parameters in task request for prompt template injection ([#426](https://github.com/andymwolf/agentium/issues/426)) ([7ceb5db](https://github.com/andymwolf/agentium/commit/7ceb5db6210b5c450e25591bbfbb8b865403ccec)), closes [#417](https://github.com/andymwolf/agentium/issues/417)
* add Langfuse observability integration ([2ad0d51](https://github.com/andymwolf/agentium/commit/2ad0d5132a66cd9f156ec54e2ef037cc57df2714))
* add Langfuse observability integration for Go controller and TypeScript API ([df97d21](https://github.com/andymwolf/agentium/commit/df97d210346972aceee108f86c25bb6f7cfcb815)), closes [#438](https://github.com/andymwolf/agentium/issues/438)
* add tier-aware label validation and tracker/sub-issue support ([#435](https://github.com/andymwolf/agentium/issues/435)) ([ae94786](https://github.com/andymwolf/agentium/commit/ae9478655f72e6802f44a8905605a6302f02cc9f))
* add VM resource monitoring for memory pressure visibility ([95dd59b](https://github.com/andymwolf/agentium/commit/95dd59b70210f0bd458242c2a8bd44074915aa70))
* add VM resource monitoring for memory pressure visibility ([febc1ab](https://github.com/andymwolf/agentium/commit/febc1abeaed559faef6566ebcb5c0577fcb31891)), closes [#441](https://github.com/andymwolf/agentium/issues/441)
* bump default VM to e2-standard-2 and add dynamic Docker memory limits ([ea4442e](https://github.com/andymwolf/agentium/commit/ea4442eebbc312db79b2b9dcb78879e50e64169d)), closes [#442](https://github.com/andymwolf/agentium/issues/442)
* remove arbitrary 1000-character issue body truncation ([d0ed69a](https://github.com/andymwolf/agentium/commit/d0ed69ad399aa895c8d19a14d95c6b7756f65478)), closes [#445](https://github.com/andymwolf/agentium/issues/445)
* Support skip_on conditions for reviewer/judge ([#433](https://github.com/andymwolf/agentium/issues/433)) ([adad146](https://github.com/andymwolf/agentium/commit/adad146d385fcb7370233c7d90b9abee3f93890b))


### Bug Fixes

* add logging.viewer role to provisioner IAM requirements ([#432](https://github.com/andymwolf/agentium/issues/432)) ([58a4fcb](https://github.com/andymwolf/agentium/commit/58a4fcb5d195e6193a59eedf6db0f3f40c838d47))
* correct BigQuery labels schema and surface gcloud stderr in logs command ([#431](https://github.com/andymwolf/agentium/issues/431)) ([2fc80fc](https://github.com/andymwolf/agentium/commit/2fc80fc89a4fbd6c90e160468dee2d2faf5fa4c2))
* Derive issue_url fallback and guard issue_number on task type ([#430](https://github.com/andymwolf/agentium/issues/430)) ([3ef73fb](https://github.com/andymwolf/agentium/commit/3ef73fbc2e34f84497adce1672a701740e7c88bb)), closes [#428](https://github.com/andymwolf/agentium/issues/428) [#429](https://github.com/andymwolf/agentium/issues/429)
* harden Langfuse observability lifecycle and error handling ([95d4189](https://github.com/andymwolf/agentium/commit/95d4189d69bbf22c04b61a69acb6335e3b06a8ed)), closes [#438](https://github.com/andymwolf/agentium/issues/438)
* prevent double-close panic in LangfuseTracer.Stop() ([4ec71f6](https://github.com/andymwolf/agentium/commit/4ec71f637ac5ef2417919190b7be70c8bc78441a))
* resolve Cloud Logging PermissionDenied on session VMs ([#437](https://github.com/andymwolf/agentium/issues/437)) ([eef9834](https://github.com/andymwolf/agentium/commit/eef983490f786e6fd0184d13d114e4403963fd73))

## [0.5.0](https://github.com/andymwolf/agentium/compare/v0.4.1...v0.5.0) (2026-02-10)


### Features

* Add BigQuery reporting module for token consumption ([#106](https://github.com/andymwolf/agentium/issues/106)) ([#409](https://github.com/andymwolf/agentium/issues/409)) ([0ed61cb](https://github.com/andymwolf/agentium/commit/0ed61cb4ce0ebe39fbd942cd2f703fb65bc0f237))
* Add service_account_key support for GCP authentication ([#423](https://github.com/andymwolf/agentium/issues/423)) ([3e4eee2](https://github.com/andymwolf/agentium/commit/3e4eee2485f6e2d65d4817a198820e0951ce0666))
* Auto-enable required GCP APIs in Terraform module ([09bf9c2](https://github.com/andymwolf/agentium/commit/09bf9c2315ca6294b4e025b3cb2752bf0b0b74ef))
* Auto-enable required GCP APIs in Terraform module ([dde657c](https://github.com/andymwolf/agentium/commit/dde657c6f216aa10a52bb3c44b489798ff9a0ae2)), closes [#424](https://github.com/andymwolf/agentium/issues/424)
* Skip reviewer/judge in VERIFY phase ([#412](https://github.com/andymwolf/agentium/issues/412)) ([5e80928](https://github.com/andymwolf/agentium/commit/5e809288bbfd0f421cadc82d1f772bfb5c6e8dae))


### Bug Fixes

* Add placeholder BigQuery table so views can be created on first apply ([#411](https://github.com/andymwolf/agentium/issues/411)) ([13e315e](https://github.com/andymwolf/agentium/commit/13e315eda64694bdd94a61b5f04b918d49563038)), closes [#106](https://github.com/andymwolf/agentium/issues/106)
* Make reviewer dependency-aware to prevent false scope creep findings ([#416](https://github.com/andymwolf/agentium/issues/416)) ([ae8243d](https://github.com/andymwolf/agentium/commit/ae8243d41acadd3e9ec309646d16cd6363f624f7)), closes [#415](https://github.com/andymwolf/agentium/issues/415)

## [0.4.1](https://github.com/andymwolf/agentium/compare/v0.4.0...v0.4.1) (2026-02-08)


### Bug Fixes

* Pass --max-duration to Terraform and fix CLI flag precedence ([#404](https://github.com/andymwolf/agentium/issues/404)) ([9a8ed6a](https://github.com/andymwolf/agentium/commit/9a8ed6ad0e836a4c95d28e62ba347b927f69f173))

## [0.4.0](https://github.com/andymwolf/agentium/compare/v0.3.0...v0.4.0) (2026-02-07)


### Features

* Add role identification to comments and remove worker complexity self-declaration ([#403](https://github.com/andymwolf/agentium/issues/403)) ([76bcf8e](https://github.com/andymwolf/agentium/commit/76bcf8e5ff47c4333e5293fd3cf622bc43ab9ae4))


### Bug Fixes

* Resolve gofmt struct field alignment in summarize_test.go ([ec7c6d5](https://github.com/andymwolf/agentium/commit/ec7c6d5e73d4fe209f28b7ea9127b11e63212629))
* Strip stream-of-thought content from GitHub comments ([#401](https://github.com/andymwolf/agentium/issues/401)) ([c2e5f54](https://github.com/andymwolf/agentium/commit/c2e5f543d4ce96c5137470d6664e195104f0b058))

## [0.3.0](https://github.com/andymwolf/agentium/compare/v0.2.0...v0.3.0) (2026-02-06)


### Features

* Add adapter-agnostic agent event abstraction ([#398](https://github.com/andymwolf/agentium/issues/398)) ([6f15bc4](https://github.com/andymwolf/agentium/commit/6f15bc4b2f07fb1afb61ee546db5464c06ad6193))
* Add worker feedback responses and remove reviewer verdict guardrail ([c6b8972](https://github.com/andymwolf/agentium/commit/c6b89728e33955f6ac73136d5c4caddec71fec89))
* Add worker feedback responses and remove reviewer verdict guardrail ([d72c5be](https://github.com/andymwolf/agentium/commit/d72c5be337386307e62fbadb875804b48d2ed5a0))
* Give judge access to its own prior ITERATE directives ([51e902f](https://github.com/andymwolf/agentium/commit/51e902f437317c3c11fc4712a4ae9cbced03f048))
* Give judge access to its own prior ITERATE directives ([3b49e00](https://github.com/andymwolf/agentium/commit/3b49e00c53cbce43c1725baaf54f7cdcfb6cbf28))
* Improve W-R-J prompts with security criteria, code inspection, and approach guidance ([#400](https://github.com/andymwolf/agentium/issues/400)) ([9142028](https://github.com/andymwolf/agentium/commit/914202890354efc037c480fc947e1accd2ac350c)), closes [#399](https://github.com/andymwolf/agentium/issues/399)


### Bug Fixes

* Resolve lint errors (gofmt alignment, variable shadow) ([4e0cad9](https://github.com/andymwolf/agentium/commit/4e0cad944f5231074d0489fb6ce1f220621fdb14))
