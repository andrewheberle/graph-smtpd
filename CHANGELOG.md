# Changelog

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
