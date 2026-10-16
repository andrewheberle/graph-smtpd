# Glossary

**Envelope**: the SMTP `MAIL FROM` and `RCPT TO` addresses for one message. The Envelope decides who a message is sent as and who it is delivered to; the message headers are only for display.

**Message**: the Microsoft Graph `sendMail` payload (`sendmail.Message`): sender, recipients, subject, body, attachments and the save-to-Sent-Items flag.

**Message intake**: turning a raw MIME message plus its Envelope into a Message (`sendmail.FromMIME`). Both `graph-smtpd` and `graph-sendmail` go through Message intake; when there is no Envelope (as in `graph-sendmail`), the headers stand in for it.

**Mailer**: what delivers a Message (`graphserver.Mailer`). `sendmail.GraphMailer` delivers via Microsoft Graph, sending as the mailbox the Message is from; tests use an in-memory fake.

**Relay policy**: which client sources may relay and which senders may send (`graphserver.relayPolicy`). Sources are IP addresses or CIDR ranges; senders are matched case-insensitively; an empty list allows everything. A denied message is refused at `MAIL FROM` with `550 5.7.1` and counted once in `graph_smtpd_email_denied_total`.
