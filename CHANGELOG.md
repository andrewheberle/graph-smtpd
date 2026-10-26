# Changelog

## [0.13.1](https://github.com/andrewheberle/graph-smtpd/compare/v0.13.0...v0.13.1) (2026-10-26)


### Bug Fixes

* **deps:** update cgr.dev/chainguard/static:latest docker digest to c12f010 ([#150](https://github.com/andrewheberle/graph-smtpd/issues/150)) ([41a8f3a](https://github.com/andrewheberle/graph-smtpd/commit/41a8f3aaeb6844f3d1c8059a61f3531835c69676))

## [0.13.0](https://github.com/andrewheberle/graph-smtpd/compare/v0.12.2...v0.13.0) (2026-10-25)


### ⚠ BREAKING CHANGES

* **deps:** The secret_file option (--secret.file flag, _SECRET_FILE env var) is now secretfile (--secretfile flag, _SECRETFILE env var) and takes precedence over secret.

### Bug Fixes

* **deps:** update module github.com/microsoftgraph/msgraph-sdk-go to v1.105.0 ([#144](https://github.com/andrewheberle/graph-smtpd/issues/144)) ([5140a92](https://github.com/andrewheberle/graph-smtpd/commit/5140a92df4c3692d92834a779c6929017fb319d7))
* **deps:** update to configger and change secret handling ([#146](https://github.com/andrewheberle/graph-smtpd/issues/146)) ([56d5407](https://github.com/andrewheberle/graph-smtpd/commit/56d54073809c659f20ecf43a886073c511e1391b))

## [0.12.2](https://github.com/andrewheberle/graph-smtpd/compare/v0.12.1...v0.12.2) (2026-10-23)


### Bug Fixes

* **deps:** update module github.com/microsoft/kiota-abstractions-go to v1.11.4 ([#139](https://github.com/andrewheberle/graph-smtpd/issues/139)) ([6b6408d](https://github.com/andrewheberle/graph-smtpd/commit/6b6408d2902b7239c1e0065dc44cea67b1a81bd3))
* **flags:** add secret.file as explicit flag ([#141](https://github.com/andrewheberle/graph-smtpd/issues/141)) ([048b901](https://github.com/andrewheberle/graph-smtpd/commit/048b901fe471729b1ba6024bd32166489339f5da))
* **release:** update release process to support immutable releases ([4ee17be](https://github.com/andrewheberle/graph-smtpd/commit/4ee17bef1d12ebfabdf7ee85921a6c9ed8ab6f64))

## [0.12.1](https://github.com/andrewheberle/graph-smtpd/compare/v0.12.0...v0.12.1) (2026-10-22)


### Bug Fixes

* **deps:** update module github.com/microsoft/kiota-abstractions-go to v1.11.2 ([#135](https://github.com/andrewheberle/graph-smtpd/issues/135)) ([4ea5d7d](https://github.com/andrewheberle/graph-smtpd/commit/4ea5d7d4153704c2ad5959274a0d445c0003dcde))
* **deps:** update module golang.org/x/crypto to v0.58.0 ([#133](https://github.com/andrewheberle/graph-smtpd/issues/133)) ([9af8727](https://github.com/andrewheberle/graph-smtpd/commit/9af8727da71291ddb83c1098e1eb3add28a83935))
* **docker:** switch base image to Chainguard static ([#140](https://github.com/andrewheberle/graph-smtpd/issues/140)) ([e153266](https://github.com/andrewheberle/graph-smtpd/commit/e1532668e9dadd301214d72a96760ccacb2d392a))

## [0.12.0](https://github.com/andrewheberle/graph-smtpd/compare/v0.11.0...v0.12.0) (2026-10-16)


### Features

* **graph-smtpd:** add --users to enable SMTP AUTH ([2eea129](https://github.com/andrewheberle/graph-smtpd/commit/2eea1294581bcb1e3cb37a4c6bb68cc27c856f95))
* **graphserver:** load SMTP AUTH users from an htpasswd file ([2eea129](https://github.com/andrewheberle/graph-smtpd/commit/2eea1294581bcb1e3cb37a4c6bb68cc27c856f95))
* **graphserver:** support SMTP AUTH over TLS ([2eea129](https://github.com/andrewheberle/graph-smtpd/commit/2eea1294581bcb1e3cb37a4c6bb68cc27c856f95))

## [0.11.0](https://github.com/andrewheberle/graph-smtpd/compare/v0.10.0...v0.11.0) (2026-10-16)


### Features

* **graphserver:** log every message with its source and outcome ([#128](https://github.com/andrewheberle/graph-smtpd/issues/128)) ([f18742d](https://github.com/andrewheberle/graph-smtpd/commit/f18742d9b2bf57e214ef2ba05e464b33b1c7b6ef))


### Bug Fixes

* **deps:** update module github.com/microsoft/kiota-abstractions-go to v1.11.1 ([#122](https://github.com/andrewheberle/graph-smtpd/issues/122)) ([131b173](https://github.com/andrewheberle/graph-smtpd/commit/131b173d764588909bffeef5c0f3e88a1c058796))
* **graph-sendmail:** honour --quiet when configuration fails to load ([#127](https://github.com/andrewheberle/graph-smtpd/issues/127)) ([3b57236](https://github.com/andrewheberle/graph-smtpd/commit/3b572367f570e8af334ac8ffc6d4f6dfa69eedaa))

## [0.10.0](https://github.com/andrewheberle/graph-smtpd/compare/v0.9.1...v0.10.0) (2026-10-16)


### Features

* **graph-sendmail:** exit 75 when a send fails temporarily ([2bf4857](https://github.com/andrewheberle/graph-smtpd/commit/2bf485727e98146656dc6dc6d0ece5c58b5fa1fa))
* **sendmail:** mark permanent Graph failures as RejectedError ([#120](https://github.com/andrewheberle/graph-smtpd/issues/120)) ([2bf4857](https://github.com/andrewheberle/graph-smtpd/commit/2bf485727e98146656dc6dc6d0ece5c58b5fa1fa))


### Bug Fixes

* **graphserver:** reject mail permanently when Graph refuses it ([2bf4857](https://github.com/andrewheberle/graph-smtpd/commit/2bf485727e98146656dc6dc6d0ece5c58b5fa1fa))

## [0.9.1](https://github.com/andrewheberle/graph-smtpd/compare/v0.9.0...v0.9.1) (2026-10-16)


### Bug Fixes

* **build:** resolve release error ([#118](https://github.com/andrewheberle/graph-smtpd/issues/118)) ([07c4f7d](https://github.com/andrewheberle/graph-smtpd/commit/07c4f7d7c834aa8589103885ef76fdb6993bb13c))

## [0.9.0](https://github.com/andrewheberle/graph-smtpd/compare/v0.8.3...v0.9.0) (2026-10-16)


### Features

* **graph-sendmail:** limit how long a Graph send can take ([43de881](https://github.com/andrewheberle/graph-smtpd/commit/43de8817a116c2d5c80a728f3a3c610f896f1fe8))
* **graph-sendmail:** support SENDMAIL_SECRET_FILE ([d1aaac2](https://github.com/andrewheberle/graph-smtpd/commit/d1aaac2d64764b00a8c6aa21d7809800031da63e))
* **graph-smtpd:** disconnect idle and stalled clients ([#115](https://github.com/andrewheberle/graph-smtpd/issues/115)) ([43de881](https://github.com/andrewheberle/graph-smtpd/commit/43de8817a116c2d5c80a728f3a3c610f896f1fe8))
* **graph-smtpd:** limit how long a Graph send can take ([43de881](https://github.com/andrewheberle/graph-smtpd/commit/43de8817a116c2d5c80a728f3a3c610f896f1fe8))


### Bug Fixes

* **deps:** update module github.com/azure/azure-sdk-for-go/sdk/azidentity to v1.15.0 ([#99](https://github.com/andrewheberle/graph-smtpd/issues/99)) ([efbd20f](https://github.com/andrewheberle/graph-smtpd/commit/efbd20f881e5be831674c9c3876f0d3728b65237))
* **deps:** update module github.com/emersion/go-smtp to v0.25.0 ([#113](https://github.com/andrewheberle/graph-smtpd/issues/113)) ([c2e42a2](https://github.com/andrewheberle/graph-smtpd/commit/c2e42a22ea2561e83e4f240fa70e6817415de478))
* **deps:** update module github.com/microsoftgraph/msgraph-sdk-go to v1.104.0 ([#94](https://github.com/andrewheberle/graph-smtpd/issues/94)) ([ebed936](https://github.com/andrewheberle/graph-smtpd/commit/ebed9369f656e1bdb6952d427bada9857e78c49e))
* **deps:** update module github.com/prometheus/client_golang to v1.25.0 ([#114](https://github.com/andrewheberle/graph-smtpd/issues/114)) ([f12ad7d](https://github.com/andrewheberle/graph-smtpd/commit/f12ad7d28cf29bbc88df0e12782fbeb08406344b))
* **graph-sendmail:** log all errors through the configured logger ([43de881](https://github.com/andrewheberle/graph-smtpd/commit/43de8817a116c2d5c80a728f3a3c610f896f1fe8))
* **graph-sendmail:** read the client ID from the clientid key ([2f3612f](https://github.com/andrewheberle/graph-smtpd/commit/2f3612fa2cd5cc1ec55e4a7ab8264678dd81d383))
* **graph-smtpd:** exit when the secret file cannot be read ([d1aaac2](https://github.com/andrewheberle/graph-smtpd/commit/d1aaac2d64764b00a8c6aa21d7809800031da63e))
* **graph-smtpd:** read the secret from SMTPD_SECRET_FILE ([d1aaac2](https://github.com/andrewheberle/graph-smtpd/commit/d1aaac2d64764b00a8c6aa21d7809800031da63e))
* **graphserver:** count graph_smtpd_email_total per sent message ([2f3612f](https://github.com/andrewheberle/graph-smtpd/commit/2f3612fa2cd5cc1ec55e4a7ab8264678dd81d383))
* **graphserver:** reply 451 when delivery to Graph fails ([43de881](https://github.com/andrewheberle/graph-smtpd/commit/43de8817a116c2d5c80a728f3a3c610f896f1fe8))
* **sendmail/graphserver:** numerous bug fixes ([#103](https://github.com/andrewheberle/graph-smtpd/issues/103)) ([d9ebb0d](https://github.com/andrewheberle/graph-smtpd/commit/d9ebb0d7985c50f30cf4123a618eb1c179113e29))
* **sendmail:** replace parsemail with go-message ([#107](https://github.com/andrewheberle/graph-smtpd/issues/107)) ([08a8797](https://github.com/andrewheberle/graph-smtpd/commit/08a8797d50dd9104ef849c464c64e07ba9c35d0a))
