# graph-smtpd

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
    ghcr.io/andrewheberle/graph-smtpd:v0.7.0
```

### Command-Line Options

* `--addr`: Listen address (default = "localhost:2525") (string)
* `--cert`: Certificate for enabling STARTTLS (string)
* `--clientid`: Client/Application ID (string)
* `--key`: Private key for enabling STARTTLS (string)
* `--sentitems`: Save to senders sent items (bool)
* `--secret`: Client Secret (string)
* `--senders`: Allowed senders ([]string)
* `--sources`: Allowed source IP addresses ([]string)
* `--tenantid`: Tenant ID (string)
* `--metrics`: Listen address for metrics (string)

All command line options may be specified as environment variables in the form of `SMTPD_<option>`, with the additional option to supply `SMTPD_SECRET_FILE` to allow loading of the client secret from a file.

### Configuration File

All configuration options may be provided in a YAML or JSON configuration file using the `--config` command-line option or if this is not set, will be looked for in the current working directory as `config.yaml`.

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
* `--sentitems`: Save to senders sent items (bool)
* `--secret`: Client Secret (string)
* `--tenantid`: Tenant ID (string)
* `--quiet`: Silence any output (bool)
* `--debug`: Enable debug logging (bool)

All command line options may be specified as environment variables in the form of `SENDMAIL_<option>`.

## Status

### What works

Based on limited testing, sending of plain text and HTML emails with or without an attachment works correctly.

Sending to one or more recipients via Cc/Bcc also works. 

### TODO

* Make logging better
* SMTP authentication
* Allow running as a service on Windows
* Test with wider variety of devices
* Implement unit tests
