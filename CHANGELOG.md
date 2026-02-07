# Changelog

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
