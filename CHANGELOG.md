# Changelog

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
