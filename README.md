# graph-smtpd

[![codecov](https://codecov.io/gh/andrewheberle/graph-smtpd/graph/badge.svg?token=F1QP6YR3GP)](https://codecov.io/gh/andrewheberle/graph-smtpd)

This service is a SMTP daemon that will accept emails via SMTP and then submit them via the Microsoft Graph API to Microsoft 365, however in theory the submission process could be generalised to support any API driven email submission service.

The idea is to allow this service to run locally and accept emails from devices/systems that cannot support the modern authentication requirements of Exchange Online, such as OAuth2, and then relay them securely to Microsoft 365.

## Azure AD/Entra ID Requirements

1. Create an "App Registration"
2. Note down the Client/Application ID and Tenant ID
3. Create a Client Secret and record the value
4. Set permissions (see below)

### Permissions

The App Registration requires the `mail.send` Graph API permission and to have admin consent granted in your tenant.

This will allow the service to send as any user in your environment.

To limit this ability to specific mailboxes/senders, it is possible to implement an `ApplicationAccessPolicy` to control this as follows:

1. Create a new mail enabled security group (or use an existing one)
2. Add the mailboxes that the service is allowed to send as to the mail enabled security group
3. Create the policy using Exchange Online PowerShell:
   ```powershell
   New-ApplicationAccessPolicy -AppId <APPLICATION_ID> -PolicyScopeGroupId <GROUP_EMAIL_ADDRESS> -AccessRight RestrictAccess -Description <DESCRIPTION>
   ```

This will limit the service to being able to send only as the members of the provided group.

## Running

### Docker

```sh
docker run -p 25:2525 \
    -e SMTPD_CLIENTID="clientid" \
    -e SMTPD_SECRET="secret" \
    -e SMTPD_TENANTID="tenantid" \
    ghcr.io/andrewheberle/graph-smtpd:v0.12.0 # x-release-please-version
```

The container listens for SMTP on port 2525 and serves metrics on port 8080.

### Command-Line Options

* `--addr`: Listen address (default = "localhost:2525") (string)
* `--cert`: Certificate for enabling STARTTLS, reloaded when it changes on disk (string)
* `--clientid`: Client/Application ID (string)
* `--config`: Configuration file (string)
* `--conntimeout`: Time allowed for each read or write on a client connection, after which an idle or stalled client is disconnected (default = 5m, 0 for no limit) (duration)
* `--debug`: Enable debug logging (bool)
* `--domain`: Hostname announced in the SMTP greeting (default = "localhost") (string)
* `--key`: Private key for enabling STARTTLS (string)
* `--max`: Maximum message size in bytes (default = 1048576) (int)
* `--metrics`: Listen address for metrics, served at `/metrics` (string)
* `--recipients`: Maximum recipients per message (default = 10) (int)
* `--secret`: Client Secret (string)
* `--senders`: Allowed senders, matched case-insensitively ([]string)
* `--sentitems`: Save to senders sent items (bool)
* `--sources`: Allowed source IP addresses or CIDR ranges, such as `10.0.0.0/8` ([]string)
* `--tenantid`: Tenant ID (string)
* `--timeout`: Time allowed to send each message via Graph, after which the client is asked to retry (default = 30s, 0 for no limit) (duration)
* `--users`: File of SMTP AUTH users, see [SMTP Authentication](#smtp-authentication) (string)
* `--version`: Display version and exit (bool)

All command line options may be specified as environment variables in the form of `SMTPD_<option>`. List options such as `SMTPD_SENDERS` take a comma-separated value.

The client secret may instead be loaded from a file by setting `SMTPD_SECRET_FILE` (or `secret_file` in the configuration file). The file is only used when no secret is set, and graph-smtpd will not start if the file cannot be read.

### Configuration File

All configuration options may be provided in a YAML configuration file using the `--config` command-line option.

Options are applied in this order, with later sources overriding earlier ones: command-line defaults, the configuration file, environment variables, then command-line options.

### SMTP Authentication

Clients can log in with SMTP AUTH, using the `PLAIN` or `LOGIN` mechanisms, when `--users` names a file of users. AUTH is only offered after the client has started TLS with STARTTLS, so `--users` requires `--cert` and `--key`.

The users file uses the htpasswd format, one `username:hash` per line, with bcrypt hashes only. Blank lines and lines starting with `#` are ignored. Create entries with `htpasswd`:

```sh
htpasswd -nB scanner >> users
```

With `--users` set:

* a client that has logged in may relay from any address
* a client that has not logged in may only relay from an address in `--sources`; if `--sources` is empty, every client must log in
* `--senders` applies to every client, logged in or not

A failed login is answered with `535 5.7.8` after a one second delay, logged with the username and client address, and counted in `graph_smtpd_auth_failures_total`. The users file is read at startup, so restart graph-smtpd after changing it.

### SMTP Replies

Clients see these replies when a message is not accepted:

* `550 5.7.1`: the client has not logged in and its address is not in `--sources`, or the sender is not in `--senders`
* `535 5.7.8`: the username or password is wrong
* `451 4.4.1`: sending via Graph failed or timed out; the client should retry later
* `550 5.7.1`, `550 5.1.1`, `552 5.3.4` or `554 5.0.0`: Graph refused the message (the app may not send as the sender, the mailbox does not exist, the message is too large, or another rejection), with Graph's error code in the reply
* `554 5.6.0`: the message could not be processed, for example it is not a valid MIME message

Recipients are taken from the SMTP envelope (`RCPT TO`): addresses in the `To` or `Cc` headers that are not envelope recipients are not sent to, and envelope recipients that are not in those headers are sent as Bcc.

### Logging

graph-smtpd logs JSON to standard error. Every message the relay is offered gets one `message` line once its outcome is known, so the log shows what happened to each message and why:

```json
{"level":"INFO","msg":"message","conn":"57347a255e","client":"10.0.0.5:51544","helo":"printer01","tls":true,"id":"57347a255e.1","from":"scanner@contoso.com","rcpt":["a@contoso.com"],"size":48211,"message_id":"<abc123@printer01>","status":"sent","reply":"250 2.0.0 OK: queued","duration":"812ms"}
```

* `status` is `sent`, `rejected` (refused permanently), `deferred` (refused temporarily, so the client should retry) or `abandoned` (the client gave up before sending the message)
* `reply` is the SMTP reply the client received, and `reason` the underlying error for anything not sent
* `conn` ties together the lines for one connection, including `connect`, `starttls`, `auth` and `disconnect` lines; `id` numbers the messages on it
* `user` is the SMTP AUTH user, once the client has logged in; failed logins are logged as `auth failed` at `WARN`
* `message_id` is the message's `Message-ID` header, which is passed to Microsoft 365 so the message can be found in Exchange message trace

Sent and abandoned messages are logged at `INFO`, and rejected and deferred messages at `WARN`. Subjects and header recipients are only logged with `--debug`.


When `--metrics` is set, the following counters are available alongside the standard Go and process metrics:

* `graph_smtpd_email_total`: messages sent
* `graph_smtpd_email_denied_total`: messages refused by `--sources` or `--senders`
* `graph_smtpd_email_errors_total`: messages that could not be processed or sent
* `graph_smtpd_auth_failures_total`: failed SMTP AUTH logins

## CLI "sendmail" mode

This is a "sendmail-ish" command line tool.

### Building

```sh
go install github.com/andrewheberle/graph-smtpd/cmd/graph-sendmail@latest
```

### Running

```sh
cat email.txt | ./graph-sendmail
```

### Command-Line Options

* `--clientid`: Client/Application ID (string)
* `--config`: Configuration file (string)
* `--debug`: Enable debug logging (bool)
* `--quiet`: Silence any output (bool)
* `--secret`: Client Secret (string)
* `--sentitems`: Save to senders sent items (bool)
* `--tenantid`: Tenant ID (string)
* `--timeout`: Time allowed to send the message via Graph (default = 30s, 0 for no limit) (duration)
* `--version`: Display version and exit (bool)

The message is read from standard input and sent as the address in its `From` header.

All command line options may be specified as environment variables in the form of `SENDMAIL_<option>`, and the client secret may be loaded from a file using `SENDMAIL_SECRET_FILE`, in the same way as for graph-smtpd.

All options may also be provided in a YAML configuration file using the `--config` command-line option.

### Exit Status

* `0`: the message was sent
* `75`: a temporary failure, such as a network error, timeout or Graph being unavailable; retrying later may succeed (`EX_TEMPFAIL`, as used by sendmail)
* `1`: any other failure, including invalid configuration, a message that cannot be parsed, or Graph refusing the message (for example the app not being allowed to send as the sender)

## Smoke testing

Before a release, run the interactive smoke test against a Microsoft 365 tenant you control:

```sh
./scripts/smoke-test/smoke-test.sh
```

It builds both binaries from the working tree, sends a set of test messages through your tenant (text and HTML, inline images, attachments, character sets, envelope-only Bcc, relay denials and more), asks you to confirm what arrived in two mailboxes, and checks the rest automatically. It needs `go`, `curl` and `bash`, plus the tenant ID, client ID and a file containing the client secret; nothing you enter is saved.

## Status

### What works

The following are covered by unit tests, and most are also checked against Microsoft 365 by the smoke test:

* Plain text and HTML messages; when a message has both, the HTML version is sent
* Inline images referenced from HTML with `cid:` URLs
* Attachments in any transfer encoding, and messages whose whole body is a file such as a PDF
* Text in character sets other than UTF-8, such as Windows-1252, converted for Graph
* Delivery to envelope recipients, including envelope-only recipients as Bcc
* Several messages on one SMTP connection
* Relay restrictions by source address or range and by sender
* Temporary Graph failures answered with `451` so clients retry, and Graph rejections answered permanently
* Loading the client secret from a file
* STARTTLS with `--cert` and `--key`, including picking up a renewed certificate without a restart (unit tests only)
* SMTP AUTH with `PLAIN` and `LOGIN` over TLS (unit tests only)

### TODO

* Allow running as a service on Windows
